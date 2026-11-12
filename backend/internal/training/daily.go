// 当日练习队列：只读个人词库，题目、游标和作答只存在内存。
package training

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"sort"
	"sync"
	"time"
	"vocab-backend/internal/model"
)

type DailyWordSource interface {
	ListPage(context.Context, int, bool, string, string, string, int, int) ([]model.Word, error)
	FindByIDs(context.Context, int, []int) ([]model.Word, error)
}
type DailyGenerator func(context.Context, model.Word) (model.TrainingBundle, error)
type DailyQuestion struct {
	Stem    string                  `json:"stem"`
	Options []model.TrainingOption  `json:"options"`
	Exam    *model.TrainingCitation `json:"exam,omitempty"`
}
type DailyCard struct {
	SearchStatus string         `json:"search_status,omitempty"`
	WordID       int            `json:"word_id"`
	Word         string         `json:"word"`
	Count        int            `json:"count"`
	Question     *DailyQuestion `json:"question,omitempty"`
}
type DailyFeedback struct {
	Correct       bool                    `json:"correct"`
	Selected      *string                 `json:"selected_option_id"`
	CorrectOption string                  `json:"correct_option_id"`
	Meaning       string                  `json:"meaning"`
	Replacement   string                  `json:"replacement"`
	Explanation   string                  `json:"explanation"`
	Mappings      []model.TrainingMapping `json:"mappings"`
}
type DailySnapshot struct {
	Date       string         `json:"date"`
	GroupID    string         `json:"group_id"`
	Status     string         `json:"status"`
	Total      int            `json:"total"`
	Position   int            `json:"position"`
	Generated  int            `json:"generated"`
	Generating bool           `json:"generating"`
	Card       *DailyCard     `json:"card"`
	Feedback   *DailyFeedback `json:"feedback"`
	Message    string         `json:"message,omitempty"`
}
type dailySlot struct {
	word       model.Word
	state      string
	bundle     model.TrainingBundle
	feedback   *DailyFeedback
	answerHash string
	message    string
	cancel     context.CancelFunc
}
type dailyRound struct {
	id     string
	slots  []*dailySlot
	cursor int
}
type dailyUser struct {
	mu       sync.Mutex
	date     string
	seen     map[int]bool
	requests map[string]bool
	round    *dailyRound
	message  string
	ctx      context.Context
	cancel   context.CancelFunc
}
type DailyManager struct {
	mu          sync.Mutex
	users       map[int]*dailyUser
	source      DailyWordSource
	generate    DailyGenerator
	now         func() time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	closed      bool
	wg          sync.WaitGroup
	sem         chan struct{}
	cleanupDone chan struct{}
}

// NewDailyManager 创建独立账号的当日队列，并在午夜释放上一日内容。
func NewDailyManager(parent context.Context, source DailyWordSource, generate DailyGenerator, now func() time.Time) *DailyManager {
	ctx, cancel := context.WithCancel(parent)
	m := &DailyManager{users: map[int]*dailyUser{}, source: source, generate: generate, now: now, ctx: ctx, cancel: cancel, sem: make(chan struct{}, 3), cleanupDone: make(chan struct{})}
	go m.cleanup()
	return m
}

// day 返回上海日期，避免设备时区影响当日内容的有效期。
func (m *DailyManager) day() string {
	return m.now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
}

// user 在读取时立即作废旧日数据，不依赖后台清理时机。
func (m *DailyManager) user(id int) (*dailyUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, Fail(503, "service_stopping", "服务正在重新启动")
	}
	day := m.day()
	u := m.users[id]
	if u != nil && u.date != day {
		u.cancel()
		delete(m.users, id)
		u = nil
	}
	if u == nil {
		ctx, cancel := context.WithCancel(m.ctx)
		u = &dailyUser{date: day, seen: map[int]bool{}, requests: map[string]bool{}, ctx: ctx, cancel: cancel}
		m.users[id] = u
	}
	return u, nil
}

