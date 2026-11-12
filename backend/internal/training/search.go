package training

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"vocab-backend/internal/model"
)

type ExamSource struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Year    string `json:"year"`
	Content string `json:"content"`
}

var examYear = regexp.MustCompile(`(?:19|20)\d{2}`)
var englishWords = regexp.MustCompile(`[A-Za-z]{2,}`)
var markdownLinks = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

// toolJSON 调用 DeepSeek 原生只读搜索/获取接口，沿用同一 Key 与有界客户端。
func (p *Provider) toolJSON(ctx context.Context, tool string, input any, dst any) error {
	base := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	data, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/tools/"+tool, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := p.Client.Do(req)
	if err != nil {
		return &ProviderError{Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return &ProviderError{Status: response.StatusCode, Retryable: response.StatusCode == 429 || response.StatusCode >= 500}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || json.Unmarshal(raw, dst) != nil {
		return Fail(502, "invalid_search_response", "搜索服务返回内容不完整")
	}
	return nil
}

// publicExamURL 拒绝私有地址和带凭证的链接，只接受可公开核对的来源。
func publicExamURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified()) {
		return false
	}
	return true
}

// usableExamSource 只选择具备考试年份与正文结构的材料，排除复习规划和词表。
func usableExamSource(title, content string) string {
	if !strings.Contains(title, "英语") || (!strings.Contains(title, "真题") && !strings.Contains(title, "试题") && !strings.Contains(title, "试卷")) {
		return ""
	}
	for _, bad := range []string{"规划", "从哪年", "技巧", "词汇表", "怎么做", "备考建议", "复习", "模拟", "预测"} {
		if strings.Contains(title, bad) {
			return ""
		}
	}
	years := examYear.FindAllString(title, -1)
	unique := map[string]bool{}
	for _, year := range years {
		unique[year] = true
	}
	if len(unique) != 1 {
		return ""
	}
	year := examYear.FindString(title)
	number, _ := strconv.Atoi(year)
	if number < 1980 || number > time.Now().Year() {
		return ""
	}
	low := strings.ToLower(content)
	paper := strings.Contains(low, "reading comprehension") || strings.Contains(low, "text 1") || strings.Contains(low, "text1") || strings.Contains(low, "[a]") || strings.Contains(low, "(a)")
	if !paper || len(englishWords.FindAllString(content, -1)) < 30 {
		return ""
	}
	return year
}

// SearchExam 自动检索原文；需要时获取正文，搜索摘要本身不用于证明真题。
func (p *Provider) SearchExam(ctx context.Context, word string) ([]ExamSource, error) {
	var response struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	query := "考研英语 阅读 真题 原文 \"" + word + "\" -规划 -复习计划 -词汇表"
	if len([]rune(query)) > 128 {
		query = "考研英语 真题 阅读原文"
	}
	if err := p.toolJSON(ctx, "search", map[string]any{"query": query, "count": 5, "need_content": true}, &response); err != nil {
		return nil, err
	}
	sources := []ExamSource{}
	for _, item := range response.Results {
		if !publicExamURL(item.URL) || !strings.Contains(item.Title, "真题") || !strings.Contains(item.Title, "英语") {
			continue
		}
		content := item.Content
		title := item.Title
		if content == "" {
			var page struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			}
			if err := p.toolJSON(ctx, "fetch", map[string]any{"url": item.URL}, &page); err != nil {
				continue
			}
			content = page.Content
			if page.Title != "" {
				title = page.Title
			}
		}
		year := usableExamSource(title, content)
		if year == "" {
			continue
		}
		sources = append(sources, ExamSource{ID: "exam" + strconv.Itoa(len(sources)+1), Title: title, URL: item.URL, Year: year, Content: content})
		if len(sources) == 2 {
			break
		}
	}
	return sources, nil
}

// evidenceText 统一网页排版与空白，不改变单词或语义。
func evidenceText(text string) string {
	text = html.UnescapeString(text)
	text = markdownLinks.ReplaceAllString(text, "$1")
	text = strings.NewReplacer("*", "", "_", "", "`", "", "“", "\"", "”", "\"", "’", "'", "\u00a0", " ").Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

// verifiedCitation 只给实际正文含有完整引用与题目文字的片段加真题标签。
func verifiedCitation(id, text, quote string, sources []ExamSource) *model.TrainingCitation {
	q := evidenceText(quote)
	t := evidenceText(text)
	if len(englishWords.FindAllString(q, -1)) < 6 || len(q) > 2000 || !strings.Contains(q, t) {
		return nil
	}
	for _, source := range sources {
		if source.ID == id && publicExamURL(source.URL) && usableExamSource(source.Title, source.Content) != "" && strings.Contains(evidenceText(source.Content), q) {
			return &model.TrainingCitation{Title: source.Title, URL: source.URL, Year: source.Year, Quote: quote}
		}
	}
	return nil
}

// ApplyExamCitations 清除模型自报标签，只保留能核对的题干或选项出处。
func ApplyExamCitations(q *model.TrainingQuestion, sources []ExamSource) {
	q.Exam = verifiedCitation(q.ExamSourceID, q.Stem, q.ExamQuote, sources)
	q.ExamSourceID = ""
	q.ExamQuote = ""
	for i := range q.Options {
		o := &q.Options[i]
		o.Exam = verifiedCitation(o.ExamSourceID, o.Text, o.ExamQuote, sources)
		o.ExamSourceID = ""
		o.ExamQuote = ""
	}
}

// SourceContext 给模型足够的原文语境，避免发送整篇长网页。
func SourceContext(sources []ExamSource, word string) []ExamSource {
	out := make([]ExamSource, len(sources))
	for i, s := range sources {
		text := s.Content
		at := strings.Index(strings.ToLower(text), strings.ToLower(word))
		start := 0
		if at > 2000 {
			start = at - 2000
		}
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		end := min(len(text), start+6500)
		for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		s.Content = text[start:end]
		out[i] = s
	}
	return out
}
