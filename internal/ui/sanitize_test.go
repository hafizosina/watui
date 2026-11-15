package ui

import "testing"

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
