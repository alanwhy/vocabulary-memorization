package training

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"
	"vocab-backend/internal/model"
)

type Problem struct {
	Status  int
	Code    string
	Message string
}

// Error 返回可以向用户显示的错误，不包含远端响应或凭证。
func (e *Problem) Error() string { return e.Message }

// Fail 为调用方提供稳定错误码和状态。
func Fail(status int, code, message string) error { return &Problem{status, code, message} }

// Hash 用于临时作答请求的幂等比较，不存储到数据库。
func Hash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Text 检查非空和长度，避免截断导致句意变化。
func Text(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
}

// ValidateQuestion 检查四选一内容、答案、逐字映射及短解析。
func ValidateQuestion(q model.TrainingQuestion, _ string) error {
	if !Text(q.Stem, 2000) || len(strings.Fields(q.Stem)) > 80 || len(q.Options) != 4 {
		return Fail(422, "invalid_question", "模型返回的题干或选项不完整")
	}
	ids, texts := map[string]bool{}, map[string]bool{}
	answer := ""
	for _, o := range q.Options {
		key := strings.ToLower(strings.Join(strings.Fields(o.Text), " "))
		if !Text(o.ID, 8) || !Text(o.Text, 1000) || ids[o.ID] || texts[key] {
			return Fail(422, "invalid_options", "模型返回了重复或无效选项")
		}
		ids[o.ID] = true
		texts[key] = true
		if o.ID == q.CorrectOptionID {
			answer = o.Text
		}
	}
	if answer == "" {
		return Fail(422, "invalid_answer", "正确答案没有对应选项")
	}
	if len(q.Mappings) < 1 || len(q.Mappings) > 3 {
		return Fail(422, "invalid_mapping", "替换对应关系不完整")
	}
	for _, m := range q.Mappings {
		if !Text(m.Original, 300) || !Text(m.Replacement, 300) || !Text(m.Note, 300) || !strings.Contains(q.Stem, m.Original) || !strings.Contains(answer, m.Replacement) {
			return Fail(422, "invalid_mapping", "替换表达与题目内容没有对应")
		}
	}
	if len(q.Explanations) != 4 {
		return Fail(422, "invalid_explanation", "选项解释不完整")
	}
	seen := map[string]bool{}
	for _, e := range q.Explanations {
		if !ids[e.ID] || seen[e.ID] || !Text(e.Text, 300) {
			return Fail(422, "invalid_explanation", "选项解释无效")
		}
		seen[e.ID] = true
	}
	return nil
}

// ValidateBundle 检查一个词只有一道有效替换题和明确义项。
func ValidateBundle(b model.TrainingBundle) error {
	c := b.Relation
	if !Text(c.Anchor, 200) || !Text(c.Meaning, 200) || !Text(c.Replacement, 200) || len(b.Questions) != 1 {
		return Fail(422, "invalid_relation", "同义替换内容不完整")
	}
	return ValidateQuestion(b.Questions[0], "basic")
}

// AcceptReview 只接受复核模型找到同一个唯一答案的题。
func AcceptReview(q model.TrainingQuestion, r model.TrainingReview) error {
	if len(r.ValidOptionIDs) != 1 || r.ValidOptionIDs[0] != q.CorrectOptionID || len(r.Issues) != 0 {
		return Fail(422, "ambiguous_question", "题目未通过句意与唯一答案检查")
	}
	return nil
}

// DecodeModel 解析模型 JSON，并兼容额外的代码围栏。
func DecodeModel(text string, dst any) error {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	if json.Unmarshal([]byte(text), dst) != nil {
		return Fail(422, "invalid_model_json", "模型返回内容格式不完整")
	}
	return nil
}
