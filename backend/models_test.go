package main

import (
	"reflect"
	"testing"
)

func TestMergeSensesByPos(t *testing.T) {
	input := []Sense{
		{Pos: "n.", Translation: "苹果"},
		{Pos: "v.", Translation: "投掷"},
		{Pos: "n.", Translation: "水果"},
		{Pos: "n.", Translation: "苹果"}, // 重复，应该被去重
	}
	got := mergeSensesByPos(input)
	want := []Sense{
		{Pos: "n.", Translation: "苹果；水果"},
		{Pos: "v.", Translation: "投掷"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMergeSensesByPosSkipsEmptyTranslation(t *testing.T) {
	input := []Sense{{Pos: "n.", Translation: ""}, {Pos: "n.", Translation: "有效"}}
	got := mergeSensesByPos(input)
	want := []Sense{{Pos: "n.", Translation: "有效"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMergeSensesByPosPreservesFirstOccurrenceOrder(t *testing.T) {
	input := []Sense{
		{Pos: "v.", Translation: "跑"},
		{Pos: "n.", Translation: "跑步"},
		{Pos: "v.", Translation: "奔跑"},
	}
	got := mergeSensesByPos(input)
	if len(got) != 2 || got[0].Pos != "v." || got[1].Pos != "n." {
		t.Fatalf("expected order [v., n.], got %+v", got)
	}
}

// TestApplySRSScheduling 覆盖简化 SM-2 的三档评分与关键边界：
// 首次间隔、难度系数倍增、模糊小增、不认识重置，以及难度系数下限收敛。
func TestApplySRSScheduling(t *testing.T) {
	cases := []struct {
		name         string
		intervalDays int
		ease         float64
		rating       string
		wantInterval int
		wantEase     float64
	}{
		{"新词认识首次间隔", 0, 2.5, "good", 1, 2.5},
		{"认识按难度倍增", 1, 2.5, "good", 3, 2.5},
		{"认识连续倍增", 3, 2.5, "good", 8, 2.5},
		{"模糊小幅增长", 1, 2.5, "hard", 1, 2.35},
		{"模糊零间隔至少一天", 0, 2.5, "hard", 1, 2.35},
		{"不认识重置间隔", 5, 2.0, "again", 1, 1.8},
		{"难度下限不越界", 5, 1.3, "again", 1, 1.3},
		{"难度下限收敛", 5, 1.4, "hard", 6, 1.3},
	}
	for _, c := range cases {
		gotInterval, gotEase := applySRSScheduling(c.intervalDays, c.ease, c.rating)
		if gotInterval != c.wantInterval || gotEase != c.wantEase {
			t.Fatalf("%s: applySRSScheduling(%d, %.2f, %q) = (%d, %.2f), want (%d, %.2f)",
				c.name, c.intervalDays, c.ease, c.rating, gotInterval, gotEase, c.wantInterval, c.wantEase)
		}
	}
}

// TestSensesEnriched 覆盖「是否已用最新 prompt 强化」判定的各边界。
// 判据是 lookalikes（形近词，最后加入的字段）是否为 nil：非 nil（含空数组）算已强化，nil 算未强化。
func TestSensesEnriched(t *testing.T) {
	cases := []struct {
		name   string
		senses []Sense
		want   bool
	}{
		{"空数组", nil, false},
		{"无任何强化字段", []Sense{{Pos: "n.", Translation: "苹果"}}, false},
		{"只有音标无形近词", []Sense{{Pos: "n.", Translation: "苹果", Phonetic: "/ˈæp(ə)l/"}}, false},
		{"形近词为空数组", []Sense{{Lookalikes: []string{}}}, true},
		{"有形近词", []Sense{{Lookalikes: []string{"transit（过境）"}}}, true},
		{"多词性第一条有形近词", []Sense{{Lookalikes: []string{}}, {Pos: "v.", Translation: "预订"}}, true},
	}
	for _, c := range cases {
		if got := sensesEnriched(c.senses); got != c.want {
			t.Fatalf("%s: sensesEnriched(%+v) = %v, want %v", c.name, c.senses, got, c.want)
		}
	}
}

// TestMergeSensesByPosPreservesEnrichment 覆盖合并同词性时强化字段不丢：
// 词级字段取组内第一个非空值，example 在第一条为空时回落到第二条非空值。
func TestMergeSensesByPosPreservesEnrichment(t *testing.T) {
	input := []Sense{
		{Pos: "n.", Translation: "苹果", Phonetic: "/ˈæp(ə)l/", Example: "", Root: "", Affix: "", Synonyms: []string{"pome"}},
		{Pos: "n.", Translation: "水果", Phonetic: "", Example: "An apple a day keeps the doctor away.", Synonyms: []string{}},
	}
	got := mergeSensesByPos(input)
	if len(got) != 1 {
		t.Fatalf("期望合并成 1 条，got %+v", got)
	}
	merged := got[0]
	if merged.Translation != "苹果；水果" {
		t.Fatalf("Translation = %q, want 苹果；水果", merged.Translation)
	}
	if merged.Phonetic != "/ˈæp(ə)l/" {
		t.Fatalf("Phonetic = %q, want 保留第一个非空值", merged.Phonetic)
	}
	if merged.Example != "An apple a day keeps the doctor away." {
		t.Fatalf("Example = %q, want 第一条为空时取第二条非空值", merged.Example)
	}
	if len(merged.Synonyms) != 1 || merged.Synonyms[0] != "pome" {
		t.Fatalf("Synonyms = %v, want 保留 [pome]", merged.Synonyms)
	}
}
