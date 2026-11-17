package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hafizosina/watui/internal/store"
)

// Search in the open chat: / in the messages pane, Enter jumps to the nearest
// match above the cursor (older), n goes further back, N comes forward again,
// esc clears it. The whole history is searched, not only what is loaded.

func (m *Model) startMsgSearch() {
	m.mode = modeMsgSearch
	m.prompt.SetValue(m.msgSearch)
	m.prompt.CursorEnd()
	m.prompt.Focus()
}

func (m Model) msgSearchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c", "ctrl+[":
		m.enterNormal()
		return m, nil
	case "enter":
		query := strings.TrimSpace(m.prompt.Value())
		m.enterNormal()
		m.focus = focusChat
		m.runMsgSearch(query)
		return m, nil
	case "backspace":
		if m.prompt.Value() == "" {
			m.enterNormal()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(k)
	return m, cmd
}

func (m *Model) runMsgSearch(query string) {
	if query == "" {
		m.clearMsgSearch()
		return
	}
	hits, err := m.wa.Store.SearchMessages(m.openJID, query, m.cfg.ShowDeleted)
	if err != nil {
		m.setFlash("search: "+err.Error(), true)
		return
	}
	m.msgSearch, m.msgHits, m.msgHit = query, hits, -1
	m.renderMessages(false)
	if len(hits) == 0 {
		m.setFlash("no messages match "+query, true)
		return
	}
	// nearest match at or above the selected message (the selected one itself
	// doesn't count, so pressing Enter again moves on)
	cur, ok := m.selected()
	start := len(hits) - 1
	if ok {
		start = -1
		for i, h := range hits {
			if h.ID != cur.ID && !h.TS.After(cur.TS) {
				start = i
			}
		}
		if start < 0 {
			start = len(hits) - 1
		}
	}
	m.gotoHit(start)
}

// stepHit moves to an older (n) or newer (N) match, wrapping around.
func (m *Model) stepHit(older bool) {
	if len(m.msgHits) == 0 {
		return
	}
	i := m.msgHit + 1
	if older {
		i = m.msgHit - 1
	}
	switch {
	case i < 0:
		i = len(m.msgHits) - 1
		m.setFlash("search went past the oldest match — back to the newest", false)
	case i >= len(m.msgHits):
		i = 0
		m.setFlash("search went past the newest match — back to the oldest", false)
	}
	m.gotoHit(i)
}

func (m *Model) gotoHit(i int) {
	h := m.msgHits[i]
	m.msgHit = i
	m.ensureLoaded(h)
	m.jumpToMessage(h.ID)
}

// ensureLoaded loads enough history (in one go) to have the hit on screen.
func (m *Model) ensureLoaded(h store.Hit) {
	for _, msg := range m.messages {
		if msg.ID == h.ID {
			return
		}
	}
	n, err := m.wa.Store.CountSince(m.openJID, h.TS)
	if err != nil {
		return
	}
	if want := n + 20; want > m.loaded {
		m.loaded = want
		if msgs, err := m.wa.Store.Messages(m.openJID, m.loaded); err == nil {
			m.messages = msgs
		}
	}
}

func (m *Model) clearMsgSearch() {
	if m.msgSearch == "" {
		return
	}
	m.msgSearch, m.msgHits, m.msgHit = "", nil, -1
	m.renderMessages(false)
}

// searchTitle is " · /query 3 of 12" for the Messages box title.
func (m Model) searchTitle() string {
	if m.msgSearch == "" {
		return ""
	}
	if len(m.msgHits) == 0 {
		return " · /" + sanitize(m.msgSearch) + " no matches"
	}
	// counted from the newest match, since search goes up the chat
	return fmt.Sprintf(" · /%s %d of %d", sanitize(m.msgSearch), len(m.msgHits)-m.msgHit, len(m.msgHits))
}

// matchSpans finds the case-insensitive occurrences of query in line, as
// byte ranges, without overlaps.
func matchSpans(line, query string) [][2]int {
	if query == "" {
		return nil
	}
	n := utf8.RuneCountInString(query)
	var out [][2]int
	for i := 0; i < len(line); {
		end, count := i, 0
		for end < len(line) && count < n {
			_, size := utf8.DecodeRuneInString(line[end:])
			end += size
			count++
		}
		if count == n && strings.EqualFold(line[i:end], query) {
			out = append(out, [2]int{i, end})
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
	}
	return out
}
