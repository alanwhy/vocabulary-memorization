package main

import "testing"

func TestFormatSenses(t *testing.T) {
	senses := []Sense{{Pos: "n.", Translation: "苹果"}, {Pos: "v.", Translation: "投掷"}}
	got := formatSenses(senses)
	want := "n. 苹果；v. 投掷"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatSensesWithoutPos(t *testing.T) {
	senses := []Sense{{Translation: "释义"}}
	got := formatSenses(senses)
	want := "释义"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatSensesEmpty(t *testing.T) {
	if got := formatSenses(nil); got != "" {
		t.Fatalf("got %q want empty string", got)
	}
}
