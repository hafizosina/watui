package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"go.mau.fi/whatsmeow/types"
)

// headerH is the box above the messages that says who the open chat is with.
const headerH = 4

func (m Model) headerView(w int) string {
	if m.openJID == "" {
		return " " + styleDim.Render("no chat open") + "\n " + styleDim.Render("pick one on the left · enter opens it")
	}
	chat, _ := m.openChat()
	name := m.names[m.openJID]
	if name == "" {
		name = m.openJID
	}
	bg := lipgloss.NewStyle().Background(senderColor(m.openJID)).Foreground(colOnAccent).Bold(true)
	ini := initials(name)
	badge := bg.Render(" " + ini + strings.Repeat(" ", 3-lipgloss.Width(ini)))
	blank := bg.Render("    ")

	icons := ""
	if chat.Pinned {
		icons += " " + styleDim.Render(iconPin)
	}
	if chat.Muted() {
		icons += " " + styleMuted.Render(iconMute+" muted")
	}
	if chat.Archived {
		icons += " " + styleDim.Render("archived")
	}
	hint := styleDim.Render(":info details ")
	title := name
	if chat.IsGroup {
		title = iconGroup + " " + name
	}
	room := max(4, w-6-lipgloss.Width(icons)-lipgloss.Width(hint))
	line1 := " " + badge + " " + styleTitle.Render(ansi.Truncate(title, room, "…")) + icons
	line1 += strings.Repeat(" ", max(0, w-lipgloss.Width(line1)-lipgloss.Width(hint))) + hint

	sub := ansi.Truncate(m.headerSubtitle(chat.IsGroup), max(4, w-7), "…")
	line2 := " " + blank + " " + styleDim.Render(sub)
	return line1 + "\n" + line2
}

func (m Model) headerSubtitle(group bool) string {
	if group {
		members, loaded := m.members[m.openJID]
		switch {
		case !loaded && m.membersLoading[m.openJID]:
			return "group · loading members…"
		case !loaded:
			return "group"
		}
		names := []string{"You"}
		for _, mb := range members {
			names = append(names, mb.Name)
		}
		return fmt.Sprintf("%d members · %s", len(members)+1, strings.Join(names, ", "))
	}
	jid, err := types.ParseJID(m.openJID)
	if err != nil {
		return ""
	}
	parts := []string{}
	if number := "+" + jid.User; jid.Server == types.DefaultUserServer && m.names[m.openJID] != number {
		parts = append(parts, number)
	}
	if m.wa != nil && m.openJID == m.wa.OwnJID() {
		parts = append(parts, "message yourself")
	} else if m.wa != nil && !m.wa.InContacts(m.openJID) {
		parts = append(parts, "not in your contacts")
	}
	return strings.Join(parts, " · ")
}
