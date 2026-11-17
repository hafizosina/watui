package ui

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"a\tb":               "a    b",
		"line\r\nnext":       "line\nnext",
		"\x1b[31mred\x1b[0m": "[31mred[0m",
		"word⁠joiner":        "wordjoiner",
		"⁨isolate⁩":          "isolate",
		"ayah۝1":             "ayah1",
		"zero​width":         "zerowidth",
		"\ufeffbom":          "bom",
		"👨‍👩‍👧":              "👨‍👩‍👧",
		"اللَّهُ":            "اللَّهُ",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUntrustedTextIsSanitized(t *testing.T) {
	evil := "\x1b]0;pwned\x07\x1b[2J👍"
	if got := reactionLine(map[string]string{"a": evil}); strings.Contains(got, "\x1b]") || strings.Contains(got, "\x1b[2J") {
		t.Errorf("reaction escape reached the screen: %q", got)
	}
	if got := reactionLine(map[string]string{"a": strings.Repeat("x", 500)}); len(got) > 200 {
		t.Errorf("reaction should be capped, got %d bytes", len(got))
	}
	m := Model{}
	m.setFlash("saved \x1b]52;c;ZXZpbA==\x07file.pdf\nnext", false)
	if strings.ContainsAny(m.flash, "\x1b\x07\n") {
		t.Errorf("flash kept control characters: %q", m.flash)
	}
}
