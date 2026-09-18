package storage

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

// 未知排序值必须退回默认排序，而不是把请求参数拼进 SQL。
func TestWordOrderByRejectsUnknownSort(t *testing.T) {
	fallback := wordOrderBy("count")
	for _, sort := range []string{"", "id", "review_count", "1; DROP TABLE words"} {
		if got := wordOrderBy(sort); got != fallback {
			t.Fatalf("wordOrderBy(%q) = %q, want fallback %q", sort, got, fallback)
		}
	}
}

// 每种排序都必须以唯一列 id 收尾，否则 LIMIT/OFFSET 翻页会跨页重复或漏行。
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
	for input, want := range cases {
		if got := escapeLikePattern(input); got != want {
			t.Fatalf("escapeLikePattern(%q) = %q, want %q", input, got, want)
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
	if conditions, args := senseFilterWhere("", ""); conditions != "" || args != nil {
		t.Fatalf("senseFilterWhere(\"\", \"\") = (%q, %v), want (\"\", nil)", conditions, args)
	}

	conditions, args := senseFilterWhere("apple", "")
	if !strings.Contains(conditions, "word_key LIKE ?") || !strings.Contains(conditions, "JSON_SEARCH") {
		t.Fatalf("keyword filter missing LIKE/JSON_SEARCH: %q", conditions)
	}
	if len(args) != 2 {
		t.Fatalf("keyword filter args = %d, want 2", len(args))
	}

	if conditions, args := senseFilterWhere("", "no_definition"); !strings.Contains(conditions, "JSON_LENGTH(senses) = 0") || len(args) != 0 {
		t.Fatalf("no_definition filter = (%q, %v)", conditions, args)
	}
	if conditions, args := senseFilterWhere("", "has_definition"); !strings.Contains(conditions, "JSON_LENGTH(senses) > 0") || len(args) != 0 {
		t.Fatalf("has_definition filter = (%q, %v)", conditions, args)
	}

	if conditions, _ := senseFilterWhere("", "bogus"); conditions != "" {
		t.Fatalf("bogus status should not filter, got %q", conditions)
	}

	conditions, args = senseFilterWhere("apple", "no_definition")
	if !strings.Contains(conditions, " AND ") || len(args) != 2 {
		t.Fatalf("combined filter = (%q, %v)", conditions, args)
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
