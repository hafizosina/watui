package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhenzhu/watui/internal/wa"
)

type newChatMsg struct {
	jid  string
	name string
	err  error
}

func looksLikePhone(s string) bool {
	digits := 0
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digits++
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return false
		}
	}
	return digits >= 8
}

func contactLabel(c wa.Contact) string {
	if c.Name == "" {
		return c.Phone
	}
	return sanitize(c.Name) + "  " + c.Phone
}

func (m *Model) openNewChat() {
	contacts, err := m.wa.Contacts(context.Background())
	if err != nil {
		m.setFlash("contacts: "+err.Error(), true)
		return
	}
	items := []menuItem{{label: "+ New group…", run: func(m *Model) tea.Cmd {
		m.openGroupPicker(map[string]bool{}, nil, "", 0)
		return nil
	}}}
	for _, c := range contacts {
		c := c
		items = append(items, menuItem{label: contactLabel(c), run: func(m *Model) tea.Cmd {
			m.startChatWith(c.JID, c.Name)
			return nil
		}})
	}
	client := m.wa
	m.menu = &menu{
		title:      fmt.Sprintf("New chat · %d contacts · type a name or number", len(contacts)),
		items:      items,
		filterable: true,
		dynamic: func(filter string) []menuItem {
			if !looksLikePhone(filter) {
				return nil
			}
			digits := client.PhoneDigits(filter)
			return []menuItem{{label: "Chat with +" + digits, run: func(m *Model) tea.Cmd {
				m.setFlash("checking +"+digits+" on WhatsApp…", false)
				return func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					jid, err := client.CheckNumber(ctx, digits)
					return newChatMsg{jid: jid, err: err}
				}
			}}}
		},
	}
}

func (m *Model) openGroupPicker(selected map[string]bool, order []string, filter string, cursor int) {
	contacts, err := m.wa.Contacts(context.Background())
	if err != nil {
		m.setFlash("contacts: "+err.Error(), true)
		return
	}
	mn := &menu{filterable: true, filter: filter, cursor: cursor}
	reopen := func(m *Model) {
		m.openGroupPicker(selected, order, mn.filter, mn.cursor)
	}
	if len(order) > 0 {
		mn.items = append(mn.items, menuItem{label: fmt.Sprintf("✓ Next: name the group (%d selected)", len(order)), run: func(m *Model) tea.Cmd {
			m.pendingGroup = append([]string(nil), order...)
			m.mode = modeCommand
			m.prompt.SetValue("group ")
			m.prompt.CursorEnd()
			m.prompt.Focus()
			m.setFlash("type the group name (max 25 characters), Enter to create", false)
			return nil
		}})
	}
	for _, c := range contacts {
		c := c
		box := "[ ] "
		if selected[c.JID] {
			box = "[x] "
		}
		mn.items = append(mn.items, menuItem{label: box + contactLabel(c), run: func(m *Model) tea.Cmd {
			if selected[c.JID] {
				delete(selected, c.JID)
				for i, j := range order {
					if j == c.JID {
						order = append(order[:i:i], order[i+1:]...)
						break
					}
				}
			} else {
				selected[c.JID] = true
				order = append(order, c.JID)
			}
			reopen(m)
			return nil
		}})
	}
	names := make([]string, 0, len(order))
	byJID := map[string]string{}
	for _, c := range contacts {
		byJID[c.JID] = c.Name
	}
	for _, j := range order {
		n := byJID[j]
		if n == "" {
			n = "+" + strings.Split(j, "@")[0]
		}
		names = append(names, sanitize(n))
	}
	mn.title = "New group · enter toggles · type to search"
	if len(names) > 0 {
		mn.title = "New group · " + strings.Join(names, ", ")
	}
	m.menu = mn
}

func (m Model) createGroupCmd(name string) tea.Cmd {
	members := m.pendingGroup
	client := m.wa
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		jid, err := client.CreateGroup(ctx, name, members)
		return newChatMsg{jid: jid, name: name, err: err}
	}
}

func (m *Model) startChatWith(jid, name string) {
	if err := m.wa.StartChat(jid, name); err != nil {
		m.setFlash(err.Error(), true)
		return
	}
	m.showArchived, m.filter = false, ""
	m.refresh()
	for i, c := range m.chats {
		if c.JID == jid {
			m.cursor = i
			if c.Archived {
				m.showArchived = true
			}
		}
	}
	m.clampCursor()
	m.open(jid)
	m.enterInsert()
}

func (m Model) newChatReady(msg newChatMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setFlash(msg.err.Error(), true)
		return m, nil
	}
	m.pendingGroup = nil
	m.startChatWith(msg.jid, msg.name)
	if strings.HasSuffix(msg.jid, "@g.us") {
		m.setFlash("group created", false)
	}
	return m, nil
}
