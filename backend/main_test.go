package main

import "testing"

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
