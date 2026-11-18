package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

func TestDownloadBarOnAttachment(t *testing.T) {
	m := testModel(t)
	dm := "628111@s.whatsapp.net"
	if err := m.wa.Store.PutMessage(store.Message{ChatJID: dm, ID: "DOC1", SenderJID: dm, TS: time.Now(), Read: true,
		Text: "[document: laporan.pdf · 27.0 MB]", MediaType: wa.MediaDoc, Media: []byte{}}); err != nil {
		t.Fatal(err)
	}
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 30}, ChangedMsg{})
	m.open(dm)

	line := func() string {
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, iconFile+" laporan.pdf") { // the messages pane, not the chat list
				return l
			}
		}
		t.Fatal("document not shown")
		return ""
	}
	if l := line(); !strings.Contains(l, "space → Open") {
		t.Fatalf("idle document should show its hint: %q", l)
	}

	stop := wa.FakeDownload("DOC1", 27<<19, 27<<20) // half of 27 MB
	m = step(m, tickMsg(time.Now()))
	l := line()
	if !strings.Contains(l, "━━━━━━") || !strings.Contains(l, "━━━━━━────── 50% · 13.5 MB") || strings.Contains(l, "space → Open") {
		t.Fatalf("progress should replace the hint on the document line: %q", l)
	}
	if strings.Contains(ansi.Strip(m.View()), "downloading") {
		t.Fatal("no download message at the bottom")
	}

	stop()
	m = step(m, tickMsg(time.Now()))
	if l := line(); !strings.Contains(l, "space → Open") || strings.Contains(l, "%") {
		t.Fatalf("bar should go away when the download ends: %q", l)
	}
}
