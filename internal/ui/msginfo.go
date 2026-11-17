package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

// msgInfo is Message info: when a message was sent, delivered and read.
type msgInfo struct {
	msg  store.Message
	data wa.MessageInfo
	err  error
}

func (m *Model) openMessageInfo(msg store.Message) {
	m.info = &infoState{title: "Message info", message: m.loadMessageInfo(msg)}
	m.menu = nil
	m.enterNormal()
	m.focus = focusChat
}

func (m Model) loadMessageInfo(msg store.Message) *msgInfo {
	data, err := m.wa.MessageInfo(msg)
	return &msgInfo{msg: msg, data: data, err: err}
}

// whenText: "today 13:34:05", "yesterday 09:12:00", "Mon 16 Nov 13:34:05", "16 Nov 2025 13:34".
func whenText(t time.Time) string {
	t = t.Local()
	now := time.Now()
	switch {
	case sameDay(t, now):
		return "today " + t.Format("15:04:05")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "yesterday " + t.Format("15:04:05")
	case t.Year() == now.Year():
		return t.Format("Mon 2 Jan 15:04:05")
	}
	return t.Format("2 Jan 2006 15:04")
}

// afterText: how long after sending, "+2s", "+6m", "+3h", "+2d".
func afterText(sent, t time.Time) string {
	d := t.Sub(sent)
	switch {
	case d < 0:
		return ""
	case d < time.Minute:
		return fmt.Sprintf("+%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("+%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("+%dh", int(d.Hours()))
	}
	return fmt.Sprintf("+%dd", int(d.Hours()/24))
}

type receiptRow struct {
	user                    string
	delivered, read, played time.Time
}

// byUser folds receipts into one row per person.
func byUser(rs []store.Receipt) []receiptRow {
	idx := map[string]int{}
	var rows []receiptRow
	for _, r := range rs {
		i, ok := idx[r.User]
		if !ok {
			i = len(rows)
			idx[r.User] = i
			rows = append(rows, receiptRow{user: r.User})
		}
		switch r.Kind {
		case store.ReceiptDelivered:
			rows[i].delivered = r.TS
		case store.ReceiptRead:
			rows[i].read = r.TS
		case store.ReceiptPlayed:
			rows[i].played = r.TS
			if rows[i].read.IsZero() {
				rows[i].read = r.TS
			}
		}
	}
	return rows
}

func (m Model) messageInfoLines(mi *msgInfo, w int) []string {
	msg := mi.msg
	label := func(s string) string { return styleDim.Render(fmt.Sprintf("  %-11s", s)) }
	value := func(t time.Time, sent time.Time) string {
		if t.IsZero() {
			return styleDim.Render("—")
		}
		out := styleText.Render(whenText(t))
		if !sent.IsZero() {
			if a := afterText(sent, t); a != "" {
				out += styleDim.Render("  " + a)
			}
		}
		return out
	}
	preview := strings.ReplaceAll(sanitize(m.withNames(msg.Text)), "\n", " ")
	if preview == "" {
		preview = "[" + msg.MediaType + "]"
	}
	bar := lipgloss.NewStyle().Foreground(senderColor(msg.SenderJID)).Render("┃ ")
	if msg.FromMe {
		bar = lipgloss.NewStyle().Foreground(colBlue).Render("┃ ")
	}
	lines := []string{"", " " + bar + styleText.Render(ansi.Truncate(preview, max(10, w-6), "…")), ""}
	if mi.err != nil {
		return append(lines, " "+styleError.Render(mi.err.Error()))
	}

	if !msg.FromMe {
		chat, _ := m.openChat()
		from := m.names[m.openJID]
		if chat.IsGroup {
			from = m.wa.ContactName(msg.SenderJID, msg.SenderName)
		}
		lines = append(lines,
			label("From")+lipgloss.NewStyle().Foreground(senderColor(msg.SenderJID)).Bold(true).Render(sanitize(from)),
			label("Sent")+value(msg.TS, time.Time{})+styleDim.Render("  (their clock)"))
		rec := value(mi.data.Received, msg.TS)
		if mi.data.Received.IsZero() {
			rec = styleDim.Render("— synced from your phone, or before watui kept track")
		}
		lines = append(lines, label("Received")+rec)
		read := value(mi.data.Read, msg.TS)
		switch {
		case mi.data.Read.IsZero() && !msg.Read:
			read = lipgloss.NewStyle().Foreground(colGreen).Render("not yet")
		case mi.data.Read.IsZero():
			read = styleDim.Render("— time not recorded")
		}
		return append(lines, label("Read by you")+read)
	}

	lines = append(lines, label("Sent")+value(msg.TS, time.Time{})+" "+statusIcon(msg.Status))
	rows := byUser(mi.data.Receipts)
	chat, _ := m.openChat()
	if !chat.IsGroup {
		var r receiptRow
		if len(rows) > 0 {
			r = rows[0]
		}
		lines = append(lines, label("Delivered")+value(r.delivered, msg.TS), label("Read")+value(r.read, msg.TS))
		if !r.played.IsZero() {
			lines = append(lines, label("Played")+value(r.played, msg.TS))
		}
		if len(rows) == 0 && msg.Status >= store.StatusDelivered {
			lines = append(lines, "", styleDim.Render("  times not recorded — sent before watui kept receipts"))
		}
		return lines
	}

	var read, delivered []receiptRow
	for _, r := range rows {
		if !r.read.IsZero() {
			read = append(read, r)
		} else if !r.delivered.IsZero() {
			delivered = append(delivered, r)
		}
	}
	sort.SliceStable(read, func(i, j int) bool { return read[i].read.Before(read[j].read) })
	sort.SliceStable(delivered, func(i, j int) bool { return delivered[i].delivered.Before(delivered[j].delivered) })
	total := -1
	if members, ok := m.members[m.openJID]; ok {
		total = len(members)
	}
	of := func(n int) string {
		if total > 0 {
			return fmt.Sprintf("%d of %d", n, total)
		}
		return fmt.Sprint(n)
	}
	nameW := max(8, min(28, w/3))
	person := func(r receiptRow, t time.Time, extra string) string {
		name := ansi.Truncate(sanitize(m.wa.ContactName(r.user, "")), nameW, "…")
		name += strings.Repeat(" ", max(0, nameW-lipgloss.Width(name)))
		return "    " + lipgloss.NewStyle().Foreground(senderColor(r.user)).Render(name) + "  " + value(t, msg.TS) + extra
	}
	lines = append(lines, "", " "+styleRead.Render("✓✓")+" "+lipgloss.NewStyle().Foreground(colFg).Bold(true).Render("Read by "+of(len(read))))
	for _, r := range read {
		extra := ""
		if !r.played.IsZero() {
			extra = styleDim.Render("  ▶ played")
		}
		lines = append(lines, person(r, r.read, extra))
	}
	lines = append(lines, "", " "+styleDim.Render("✓✓")+" "+lipgloss.NewStyle().Foreground(colFg).Bold(true).Render("Delivered to "+of(len(delivered))))
	for _, r := range delivered {
		lines = append(lines, person(r, r.delivered, ""))
	}
	if total > 0 {
		if waiting := total - len(read) - len(delivered); waiting > 0 {
			lines = append(lines, "", " "+styleDim.Render(fmt.Sprintf("✓  not delivered yet: %d", waiting)))
		}
	}
	if len(rows) == 0 && msg.Status >= store.StatusDelivered {
		lines = append(lines, "", styleDim.Render("  who/when not recorded — sent before watui kept receipts"))
	}
	return lines
}

func (m Model) messageInfoView(in *infoState, w, h int) string {
	lines := m.messageInfoLines(in.message, w)
	in.offset = max(0, min(in.offset, len(lines)-h))
	end := min(len(lines), in.offset+h)
	return strings.Join(lines[in.offset:end], "\n")
}

func (m Model) messageInfoKey(key string) (Model, bool) {
	in := m.info
	switch key {
	case "esc", "q":
		m.info = nil
		m.renderMessages(false)
	case "j", "down":
		in.offset++
	case "k", "up":
		in.offset = max(0, in.offset-1)
	case "ctrl+d":
		in.offset += 10
	case "ctrl+u":
		in.offset = max(0, in.offset-10)
	case "g":
		in.offset = 0
	default:
		return m, false
	}
	return m, true
}
