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
