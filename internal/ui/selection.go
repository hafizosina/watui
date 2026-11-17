package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

// Multi-select: v toggles the selected message, V starts a range that follows
// the cursor (V again keeps it). Space then opens the bulk menu.

func (m Model) hasSelection() bool {
	return len(m.marked) > 0 || m.rangeFrom != ""
}

// isMarked reports whether message i is part of the selection.
func (m Model) isMarked(i int) bool {
	if m.marked[m.messages[i].ID] {
		return true
	}
	if m.rangeFrom == "" {
		return false
	}
	a := m.indexOf(m.rangeFrom)
	if a < 0 {
		return false
	}
	lo, hi := min(a, m.sel), max(a, m.sel)
	return i >= lo && i <= hi
}

func (m Model) indexOf(id string) int {
	for i, msg := range m.messages {
		if msg.ID == id {
			return i
		}
	}
	return -1
}

// selection returns the selected messages, oldest first.
func (m Model) selection() []store.Message {
	var out []store.Message
	seen := map[string]bool{}
	for i, msg := range m.messages {
		if m.isMarked(i) {
			out = append(out, msg)
			seen[msg.ID] = true
		}
	}
	// marks on messages that scrolled out of the loaded window still count
	if len(seen) < len(m.marked) {
		for id := range m.marked {
			if !seen[id] {
				if msg, ok := m.markedMsgs[id]; ok {
					out = append(out, msg)
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	}
	return out
}

func (m *Model) toggleMark() {
	if m.sel < 0 || m.sel >= len(m.messages) {
		return
	}
	if m.rangeFrom != "" {
		m.commitRange()
		m.renderMessages(false)
		return
	}
	msg := m.messages[m.sel]
	if m.marked[msg.ID] {
		delete(m.marked, msg.ID)
		delete(m.markedMsgs, msg.ID)
	} else {
		m.marked[msg.ID] = true
		m.markedMsgs[msg.ID] = msg
	}
	m.renderMessages(false)
}

func (m *Model) toggleRange() {
	if m.sel < 0 || m.sel >= len(m.messages) {
		return
	}
	if m.rangeFrom != "" {
		m.commitRange()
	} else {
		m.rangeFrom = m.messages[m.sel].ID
	}
	m.renderMessages(false)
}

func (m *Model) commitRange() {
	for i, msg := range m.messages {
		if m.isMarked(i) {
			m.marked[msg.ID] = true
			m.markedMsgs[msg.ID] = msg
		}
	}
	m.rangeFrom = ""
}

func (m *Model) clearSelection() {
	if !m.hasSelection() {
		return
	}
	m.marked = map[string]bool{}
	m.markedMsgs = map[string]store.Message{}
	m.rangeFrom = ""
	m.renderMessages(false)
}

func (m Model) senderName(msg store.Message) string {
	switch {
	case msg.FromMe:
		return "You"
	case !strings.HasSuffix(msg.ChatJID, "@g.us"):
		return m.names[msg.ChatJID]
	}
	return sanitize(m.wa.ContactName(msg.SenderJID, msg.SenderName))
}

// selectionText formats messages the way WhatsApp copies several at once.
func (m Model) selectionText(msgs []store.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		text := msg.Text
		if msg.Deleted {
			text = "[deleted]"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", msg.TS.Local().Format("15:04, 02/01/2006"), m.senderName(msg), sanitize(m.withNames(text)))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (m *Model) openSelectionMenu() {
	if m.rangeFrom != "" {
		m.commitRange()
	}
	msgs := m.selection()
	if len(msgs) == 0 {
		return
	}
	n := len(msgs)
	var forwardable, media, revocable, starred int
	for _, msg := range msgs {
		if wa.CanForward(msg) {
			forwardable++
		}
		if msg.MediaType != "" && !m.gone(msg) {
			media++
		}
		if wa.CanRevoke(msg) {
			revocable++
		}
		if msg.Starred {
			starred++
		}
	}
	items := []menuItem{{label: "Copy text", key: "y", run: func(m *Model) tea.Cmd {
		m.clearSelection()
		return copyCmd(m.selectionText(msgs))
	}}}
	if forwardable > 0 {
		label := "Forward…"
		if forwardable < n {
			label = fmt.Sprintf("Forward %d of %d…", forwardable, n)
		}
		items = append(items, menuItem{label: label, key: "f", run: func(m *Model) tea.Cmd {
			m.openBulkForwardMenu(msgs)
			return nil
		}})
	}
	if starred < n {
		items = append(items, menuItem{label: "Star", key: "s", run: func(m *Model) tea.Cmd {
			client := m.wa
			return m.bulkCmd(msgs, "starred", func(ctx context.Context, msg store.Message) error {
				if msg.Starred || msg.Deleted {
					return nil
				}
				return client.Star(ctx, msg, true)
			})
		}})
	}
	if starred > 0 {
		items = append(items, menuItem{label: "Unstar", key: "u", run: func(m *Model) tea.Cmd {
			client := m.wa
			return m.bulkCmd(msgs, "unstarred", func(ctx context.Context, msg store.Message) error {
				if !msg.Starred {
					return nil
				}
				return client.Star(ctx, msg, false)
			})
		}})
	}
	if media > 0 {
		items = append(items, menuItem{label: fmt.Sprintf("Save %d media to ~/Downloads/WhatsApp", media), key: "S", run: func(m *Model) tea.Cmd {
			client := m.wa
			var picked []store.Message
			for _, msg := range msgs {
				if msg.MediaType != "" && !m.gone(msg) {
					picked = append(picked, msg)
				}
			}
			m.setFlash(fmt.Sprintf("saving %d files…", len(picked)), false)
			return m.bulkCmd(picked, "saved to ~/Downloads/WhatsApp", func(ctx context.Context, msg store.Message) error {
				_, err := client.SaveMedia(ctx, msg.ChatJID, msg.ID, msg.TS.Local().Format("20060102-150405"))
				return err
			})
		}})
	}
	if revocable == n {
		items = append(items, menuItem{label: "Delete for everyone", key: "d", danger: true, run: func(m *Model) tea.Cmd {
			m.menu = &menu{title: fmt.Sprintf("Delete %d messages for everyone?", n), items: []menuItem{
				{label: "Cancel", key: "n", run: func(m *Model) tea.Cmd { return nil }},
				{label: "Yes, delete them", key: "y", danger: true, run: func(m *Model) tea.Cmd {
					client := m.wa
					return m.bulkCmd(msgs, "deleted", func(ctx context.Context, msg store.Message) error { return client.Revoke(ctx, msg) })
				}},
			}}
			return nil
		}})
	}
	items = append(items, menuItem{label: "Clear selection", key: "x", run: func(m *Model) tea.Cmd {
		m.clearSelection()
		return nil
	}})
	m.menu = &menu{title: fmt.Sprintf("%d selected", n), items: items}
}

func (m *Model) openBulkForwardMenu(msgs []store.Message) {
	var fwd []store.Message
	for _, msg := range msgs {
		if wa.CanForward(msg) {
			fwd = append(fwd, msg)
		}
	}
	chats := append([]store.Chat(nil), m.allChats...)
	sort.SliceStable(chats, func(i, j int) bool { return chats[i].LastTS.After(chats[j].LastTS) })
	var items []menuItem
	for _, c := range chats {
		name := m.names[c.JID]
		items = append(items, menuItem{label: name, run: func(m *Model) tea.Cmd {
			client := m.wa
			m.setFlash(fmt.Sprintf("forwarding %d to %s…", len(fwd), name), false)
			return m.bulkCmd(fwd, "forwarded to "+name, func(ctx context.Context, msg store.Message) error {
				return client.Forward(ctx, msg, c.JID)
			})
		}})
	}
	m.menu = &menu{title: fmt.Sprintf("Forward %d to… (type to search)", len(fwd)), items: items, filterable: true}
}

// bulkCmd clears the selection and runs fn on each message in order, in the
// background; it reports how many succeeded and the first error.
func (m *Model) bulkCmd(msgs []store.Message, done string, fn func(ctx context.Context, msg store.Message) error) tea.Cmd {
	m.clearSelection()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		var failed int
		var first error
		for _, msg := range msgs {
			if err := fn(ctx, msg); err != nil {
				failed++
				if first == nil {
					first = err
				}
			}
		}
		if failed > 0 {
			return actionDoneMsg{err: fmt.Errorf("%d of %d failed: %w", failed, len(msgs), first)}
		}
		if len(msgs) == 0 {
			return actionDoneMsg{err: errors.New("nothing to do")}
		}
		return actionDoneMsg{info: fmt.Sprintf("%d %s", len(msgs), done)}
	}
}
