// Package billing 统一人民币日额度、预留与供应商请求结算；账本由存储层持久化。
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	NanoPerYuan     int64 = 1_000_000_000
	DefaultLimit    int64 = 5 * NanoPerYuan
	MaxLimit        int64 = 1_000_000 * NanoPerYuan
	MaxInputBytes         = 64 * 1024
	MaxOutputTokens       = 4096
	ToolCost        int64 = 5_000_000
)

var (
	ErrExceeded    = errors.New("今日 AI 额度不足，请明天再试或联系管理员调整")
	ErrUnavailable = errors.New("AI 费用暂时无法确认，请联系管理员检查模型配置或待核算请求")
	ErrDisabled    = errors.New("账号已被禁用或删除，不能发起 AI 请求")
	ErrLastAdmin   = errors.New("不能取消最后一个可用管理员")
	moneyPattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,6})(\.[0-9]{1,2})?$`)
)

type Snapshot struct {
	Date     string `json:"date"`
	Limit    *int64 `json:"-"`
	Spent    int64  `json:"-"`
	Reserved int64  `json:"-"`
	Pending  int    `json:"pending_requests"`
}

// MarshalJSON 将精确整数金额转换成十进制字符串，避免浏览器浮点累加。
func (s Snapshot) MarshalJSON() ([]byte, error) {
	var limit, remaining *string
	if s.Limit != nil {
		l := Money(*s.Limit)
		r := Money(max(int64(0), *s.Limit-s.Spent-s.Reserved))
		limit = &l
		remaining = &r
	}
	return json.Marshal(struct {
		Date      string  `json:"date"`
		Limit     *string `json:"daily_limit_cny"`
		Spent     string  `json:"spent_cny"`
		Reserved  string  `json:"reserved_cny"`
		Remaining *string `json:"remaining_cny"`
		Pending   int     `json:"pending_requests"`
	}{s.Date, limit, Money(s.Spent), Money(s.Reserved), remaining, s.Pending})
}

// Money 返回最多九位小数的人民币字符串；参数为纳元，返回值至少含两位小数。
func Money(n int64) string {
	digits := fmt.Sprintf("%09d", n%NanoPerYuan)
	digits = strings.TrimRight(digits, "0")
	for len(digits) < 2 {
		digits += "0"
	}
	return fmt.Sprintf("%d.%s", n/NanoPerYuan, digits)
}

// ParseLimit 校验必填金额，null 表示不限额；返回精确纳元或校验错误。
func ParseLimit(raw json.RawMessage) (*int64, error) {
	if len(raw) == 0 {
		return nil, errors.New("请设置每日 AI 额度或选择不限额")
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		value = string(raw)
	}
	if !moneyPattern.MatchString(value) {
		return nil, errors.New("每日额度须为非负金额，最多两位小数")
	}
	parts := strings.Split(value, ".")
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	n := whole * NanoPerYuan
	if len(parts) == 2 {
		fraction, _ := strconv.ParseInt(parts[1]+strings.Repeat("0", 9-len(parts[1])), 10, 64)
		n += fraction
	}
	if n > MaxLimit {
		return nil, errors.New("每日额度不能超过 1000000 元")
	}
	return &n, nil
}

// Day 以北京时间确定请求所属日，不依赖数据库或设备默认时区。
func Day(t time.Time) string { return t.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02") }

type Reservation struct {
	Unpriced                                     bool
	ID, ParentID, Date, Purpose, Provider, Model string
	UserID                                       int
	Amount                                       int64
	Details                                      []byte
}

type Store interface {
	Snapshot(context.Context, int, string) (Snapshot, error)
	Reserve(context.Context, Reservation) error
	Settle(context.Context, string, int64, string, []byte) error
	WordOwner(context.Context, int) (int, error)
}

type Service struct {
	Store Store
	Now   func() time.Time
}

// New 创建持久化配额服务；store 提供短事务，clock 用于日期和价格快照。
func New(store Store) *Service { return &Service{Store: store, Now: time.Now} }

type ownerKey struct{}
type parentKey struct{}
type owner struct {
	ID      int
	Purpose string
}
type parent struct{ ID, Date string }

// WithOwner 绑定服务器确认的计费主体，后台任务不能用前端用户参数替代该身份。
func WithOwner(ctx context.Context, id int, purpose string) context.Context {
	return context.WithValue(ctx, ownerKey{}, owner{id, purpose})
}

// Owner 返回上下文中受信任的用户与业务来源。
func Owner(ctx context.Context) (int, string) {
	o, _ := ctx.Value(ownerKey{}).(owner)
	return o.ID, o.Purpose
}

type Price struct {
	Version           string `json:"version"`
	Model             string `json:"model"`
	Peak              bool   `json:"peak"`
	Hit, Miss, Output int64  // 每 token 纳元
}

// PriceFor 只对官方 HTTPS 接口和明确型号计价；节假日高峰窗口保守估算。
func PriceFor(base, model string, now time.Time) (Price, bool) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host != "api.deepseek.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/" && u.Path != "/v1" && u.Path != "/v1/") {
		return Price{}, false
	}
	p := Price{Version: "deepseek-2026-11-12", Model: model}
	switch model {
	case "deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp":
		p.Hit = 20
		p.Miss = 1000
		p.Output = 4000
	case "deepseek-v4-pro":
		p.Hit = 150
		p.Miss = 4500
		p.Output = 13500
	default:
		return Price{}, false
	}
	cst := now.In(time.FixedZone("CST", 8*3600))
	minute := cst.Hour()*60 + cst.Minute()
	p.Peak = cst.Weekday() != time.Saturday && cst.Weekday() != time.Sunday && ((minute >= 540 && minute < 720) || (minute >= 840 && minute < 1080))
	if p.Peak {
		p.Hit *= 2
		p.Miss *= 2
		p.Output *= 2
	}
	return p, true
}

// Upper 返回最高时段费率预留，输入 token 上界含消息框架，输出受 max_tokens 限制。
func (p Price) Upper(inputTokens, outputTokens int) int64 {
	factor := int64(2)
	if p.Peak {
		factor = 1
	}
	return (int64(inputTokens)*p.Miss + int64(outputTokens)*p.Output) * factor
}

type Usage struct {
	Prompt        *int64 `json:"prompt_tokens"`
	Completion    *int64 `json:"completion_tokens"`
	Hit           *int64 `json:"prompt_cache_hit_tokens"`
	Miss          *int64 `json:"prompt_cache_miss_tokens"`
	PromptDetails *struct {
		Cached *int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
}

// Cost 校验 usage 后按输入缓存/输出分别计价；缺缓存细分时保守按未命中估算。
func (p Price) Cost(u Usage, inputBound, outputBound int) (int64, error) {
	if u.Prompt == nil || u.Completion == nil || *u.Prompt < 0 || *u.Completion < 0 || *u.Prompt > int64(inputBound) || *u.Completion > int64(outputBound) {
		return 0, ErrUnavailable
	}
	hit := int64(0)
	if u.Hit != nil {
		hit = *u.Hit
	} else if u.PromptDetails != nil && u.PromptDetails.Cached != nil {
		hit = *u.PromptDetails.Cached
	}
	if hit < 0 || hit > *u.Prompt {
		return 0, ErrUnavailable
	}
	miss := *u.Prompt - hit
	if u.Miss != nil && (*u.Miss != miss || *u.Miss < 0) {
		return 0, ErrUnavailable
	}
	return hit*p.Hit + miss*p.Miss + *u.Completion*p.Output, nil
}

// Terminal 标识不应进入普通网络重试的额度、身份及费用未知错误。
func Terminal(err error) bool {
	return errors.Is(err, ErrExceeded) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrDisabled)
}
