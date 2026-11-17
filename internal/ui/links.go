package ui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/desktop"
)

var linkRe = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"]+`)

// extractLinks returns the http(s) links in text, in order, without duplicates.
// Bare "www." links get an https:// prefix; trailing punctuation that usually
// ends a sentence (and an unbalanced closing bracket) is dropped.
func extractLinks(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range linkRe.FindAllString(sanitize(text), -1) {
		link := trimLink(raw)
		if strings.HasPrefix(strings.ToLower(link), "www.") {
			link = "https://" + link
		}
		lower := strings.ToLower(link)
		if lower == "https://www." || lower == "http://" || lower == "https://" || seen[link] {
			continue
		}
		seen[link] = true
		out = append(out, link)
	}
	return out
}

func trimLink(s string) string {
	for s != "" {
		last := s[len(s)-1]
		switch {
		case strings.IndexByte(".,;:!?'*_~", last) >= 0:
			s = s[:len(s)-1]
		case last == ')' && strings.Count(s, "(") < strings.Count(s, ")"),
			last == ']' && strings.Count(s, "[") < strings.Count(s, "]"),
			last == '}' && strings.Count(s, "{") < strings.Count(s, "}"):
			s = s[:len(s)-1]
		default:
			return s
		}
	}
	return s
}

func openLinkCmd(link string) tea.Cmd {
	return func() tea.Msg {
		if err := desktop.Open(link); err != nil {
			return actionDoneMsg{err: fmt.Errorf("open link: %w", err)}
		}
		return actionDoneMsg{info: "opened " + ansi.Truncate(link, 60, "…")}
	}
}

func (m *Model) openLinksMenu(links []string) {
	var items []menuItem
	for i, link := range links {
		it := menuItem{label: link, run: func(m *Model) tea.Cmd { return openLinkCmd(link) }}
		if i < 9 {
			it.key = string(rune('1' + i))
		}
		items = append(items, it)
	}
	m.menu = &menu{title: "Open link", items: items}
}
