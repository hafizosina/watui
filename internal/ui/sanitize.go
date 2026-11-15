package ui

import (
	"strings"
	"unicode"
)

func sanitize(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case r == '‍':
			b.WriteRune(r)
		case r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0:
		case unicode.Is(unicode.Cf, r):
		case unicode.Is(unicode.Co, r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
