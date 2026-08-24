package model

import "testing"

func TestNormalizeSensitiveWord(t *testing.T) {
	if _, err := normalizeSensitiveWord("  禁词  "); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeSensitiveWord(""); err == nil {
		t.Fatal("expected empty word error")
	}
}

func TestTruncateSensitivePrompt(t *testing.T) {
	if got := truncateSensitivePrompt("abcdef", 3); got != "abc…" {
		t.Fatalf("got %q", got)
	}
	if got := truncateSensitivePrompt("a\x00b", 10); got != "ab" {
		t.Fatalf("got %q", got)
	}
}
