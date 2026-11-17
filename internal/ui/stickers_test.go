package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

func TestStickerPicker(t *testing.T) {
	m := testModel(t)
	chat := "628111@s.whatsapp.net"
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 40}, ChangedMsg{}, key("j"), key("enter"), tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.stickers != nil || !strings.Contains(m.flash, "no stickers yet") {
		t.Fatal("with no stickers, ctrl+t should explain instead of opening an empty picker")
	}
	put := func(id string, sha byte, sec int) {
		data, _ := proto.Marshal(&waE2E.StickerMessage{DirectPath: proto.String("/v/" + id), FileSHA256: []byte{sha}})
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: chat, ID: id, SenderJID: chat, TS: time.Now().Add(time.Duration(sec) * time.Second),
			Text: "[sticker]", MediaType: wa.MediaSticker, Media: data, Read: true}); err != nil {
			t.Fatal(err)
		}
	}
	put("S1", 1, 1)
	put("S2", 2, 2)
	put("S3", 1, 3)
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.stickers == nil || len(m.stickers.refs) != 2 || m.stickers.refs[0].ID != "S3" {
		t.Fatalf("picker should list 2 distinct stickers, newest first, got %+v", m.stickers)
	}
	frame := m.View()
	if !strings.Contains(frame, "Stickers · recent") {
		t.Fatal("sticker picker should be visible")
	}
	checkFrame(t, frame, 120, 40)
	m = step(m, key("l"))
	if m.stickers.sel != 1 {
		t.Fatal("l should move to the next sticker")
	}
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if m.stickers != nil || cmd == nil {
		t.Fatal("enter should send the sticker and close the picker")
	}
}