// orderWords 沿用词库次数、最近出现时间及 ID 的稳定降序，不随机选词。
func orderWords(words []model.Word) {
	sort.SliceStable(words, func(i, j int) bool {
		a, b := words[i], words[j]
		if a.ReviewCount != b.ReviewCount {
			return a.ReviewCount > b.ReviewCount
		}
		if !a.LastReviewedAt.Equal(b.LastReviewedAt) {
			return a.LastReviewedAt.After(b.LastReviewedAt)
		}
		return a.ID > b.ID
	})
}

// selectWords 从当前未归档词中继续取未生成过的十个；归档变化不会因 offset 跳过单词。
func (m *DailyManager) selectWords(ctx context.Context, user int, seen map[int]bool) ([]model.Word, error) {
	selected := []model.Word{}
	picked := map[int]bool{}
	for offset := 0; len(selected) < 10; offset += 100 {
		page, err := m.source.ListPage(ctx, user, false, "", "", "count", 100, offset)
		if err != nil {
			return nil, err
		}
		orderWords(page)
		for _, word := range page {
			if !word.Archived && !seen[word.ID] && !picked[word.ID] {
				selected = append(selected, word)
				picked[word.ID] = true
				if len(selected) == 10 {
					break
				}
			}
		}
		if len(page) < 100 {
			break
		}
	}
	return selected, nil
}

// Ensure 自动准备今日首组或应用户请求继续下一批；并发重复调用共用队列。
func (m *DailyManager) Ensure(ctx context.Context, user int, more bool, requestID string) (DailySnapshot, error) {
	u, err := m.user(user)
	if err != nil {
		return DailySnapshot{}, err
	}
	u.mu.Lock()
	if u.round != nil {
		pending := false
		for _, s := range u.round.slots {
			pending = pending || s.state == "pending"
		}
		if !more || pending || u.requests[requestID] {
			u.mu.Unlock()
			return m.State(ctx, user)
		}
	}
	words, err := m.selectWords(ctx, user, u.seen)
	if err != nil {
		u.mu.Unlock()
		return DailySnapshot{}, err
	}
	if len(words) == 0 {
		if u.round == nil {
			u.message = "没有未归档的单词，先到词库添加单词。"
		} else {
			u.message = "今天的未归档单词已全部生成，明天会重新准备。"
		}
		if requestID != "" {
			u.requests[requestID] = true
		}
		u.mu.Unlock()
		return m.State(ctx, user)
	}
	bytes := make([]byte, 12)
	if _, err = rand.Read(bytes); err != nil {
		u.mu.Unlock()
		return DailySnapshot{}, err
	}
	round := &dailyRound{id: hex.EncodeToString(bytes)}
	for _, w := range words {
		cancel := func() {}
		round.slots = append(round.slots, &dailySlot{word: w, state: "pending", cancel: cancel})
		u.seen[w.ID] = true
	}
	u.round = round
	u.message = ""
	if requestID != "" {
		u.requests[requestID] = true
	}
	m.mu.Lock()
	if m.closed || m.users[user] != u || u.date != m.day() {
		m.mu.Unlock()
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "day_changed", "当天练习已更新，请重新读取")
	}
	m.wg.Add(len(round.slots))
	for _, s := range round.slots {
		go m.generateSlot(user, u, round, s)
	}
	m.mu.Unlock()
	u.mu.Unlock()
	return m.State(ctx, user)
}

