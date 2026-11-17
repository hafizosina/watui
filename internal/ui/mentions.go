package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/wa"
)

type membersMsg struct {
	chat    string
	members []wa.Mentionable
	err     error
}

func isMentionRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}

// mentionQuery returns the "@word" being typed right before the cursor, if any ("@" alone counts).
func (e *editor) mentionQuery() (int, string, bool) {
	if e.mode != vimInsert {
		return 0, "", false
	}
	i := e.cur
	for i > 0 && isMentionRune(e.buf[i-1]) {
		i--
	}
	if i == 0 || e.buf[i-1] != '@' {
		return 0, "", false
	}
	at := i - 1
	if at > 0 && !unicode.IsSpace(e.buf[at-1]) {
		return 0, "", false // an email address, not a mention
	}
	return at, string(e.buf[i:e.cur]), true
}

func isGroupJID(jid string) bool { return strings.HasSuffix(jid, "@g.us") }

// mentionState: active = the popup should show (members may still be loading).
func (m Model) mentionSuggestions() (from int, query string, list []wa.Mentionable, active bool) {
	if m.mode != modeInsert || m.input.mode != vimInsert || !isGroupJID(m.openJID) || m.editing != nil {
		return 0, "", nil, false
	}
	from, query, ok := m.input.mentionQuery()
	if !ok || m.mentionHidden == mentionKey(from, query) {
		return 0, "", nil, false
	}
	members, loaded := m.members[m.openJID]
	if !loaded {
		return from, query, nil, m.membersLoading[m.openJID]
	}
	list = matchMembers(members, query, 8)
	return from, query, list, len(list) > 0
}

func mentionKey(from int, query string) string { return fmt.Sprintf("%d:%s", from, query) }

