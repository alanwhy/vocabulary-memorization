package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestWordOrderByWhitelist(t *testing.T) {
	cases := map[string]string{
		"time":       `last_reviewed_at DESC, id DESC`,
		"time_asc":   `last_reviewed_at ASC, id ASC`,
		"alpha":      `word_key ASC, id ASC`,
		"alpha_desc": `word_key DESC, id DESC`,
		"count":      `review_count DESC, last_reviewed_at DESC, id DESC`,
		"count_asc":  `review_count ASC, last_reviewed_at ASC, id ASC`,
	}
	for sort, want := range cases {
		if got := wordOrderBy(sort); got != want {
			t.Fatalf("wordOrderBy(%q) = %q, want %q", sort, got, want)
		}
	}
}

// 未知排序值必须退回默认排序，而不是把请求参数拼进 SQL
func TestWordOrderByRejectsUnknownSort(t *testing.T) {
	fallback := wordOrderBy("count")
	for _, sort := range []string{"", "id", "review_count", "1; DROP TABLE words"} {
		if got := wordOrderBy(sort); got != fallback {
			t.Fatalf("wordOrderBy(%q) = %q, want fallback %q", sort, got, fallback)
		}
	}
}

// 每种排序都必须以唯一列 id 收尾，否则 LIMIT/OFFSET 翻页会跨页重复或漏行
func TestWordOrderByAlwaysHasUniqueTiebreaker(t *testing.T) {
	for _, sort := range []string{"count", "count_asc", "time", "time_asc", "alpha", "alpha_desc", "unknown"} {
		clauses := strings.Split(wordOrderBy(sort), ",")
		last := strings.TrimSpace(clauses[len(clauses)-1])
		if !strings.HasPrefix(last, "id ") {
			t.Fatalf("wordOrderBy(%q) 最后一个排序键是 %q，应当是 id", sort, last)
		}
	}
}

func TestEscapeLikePattern(t *testing.T) {
	cases := map[string]string{
		"abc":     "abc",
		"100%":    `100\%`,
		"a_b":     `a\_b`,
		`back\sl`: `back\\sl`,
		`%_\`:     `\%\_\\`,
	}
	for in, want := range cases {
		if got := escapeLikePattern(in); got != want {
			t.Fatalf("escapeLikePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLikeContainsEmptyKeywordMatchesAll(t *testing.T) {
	if got := likeContains(""); got != "%" {
		t.Fatalf("likeContains(\"\") = %q, want %q", got, "%")
	}
}

func TestLikeContainsWrapsEscapedKeyword(t *testing.T) {
	if got := likeContains("50%off"); got != `%50\%off%` {
		t.Fatalf("likeContains(\"50%%off\") = %q, want %q", got, `%50\%off%`)
	}
}

func TestSenseFilterWhere(t *testing.T) {
	// 都为空 → 不过滤
	if conds, args := senseFilterWhere("", ""); conds != "" || args != nil {
		t.Fatalf("senseFilterWhere(\"\", \"\") = (%q, %v), want (\"\", nil)", conds, args)
	}

	// 仅 keyword → 同时匹配 word_key 与释义，两个占位符
	conds, args := senseFilterWhere("apple", "")
	if !strings.Contains(conds, "word_key LIKE ?") || !strings.Contains(conds, "JSON_SEARCH") {
		t.Fatalf("keyword filter missing LIKE/JSON_SEARCH: %q", conds)
	}
	if len(args) != 2 {
		t.Fatalf("keyword filter args = %d, want 2", len(args))
	}

	// 仅 status → 无占位符
	if conds, args := senseFilterWhere("", "no_definition"); !strings.Contains(conds, "JSON_LENGTH(senses) = 0") || len(args) != 0 {
		t.Fatalf("no_definition filter = (%q, %v)", conds, args)
	}
	if conds, args := senseFilterWhere("", "has_definition"); !strings.Contains(conds, "JSON_LENGTH(senses) > 0") || len(args) != 0 {
		t.Fatalf("has_definition filter = (%q, %v)", conds, args)
	}

	// 非法 status → 视为不过滤
	if conds, _ := senseFilterWhere("", "bogus"); conds != "" {
		t.Fatalf("bogus status should not filter, got %q", conds)
	}

	// keyword + status → 两个条件用 AND 连接，参数只有 keyword 的两个
	conds, args = senseFilterWhere("apple", "no_definition")
	if !strings.Contains(conds, " AND ") || len(args) != 2 {
		t.Fatalf("combined filter = (%q, %v)", conds, args)
	}
}

func TestNullTimePtr(t *testing.T) {
	if got := nullTimePtr(sql.NullTime{}); got != nil {
		t.Fatalf("expected nil for NULL time, got %v", got)
	}
	now := time.Now()
	got := nullTimePtr(sql.NullTime{Time: now, Valid: true})
	if got == nil || !got.Equal(now) {
		t.Fatalf("expected %v, got %v", now, got)
	}
}

func TestNewPageResultHasMore(t *testing.T) {
	cases := []struct {
		total, page, limit int
		wantHasMore        bool
	}{
		{total: 455, page: 1, limit: 100, wantHasMore: true},
		{total: 455, page: 5, limit: 100, wantHasMore: false},
		{total: 100, page: 1, limit: 100, wantHasMore: false},
		{total: 0, page: 1, limit: 100, wantHasMore: false},
		{total: 101, page: 1, limit: 100, wantHasMore: true},
	}
	for _, c := range cases {
		got := newPageResult([]Word{}, c.total, c.page, c.limit)
		if got.HasMore != c.wantHasMore {
			t.Fatalf("total=%d page=%d limit=%d: HasMore = %v, want %v", c.total, c.page, c.limit, got.HasMore, c.wantHasMore)
		}
	}
}
