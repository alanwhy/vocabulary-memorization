package main

import (
	"net/http/httptest"
	"testing"
)

func TestParsePaginationDefaults(t *testing.T) {
	page, limit, offset := parsePagination(httptest.NewRequest("GET", "/api/words", nil))
	if page != 1 || limit != defaultPageLimit || offset != 0 {
		t.Fatalf("expected (1, %d, 0), got (%d, %d, %d)", defaultPageLimit, page, limit, offset)
	}
}

func TestParsePaginationComputesOffset(t *testing.T) {
	page, limit, offset := parsePagination(httptest.NewRequest("GET", "/api/words?page=3&limit=50", nil))
	if page != 3 || limit != 50 || offset != 100 {
		t.Fatalf("expected (3, 50, 100), got (%d, %d, %d)", page, limit, offset)
	}
}

func TestParsePaginationClampsInvalidValues(t *testing.T) {
	cases := []struct {
		query           string
		wantPage        int
		wantLimit       int
		wantOffset      int
		describeInvalid string
	}{
		{"?page=0&limit=10", 1, 10, 0, "page 小于 1 时退回第一页"},
		{"?page=-5", 1, defaultPageLimit, 0, "负页码退回第一页"},
		{"?page=abc&limit=xyz", 1, defaultPageLimit, 0, "非数字参数走默认值"},
		{"?limit=0", 1, defaultPageLimit, 0, "limit 为 0 时走默认值"},
		{"?limit=9999", 1, maxPageLimit, 0, "limit 超上限时夹到 maxPageLimit"},
	}
	for _, c := range cases {
		page, limit, offset := parsePagination(httptest.NewRequest("GET", "/api/words"+c.query, nil))
		if page != c.wantPage || limit != c.wantLimit || offset != c.wantOffset {
			t.Fatalf("%s: %s expected (%d, %d, %d), got (%d, %d, %d)",
				c.describeInvalid, c.query, c.wantPage, c.wantLimit, c.wantOffset, page, limit, offset)
		}
	}
}

func TestParseIDListSkipsInvalidAndDedupes(t *testing.T) {
	got := parseIDList(" 3, 1 ,abc,3,0,-2,2,", 10)
	want := []int{3, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseIDListEmptyInput(t *testing.T) {
	if ids := parseIDList("", 10); ids != nil {
		t.Fatalf("expected nil for empty input, got %v", ids)
	}
	if ids := parseIDList("abc,-1", 10); len(ids) != 0 {
		t.Fatalf("expected no ids for all-invalid input, got %v", ids)
	}
}

func TestParseIDListCapsLength(t *testing.T) {
	if ids := parseIDList("1,2,3,4,5", 3); len(ids) != 3 {
		t.Fatalf("expected list capped at 3, got %v", ids)
	}
}

func TestWordPatternValidWords(t *testing.T) {
	valid := []string{"apple", "well-known", "don't", "New York", "a"}
	for _, w := range valid {
		if !wordPattern.MatchString(w) {
			t.Errorf("expected %q to be valid", w)
		}
	}
}

func TestWordPatternInvalidWords(t *testing.T) {
	invalid := []string{"", "123abc", "-apple", "apple!", "苹果"}
	for _, w := range invalid {
		if wordPattern.MatchString(w) {
			t.Errorf("expected %q to be invalid", w)
		}
	}
}

func TestWordPatternRejectsOverlongInput(t *testing.T) {
	long := "a"
	for i := 0; i < 64; i++ {
		long += "a"
	}
	if wordPattern.MatchString(long) {
		t.Errorf("expected 65-char input to be rejected")
	}
}