// generateSlot 生成前后都检查归档；结果只能写入同一用户同一天的内存队列。
func (m *DailyManager) generateSlot(user int, u *dailyUser, round *dailyRound, slot *dailySlot) {
	defer m.wg.Done()
	ctx, cancel := context.WithCancel(u.ctx)
	defer cancel()
	u.mu.Lock()
	slot.cancel = cancel
	u.mu.Unlock()
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return
	}
	words, err := m.source.FindByIDs(ctx, user, []int{slot.word.ID})
	alive := err == nil && len(words) == 1 && !words[0].Archived
	var bundle model.TrainingBundle
	if alive {
		bundle, err = m.generate(ctx, words[0])
	} else if err == nil {
		err = Fail(409, "word_archived", "单词已归档或删除")
	}
	if err == nil {
		fresh, e := m.source.FindByIDs(ctx, user, []int{slot.word.ID})
		if e != nil {
			err = e
		} else if len(fresh) != 1 || fresh[0].Archived {
			alive = false
		}
	}
	m.mu.Lock()
	valid := !m.closed && m.users[user] == u && u.date == m.day()
	m.mu.Unlock()
	if !valid || ctx.Err() != nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.round != round || slot.state == "archived" {
		return
	}
	if !alive {
		slot.state = "archived"
		return
	}
	if err != nil {
		slot.state = "failed"
		slot.message = "本词暂未生成成功，请重试。"
		if p, ok := err.(*Problem); ok {
			slot.message = p.Message
		}
		return
	}
	if len(bundle.Questions) == 1 {
		options := bundle.Questions[0].Options
		for i := len(options) - 1; i > 0; i-- {
			index, e := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			if e == nil {
				j := int(index.Int64())
				options[i], options[j] = options[j], options[i]
			}
		}
	}
	slot.bundle = bundle
	slot.state = "ready"
}

// State 只返回当前词，隐藏未作答答案，并即时移除归档或删除的词。
func (m *DailyManager) State(ctx context.Context, user int) (DailySnapshot, error) {
	u, err := m.user(user)
	if err != nil {
		return DailySnapshot{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	out := DailySnapshot{Date: u.date, Status: "empty", Message: u.message}
	r := u.round
	if r == nil {
		return out, nil
	}
	out.GroupID = r.id
	ids := []int{}
	for _, s := range r.slots {
		ids = append(ids, s.word.ID)
	}
	words, err := m.source.FindByIDs(ctx, user, ids)
	if err != nil {
		return out, err
	}
	alive := map[int]model.Word{}
	for _, w := range words {
		if !w.Archived {
			alive[w.ID] = w
		}
	}
	for _, s := range r.slots {
		w, ok := alive[s.word.ID]
		if !ok {
			s.state = "archived"
			s.bundle = model.TrainingBundle{}
			s.feedback = nil
			s.cancel()
		} else {
			s.word.ReviewCount = w.ReviewCount
		}
		if s.state != "archived" {
			out.Total++
			if s.state != "pending" {
				out.Generated++
			} else {
				out.Generating = true
			}
		}
	}
	for r.cursor < len(r.slots) && r.slots[r.cursor].state == "archived" {
		r.cursor++
	}
	out.Position = 1
	for i := 0; i < r.cursor; i++ {
		if r.slots[i].state != "archived" {
			out.Position++
		}
	}
	if r.cursor >= len(r.slots) {
		out.Status = "complete"
		if out.Total == 0 {
			out.Status = "empty"
			out.Message = "本组单词已全部归档或删除。"
		}
		return out, nil
	}
	s := r.slots[r.cursor]
	out.Card = &DailyCard{WordID: s.word.ID, Word: s.word.DisplayWord, Count: s.word.ReviewCount, SearchStatus: s.bundle.SearchStatus}
	out.Status = s.state
	out.Feedback = s.feedback
	if s.state == "pending" {
		out.Status = "generating"
	}
	if s.state == "failed" {
		out.Message = s.message
	}
	if s.state == "ready" && len(s.bundle.Questions) == 1 {
		q := s.bundle.Questions[0]
		out.Card.Question = &DailyQuestion{Stem: q.Stem, Options: q.Options, Exam: q.Exam}
	}
	return out, nil
}

// Answer 将当前题评分保存在内存，同一答案重试不重复推进或改变词库次数。
func (m *DailyManager) Answer(ctx context.Context, user int, group string, wordID int, selected *string) (DailySnapshot, error) {
	if _, err := m.State(ctx, user); err != nil {
		return DailySnapshot{}, err
	}
	u, err := m.user(user)
	if err != nil {
		return DailySnapshot{}, err
	}
	u.mu.Lock()
	r := u.round
	if r == nil || r.id != group || r.cursor >= len(r.slots) || r.slots[r.cursor].word.ID != wordID {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "stale_group", "当前练习已更新")
	}
	s := r.slots[r.cursor]
	if s.state != "ready" {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "not_ready", "这道题还未准备好")
	}
	hash := Hash(selected)
	if s.feedback != nil {
		same := s.answerHash == hash
		u.mu.Unlock()
		if !same {
			return DailySnapshot{}, Fail(409, "already_answered", "这道题已经作答")
		}
		return m.State(ctx, user)
	}
	q := s.bundle.Questions[0]
	valid := selected == nil
	correct := false
	if selected != nil {
		for _, o := range q.Options {
			valid = valid || o.ID == *selected
		}
		correct = *selected == q.CorrectOptionID
	}
	if !valid {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(422, "invalid_option", "所选答案不属于当前题目")
	}
	reason := ""
	for _, e := range q.Explanations {
		if (selected != nil && e.ID == *selected) || selected == nil && e.ID == q.CorrectOptionID {
			reason = e.Text
		}
	}
	s.feedback = &DailyFeedback{Correct: correct, Selected: selected, CorrectOption: q.CorrectOptionID, Meaning: s.bundle.Relation.Meaning, Replacement: s.bundle.Relation.Replacement, Explanation: reason, Mappings: q.Mappings}
	s.answerHash = hash
	u.mu.Unlock()
	return m.State(ctx, user)
}

