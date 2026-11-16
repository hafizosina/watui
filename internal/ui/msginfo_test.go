package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zhenzhu/watui/internal/store"
)

func TestMessageInfo(t *testing.T) {
	const w, h = 120, 34
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	st := m.wa.Store
	group, dm := "1203630@g.us", "628111@s.whatsapp.net"
	var sent time.Time
	msgs, err := st.Messages(group, 50)
	must(t, err)
	for _, msg := range msgs {
		if msg.ID == "6" {
			sent = msg.TS
		}
	}
	// group: Sari read it (after delivery), Budi only got it; "6" is your "merged"
	must(t, st.PutReceipt(group, []string{"6"}, "628222@s.whatsapp.net", store.ReceiptDelivered, sent.Add(2*time.Second)))
	must(t, st.PutReceipt(group, []string{"6"}, "628222@s.whatsapp.net", store.ReceiptRead, sent.Add(6*time.Minute)))
	must(t, st.PutReceipt(group, []string{"6"}, "628444@s.whatsapp.net", store.ReceiptDelivered, sent.Add(3*time.Second)))
	must(t, st.PutReceipt(group, []string{"6"}, "628444@s.whatsapp.net", store.ReceiptDelivered, sent.Add(time.Hour))) // first one counts

	open := func(chat string) string {
		t.Helper()
		m = step(m, ChangedMsg{})
		m.open(chat)
		m.refresh()
		m.focus = focusChat
		m.sel = len(m.messages) - 1
		for i, msg := range m.messages {
			if msg.FromMe && chat == group && msg.ID == "6" {
				m.sel = i
			}
		}
		m = step(m, key(" "), key("I"))
		if m.info == nil || m.info.message == nil {
			t.Fatalf("Message info didn't open for %s", chat)
		}
		view := ansi.Strip(m.View())
		checkFrame(t, m.View(), w, h)
		return view
	}

	view := open(group)
	for _, want := range []string{"[3] Message info", "merged", "Sent", "Read by 1", "Delivered to 1", "+6m", "+3s"} {
		if !strings.Contains(view, want) {
			t.Errorf("group info missing %q:\n%s", want, view)
		}
	}
	m = step(m, key("esc"))
	if m.info != nil {
		t.Fatal("esc should close Message info")
	}

	// incoming DM message: arrived now, read by you after
	must(t, st.PutMessage(store.Message{ChatJID: dm, ID: "in1", SenderJID: dm, TS: time.Now().Add(-time.Minute), Text: "halo bro", ReceivedAt: time.Now().Add(-50 * time.Second)}))
	view = open(dm)
	for _, want := range []string{"halo bro", "From", "Received", "+10s", "Read by you"} {
		if !strings.Contains(view, want) {
			t.Errorf("incoming info missing %q:\n%s", want, view)
		}
	}
	must(t, st.MarkChatRead(dm))
	m = step(m, ChangedMsg{})
	if view := ansi.Strip(m.View()); strings.Contains(view, "not yet") || !strings.Contains(view, "today") {
		t.Errorf("read time should show live:\n%s", view)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
