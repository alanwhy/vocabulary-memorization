package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestWordOrderByWhitelist(t *testing.T) {
	cases := map[string]string{
		"time":  `last_reviewed_at DESC, id DESC`,
		"alpha": `word_key ASC, id ASC`,
		"count": `review_count DESC, last_reviewed_at DESC, id DESC`,
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
	for _, sort := range []string{"count", "time", "alpha", "unknown"} {
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