// Advance 只推进本组当前词，不写复习次数或排期。
func (m *DailyManager) Advance(ctx context.Context, user int, group string, wordID int) (DailySnapshot, error) {
	u, err := m.user(user)
	if err != nil {
		return DailySnapshot{}, err
	}
	u.mu.Lock()
	r := u.round
	if r == nil || r.id != group || r.cursor >= len(r.slots) || r.slots[r.cursor].word.ID != wordID {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "stale_group", "当前练习已更新")
	}
	s := r.slots[r.cursor]
	if s.feedback == nil && s.state != "failed" && s.state != "archived" {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "not_answered", "请先选择答案")
	}
	r.cursor++
	u.mu.Unlock()
	return m.State(ctx, user)
}

// cleanup 在上海午夜取消并释放旧日生成和练习内容。
func (m *DailyManager) cleanup() {
	defer close(m.cleanupDone)
	for {
		now := m.now().In(time.FixedZone("CST", 8*3600))
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			m.mu.Lock()
			for id, u := range m.users {
				if u.date != m.day() {
					u.cancel()
					delete(m.users, id)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Close 先禁止新生成，再取消当前请求，让退出时不会继续调用模型。
func (m *DailyManager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	for _, u := range m.users {
		u.cancel()
	}
	m.users = map[int]*dailyUser{}
	m.mu.Unlock()
}

// WaitIdle 等待已启动生成结束，不等待长期的午夜清理循环。
func (m *DailyManager) WaitIdle() { m.wg.Wait() }

// Wait 等待关闭后的模型请求和日期清理完成。
func (m *DailyManager) Wait() { m.wg.Wait(); <-m.cleanupDone }

// Retry 只重新生成失败的当前词，仍使用当日内存队列。
func (m *DailyManager) Retry(ctx context.Context, user int, group string, wordID int) (DailySnapshot, error) {
	u, err := m.user(user)
	if err != nil {
		return DailySnapshot{}, err
	}
	u.mu.Lock()
	r := u.round
	if r == nil || r.id != group || r.cursor >= len(r.slots) || r.slots[r.cursor].word.ID != wordID || r.slots[r.cursor].state != "failed" {
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "retry_unavailable", "当前词不需要重试")
	}
	s := r.slots[r.cursor]
	m.mu.Lock()
	if m.closed || m.users[user] != u || u.date != m.day() {
		m.mu.Unlock()
		u.mu.Unlock()
		return DailySnapshot{}, Fail(409, "day_changed", "当天练习已更新")
	}
	s.state = "pending"
	s.message = ""
	m.wg.Add(1)
	go m.generateSlot(user, u, r, s)
	m.mu.Unlock()
	u.mu.Unlock()
	return m.State(ctx, user)
}
