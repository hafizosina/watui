package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hafizosina/watui/internal/store"
)

func (m *Model) openChatMenu() {
	c, ok := m.currentChat()
	if !ok {
		return
	}
	name := m.names[c.JID]
	client := m.wa
	jid := c.JID
	items := []menuItem{{label: "Open", key: "o", run: func(m *Model) tea.Cmd {
		m.open(jid)
		m.focus = focusChat
		m.renderMessages(true)
		return m.markReadCmd()
	}}}

	if c.IsGroup {
		items = append(items, menuItem{label: "Group info", key: "i", run: func(m *Model) tea.Cmd {
			return m.groupInfoCmd(jid, name)
		}})
	} else {
		items = append(items, menuItem{label: "Contact info", key: "i", run: func(m *Model) tea.Cmd {
			return m.personInfoCmd(jid, name)
		}})
	}

	pinLabel, pinned := "Pin", !c.Pinned
	if c.Pinned {
		pinLabel = "Unpin"
	}
	items = append(items, menuItem{label: pinLabel, key: "p", run: func(m *Model) tea.Cmd {
		return m.actionCmd(strings.ToLower(pinLabel)+"ned "+name, func(ctx context.Context) error { return client.SetPinned(ctx, jid, pinned) })
	}})

	if c.Muted() {
		items = append(items, menuItem{label: "Unmute", key: "m", run: func(m *Model) tea.Cmd {
			return m.actionCmd("unmuted "+name, func(ctx context.Context) error { return client.SetMuted(ctx, jid, false) })
		}})
	} else {
		items = append(items, menuItem{label: "Mute…", key: "m", run: func(m *Model) tea.Cmd {
			mute := func(label string, d time.Duration, key string) menuItem {
				return menuItem{label: label, key: key, run: func(m *Model) tea.Cmd {
					return m.actionCmd("muted "+name+" ("+label+")", func(ctx context.Context) error { return client.MuteFor(ctx, jid, d) })
				}}
			}
			m.menu = &menu{title: "Mute " + name, items: []menuItem{
				mute("8 hours", 8*time.Hour, "1"),
				mute("1 week", 7*24*time.Hour, "2"),
				mute("Always", 0, "3"),
			}}
			return nil
		}})
	}

	archLabel, archive := "Archive", !c.Archived
	if c.Archived {
		archLabel = "Unarchive"
	}
	items = append(items, menuItem{label: archLabel, key: "a", run: func(m *Model) tea.Cmd {
		return m.actionCmd(strings.ToLower(archLabel)+"d "+name, func(ctx context.Context) error { return client.SetArchived(ctx, jid, archive) })
	}})

	if c.Unread > 0 {
		items = append(items, menuItem{label: "Mark as read", key: "r", run: func(m *Model) tea.Cmd {
			return m.actionCmd("marked "+name+" as read", func(ctx context.Context) error { return client.MarkRead(ctx, jid) })
		}})
	} else {
		items = append(items, menuItem{label: "Mark as unread", key: "u", run: func(m *Model) tea.Cmd {
			return m.actionCmd("marked "+name+" as unread", func(ctx context.Context) error { return client.MarkUnread(ctx, jid) })
		}})
	}

	if !c.IsGroup && strings.HasSuffix(jid, "@s.whatsapp.net") {
		number := "+" + strings.TrimSuffix(jid, "@s.whatsapp.net")
		items = append(items, menuItem{label: "Copy number " + number, key: "y", run: func(m *Model) tea.Cmd {
			return copyCmd(number)
		}})
	}

	if c.IsGroup {
		items = append(items, menuItem{label: "Leave group", key: "L", danger: true, run: func(m *Model) tea.Cmd {
			m.confirm("Leave "+name+"?", "Yes, leave the group", func(m *Model) tea.Cmd {
				return m.actionCmd("left "+name, func(ctx context.Context) error { return client.LeaveGroup(ctx, jid) })
			})
			return nil
		}})
	}

	items = append(items, menuItem{label: "Delete chat", key: "d", danger: true, run: func(m *Model) tea.Cmd {
		m.confirm("Delete chat with "+name+"? (also on your phone)", "Yes, delete it", func(m *Model) tea.Cmd {
			if m.openJID == jid {
				m.openJID, m.messages = "", nil
			}
			return m.actionCmd("deleted chat with "+name, func(ctx context.Context) error { return client.DeleteChat(ctx, jid) })
		})
		return nil
	}})

	m.menu = &menu{title: chatMenuTitle(c, name), items: items}
}

func chatMenuTitle(c store.Chat, name string) string {
	title := "Chat · " + name
	if c.IsGroup {
		title = "Group · " + name
	}
	return title
}

func (m *Model) confirm(title, yes string, run func(m *Model) tea.Cmd) {
	m.menu = &menu{title: title, items: []menuItem{
		{label: "Cancel", key: "n", run: func(m *Model) tea.Cmd { return nil }},
		{label: yes, key: "y", danger: true, run: run},
	}}
}