// matchMembers: names with a word starting with the query first, then names containing it, then numbers.
func matchMembers(members []wa.Mentionable, query string, limit int) []wa.Mentionable {
	q := strings.ToLower(query)
	var tiers [3][]wa.Mentionable
	for _, mb := range members {
		name := strings.ToLower(mb.Name)
		switch {
		case q == "" || strings.HasPrefix(name, q) || strings.Contains(name, " "+q):
			tiers[0] = append(tiers[0], mb)
		case strings.Contains(name, q):
			tiers[1] = append(tiers[1], mb)
		case strings.Contains(strings.TrimPrefix(mb.Phone, "+"), strings.TrimPrefix(q, "+")):
			tiers[2] = append(tiers[2], mb)
		}
	}
	out := append(append(tiers[0], tiers[1]...), tiers[2]...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// loadMembersCmd fetches the open group's members when "@" is typed there.
func (m *Model) loadMembersCmd() tea.Cmd {
	if _, _, ok := m.input.mentionQuery(); !ok || m.editing != nil {
		return nil
	}
	return m.ensureMembersCmd()
}

// membersTTL: how long a group's member list is used before it is fetched again.
const membersTTL = 5 * time.Minute

// ensureMembersCmd fetches the open group's members (for the header and @mentions)
// unless a fresh list is cached, a fetch is running, or one just failed.
func (m *Model) ensureMembersCmd() tea.Cmd {
	chat := m.openJID
	if !isGroupJID(chat) || m.wa == nil {
		return nil
	}
	if _, loaded := m.members[chat]; loaded && time.Since(m.membersAt[chat]) < membersTTL {
		return nil
	}
	if m.membersLoading[chat] || time.Since(m.membersFailed[chat]) < 30*time.Second {
		return nil
	}
	m.membersLoading[chat] = true
	client := m.wa
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		members, err := client.GroupMembers(ctx, chat)
		return membersMsg{chat: chat, members: members, err: err}
	}
}

func (m Model) gotMembers(msg membersMsg) (tea.Model, tea.Cmd) {
	delete(m.membersLoading, msg.chat)
	if msg.err != nil {
		m.membersFailed[msg.chat] = time.Now()
		if _, _, typing := m.input.mentionQuery(); typing && msg.chat == m.openJID {
			m.setFlash("group members: "+msg.err.Error(), true)
		}
		return m, nil
	}
	for i := range msg.members {
		msg.members[i].Name = sanitize(msg.members[i].Name)
	}
	m.members[msg.chat] = msg.members
	m.membersAt[msg.chat] = time.Now()
	return m, nil
}

// mentionKeyPress handles keys while the mention popup is open.
func (m Model) mentionKeyPress(k tea.KeyMsg) (tea.Model, bool) {
	from, query, list, active := m.mentionSuggestions()
	if !active {
		return m, false
	}
	switch k.String() {
	case "tab", "enter", "ctrl+y":
		if len(list) == 0 {
			return m, true
		}
		mb := list[min(m.mentionSel, len(list)-1)]
		m.input.insertAtom(from, atom{label: "@" + strings.Join(strings.Fields(mb.Name), " "), user: mb.User, jid: mb.JID})
		m.mentionSel = 0
		return m, true
	case "ctrl+n", "down":
		if len(list) > 0 {
			m.mentionSel = (m.mentionSel + 1) % len(list)
		}
		return m, true
	case "ctrl+p", "up", "shift+tab":
		if len(list) > 0 {
			m.mentionSel = (m.mentionSel - 1 + len(list)) % len(list)
		}
		return m, true
	case "esc", "ctrl+[":
		m.mentionHidden = mentionKey(from, query)
		return m, true
	}
	return m, false
}

func (m Model) mentionView(query string, list []wa.Mentionable) (string, int, int) {
	w := 44
	var rows []string
	if len(list) == 0 {
		rows = append(rows, styleDim.Render(" loading members…"))
	}
	for i, mb := range list {
		sel := i == min(m.mentionSel, len(list)-1)
		bg := lipgloss.NewStyle()
		if sel {
			bg = bg.Background(colHighlight)
		}
		phone := ""
		if mb.Phone != "" && mb.Phone != mb.Name {
			phone = "  " + mb.Phone
		}
		name := ansi.Truncate(mb.Name, w-4-lipgloss.Width(phone), "…")
		colorOf := mb.JID
		if mb.Phone != "" {
			colorOf = strings.TrimPrefix(mb.Phone, "+") + "@s.whatsapp.net" // same color as their messages
		}
		row := bg.Render(" ") + bg.Foreground(senderColor(colorOf)).Bold(sel).Render(name) + bg.Foreground(colComment).Render(phone)
		rows = append(rows, row+bg.Render(strings.Repeat(" ", max(0, w-2-lipgloss.Width(row)))))
	}
	h := len(rows) + 2
	return panel("Mention @"+query, "tab/enter pick · ^n/^p", strings.Join(rows, "\n"), w, h, true), w, h
}

// highlightMentions colors the "@Name" marks in one plain line: yours magenta, others blue.
func highlightMentions(line string, marks []string) string {
	return highlightLine(line, marks, "")
}

// highlightLine colors @mentions and, when searching, the matches of query
// (case-insensitive) in one plain line.
func highlightLine(line string, marks []string, query string) string {
	var b strings.Builder
	for line != "" {
		at, mark := -1, ""
		for _, mk := range marks {
			if j := strings.Index(line, mk); j >= 0 && (at < 0 || j < at || j == at && len(mk) > len(mark)) {
				at, mark = j, mk
			}
		}
		if spans := matchSpans(line, query); len(spans) > 0 && (at < 0 || spans[0][0] < at) {
			at = spans[0][0]
			if at > 0 {
				b.WriteString(styleText.Render(line[:at]))
			}
			b.WriteString(styleSearchHit.Render(line[at:spans[0][1]]))
			line = line[spans[0][1]:]
			continue
		}
		if at < 0 {
			b.WriteString(styleText.Render(line))
			break
		}
		if at > 0 {
			b.WriteString(styleText.Render(line[:at]))
		}
		st := styleMention
		if mark == "@You" {
			st = styleMentionMe
		}
		b.WriteString(st.Render(mark))
		line = line[at+len(mark):]
	}
	return b.String()
}
