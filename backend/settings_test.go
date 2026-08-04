package main

import "testing"

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		"":           "****",
		"short":      "****",
		"abcdefgh":   "****",
		"abcdefghij": "abcd****ghij",
	}
	for in, want := range cases {
		if got := maskAPIKey(in); got != want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}
