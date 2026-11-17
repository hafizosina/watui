package ui

import (
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"no links here", nil},
		{"see https://example.com.", []string{"https://example.com"}},
		{"(https://example.com/a)", []string{"https://example.com/a"}},
		{"https://en.wikipedia.org/wiki/Go_(language)", []string{"https://en.wikipedia.org/wiki/Go_(language)"}},
		{"go to www.example.com, now", []string{"https://www.example.com"}},
		{"http://a.io/x?y=1&z=2 and http://a.io/x?y=1&z=2 and HTTPS://B.io", []string{"http://a.io/x?y=1&z=2", "HTTPS://B.io"}},
		{"*https://bold.com*", []string{"https://bold.com"}},
		{"ftp://nope.com javascript:alert(1)", nil},
	}
	for _, c := range cases {
		if got := extractLinks(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("extractLinks(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
