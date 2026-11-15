package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

func TestDocuments(t *testing.T) {
	m := testModel(t)
	chat := "628111@s.whatsapp.net"
	put := func(id, name string, size uint64, sec int) {
		doc, _ := proto.Marshal(&waE2E.DocumentMessage{FileName: proto.String(name), FileLength: proto.Uint64(size)})
		text := "[document: " + name + " · " + wa.Size(size) + "]"
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: chat, ID: id, SenderJID: chat, TS: time.Now().Add(time.Duration(sec) * time.Second),
			Text: text, Status: store.StatusSent, Read: true, MediaType: wa.MediaDoc, Media: doc}); err != nil {
			t.Fatal(err)
		}
	}
	put("PY", "processor.py", 12_345, 1)
	put("PDF", "2025+Protein+Naskah.pdf", 2_500_000, 2)
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 40}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	view := m.vp.View()
	for _, want := range []string{"processor.py · 12 KB", "2025+Protein+Naskah.pdf · 2.4 MB", "space → Open"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q", want)
		}
	}
	m = step(m, key(" "))
	pdf := strings.Join(labelsOf(m.menu.items), "|")
	if !strings.Contains(pdf, "Open file") || strings.Contains(pdf, "View in nvim") || !strings.Contains(pdf, "Save to ~/Downloads/WhatsApp") {
		t.Errorf("pdf menu: want Open file + Save, no nvim; got %s", pdf)
	}
	m = step(m, key("esc"), key("k"), key(" "))
	py := strings.Join(labelsOf(m.menu.items), "|")
	if !strings.Contains(py, "View in nvim") {
		t.Errorf(".py menu should offer View in nvim, got %s", py)
	}
}
