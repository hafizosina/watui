package ui

import (
	"strings"
	"unicode"

	"github.com/hafizosina/watui/internal/store"
)

// sanitize makes text safe and width-predictable for the screen. Zero-width
// joiners go too (they're Cf): tmux draws "👩‍👩‍👧‍👦" as four 2-wide emoji while
// lipgloss counts one, so a joined emoji pushed rows past their box border.
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
		case r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0:
		case unicode.Is(unicode.Cf, r):
		case unicode.Is(unicode.Co, r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// withNames shows "@<id>" mentions as "@<name>", like the phone does.
func (m Model) withNames(s string) string {
	if m.wa == nil {
		return s
	}
	return m.wa.ResolveMentions(s)
}

// withMentionLabels is withNames, also returning the "@Name" labels to highlight.
func (m Model) withMentionLabels(s string) (string, []string) {
	if m.wa == nil {
		return s, nil
	}
	text, labels := m.wa.ResolveMentionLabels(s)
	for i := range labels {
		labels[i] = sanitize(labels[i])
	}
	return text, labels
}

// gone: a deleted message shown only as "message deleted" (show_deleted off, or nothing was kept).
func (m Model) gone(msg store.Message) bool {
	return msg.Deleted && (!m.cfg.ShowDeleted || msg.Text == "" && msg.MediaType == "")
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
