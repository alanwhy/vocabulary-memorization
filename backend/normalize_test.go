package main

import (
	"strings"
	"testing"
)

func TestNormalizeGlossesEmptyAndWhitespace(t *testing.T) {
	if got := normalizeGlosses(nil); len(got) != 0 {
		t.Fatalf("nil 输入应返回空数组，got %v", got)
	}
	if got := normalizeGlosses([]string{"", "  ", "　"}); len(got) != 0 {
		t.Fatalf("全空串输入应返回空数组，got %v", got)
	}
}

func TestNormalizeGlossesTrimsAndDedupes(t *testing.T) {
	got := normalizeGlosses([]string{" 跑 ", "", "运行", "跑", " 运行 "})
	want := []string{"跑", "运行"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestNormalizeGlossesCapsSingleLength(t *testing.T) {
	long := strings.Repeat("长", 250)
	got := normalizeGlosses([]string{long})
	if len(got) != 1 || len([]rune(got[0])) != 200 {
		t.Fatalf("单条超长应截断到 200 字，got len=%d", len([]rune(got[0])))
	}
}

func TestNormalizeGlossesCapsCount(t *testing.T) {
	in := make([]string, 25)
	for i := range in {
		in[i] = strings.Repeat(string(rune('a'+i)), 3)
	}
	got := normalizeGlosses(in)
	if len(got) != 20 {
		t.Fatalf("总数应截断到 20，got %d", len(got))
	}
}
