package ui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"go.mau.fi/whatsmeow/types"

	"github.com/hafizosina/watui/internal/config"
	"github.com/hafizosina/watui/internal/profile"
	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

func testModel(t *testing.T) Model {
	t.Helper()
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+t.TempDir()+"/wa.db?_foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/msg.db")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	put := func(chat, id, sender, name string, fromMe bool, ago time.Duration, text string, status store.Status, read bool) {
		if err := st.PutMessage(store.Message{ChatJID: chat, ID: id, SenderJID: sender, SenderName: name, FromMe: fromMe, TS: now.Add(-ago), Text: text, Status: status, Read: read}); err != nil {
			t.Fatal(err)
		}
	}
	dm := "628111@s.whatsapp.net"
	group := "1203630@g.us"
	put(dm, "1", dm, "Budi", false, 26*time.Hour, "Halo, besok jadi meeting?", store.StatusSent, true)
	put(dm, "2", "me@s.whatsapp.net", "", true, 25*time.Hour, "Jadi, jam 10 ya", store.StatusRead, true)
	put(dm, "3", dm, "Budi", false, 10*time.Minute, "Oke siap. Ini agak panjang supaya kelihatan wrapping-nya bekerja dengan benar di panel pesan sebelah kanan.", store.StatusSent, false)
	put(group, "4", "628222@s.whatsapp.net", "Sari", false, 3*time.Hour, "[image] deploy result", store.StatusSent, true)
	put(group, "5", "628333@s.whatsapp.net", "Andi", false, 2*time.Hour, "LGTM", store.StatusSent, false)
	put(group, "6", "me@s.whatsapp.net", "", true, time.Hour, "merged", store.StatusDelivered, true)
	if err := st.SetChatName(group, "BE Squad"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPinned(group, true); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMessage(group, "5"); err != nil {
		t.Fatal(err)
	}
	client := wa.New(whatsmeow.NewClient(device, waLog.Noop), st, waLog.Noop)
	return New([]*profile.Profile{{Name: profile.Default, WA: client}}, config.Default(), nil, Options{})
}

func step(m Model, msgs ...tea.Msg) Model {
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func checkFrame(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Errorf("frame has %d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("line %d is %d wide, want <= %d", i, lw, w)
		}
	}
}

func TestRender(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	if len(m.chats) != 2 || m.names[m.chats[0].JID] != "BE Squad" {
		t.Fatalf("pinned group should be first, got %+v", m.chats)
	}
	checkFrame(t, m.View(), w, h)

	m = step(m, key("j"), key("enter"))
	if m.openJID != "628111@s.whatsapp.net" || m.focus != focusChat {
		t.Fatalf("enter should open the DM, got %q", m.openJID)
	}
	open := m.View()
	checkFrame(t, open, w, h)

	m = step(m, key("i"))
	if m.mode != modeInsert {
		t.Fatal("i should enter insert mode")
	}
	m = step(m, key("esc"), key("esc"), key("q"), key("/"), key("squ"))
	if len(m.chats) != 1 {
		t.Fatalf("search should leave 1 chat, got %d", len(m.chats))
	}
	m = step(m, key("esc"), key("?"))
	checkFrame(t, m.View(), w, h)

	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + open)
	}
}

func TestMessageMenu(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	if m.sel != len(m.messages)-1 {
		t.Fatalf("newest message should be selected, got %d", m.sel)
	}
	m = step(m, key("k"))
	if m.sel != len(m.messages)-2 {
		t.Fatalf("k should select the previous message, got %d", m.sel)
	}
	m = step(m, key(" "))
	if m.menu == nil || m.menu.title != "Message" {
		t.Fatal("space should open the message menu")
	}
	frame := m.View()
	checkFrame(t, frame, w, h)
	if !strings.Contains(frame, "Reply") || !strings.Contains(frame, "Forward") {
		t.Fatal("menu should list Reply and Forward")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlN}, tea.KeyMsg{Type: tea.KeyCtrlP}, key("enter"))
	if m.menu != nil || m.replyTo == nil || m.mode != modeInsert {
		t.Fatalf("picking Reply should close the menu and start a reply, menu=%v reply=%v", m.menu, m.replyTo)
	}
	if !strings.Contains(m.View(), "Replying to") {
		t.Fatal("compose title should show the reply")
	}
	m = step(m, key("esc"), key("esc"), key(" "), key("f"))
	if m.menu == nil || !m.menu.filterable {
		t.Fatal("f should open the forward picker")
	}
	m = step(m, key("b"), key("e"))
	if got := len(m.menu.visible()); got != 1 {
		t.Fatalf("typing 'be' should leave 1 chat, got %d", got)
	}
	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + frame)
	}
}

func TestCtrlHL(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"), tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.focus != focusList {
		t.Fatal("ctrl+h should focus the chat list")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.focus != focusChat {
		t.Fatal("ctrl+l should focus the messages")
	}
	m = step(m, key("i"), tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.focus != focusList || m.mode != modeNormal {
		t.Fatal("ctrl+h from compose should leave compose and focus the chat list")
	}
}

func TestCtrlJK(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"), tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.mode != modeInsert || m.input.mode != vimNormal {
		t.Fatalf("ctrl+j from messages should open compose in normal mode, got mode=%v vim=%v", m.mode, m.input.mode)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.mode != modeNormal || m.focus != focusChat {
		t.Fatal("ctrl+k from compose normal should go back to messages")
	}
	m = step(m, key("i"), key("ab"), tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.mode != modeInsert || m.input.mode != vimInsert {
		t.Fatal("ctrl+k in insert mode should not leave compose")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.input.Value() != "ab\n" {
		t.Fatalf("ctrl+j in insert should add a new line, got %q", m.input.Value())
	}
}

func TestLongMessageScroll(t *testing.T) {
	m := testModel(t)
	m.cfg.CollapseLines = 0
	long := strings.Repeat("baris panjang\n", 40) + "akhir"
	if err := m.wa.Store.PutMessage(store.Message{ChatJID: "628111@s.whatsapp.net", ID: "LONG", SenderJID: "628111@s.whatsapp.net", TS: time.Now(), Text: long, Status: store.StatusSent, Read: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.wa.Store.PutMessage(store.Message{ChatJID: "628111@s.whatsapp.net", ID: "AFTER", SenderJID: "628111@s.whatsapp.net", TS: time.Now().Add(time.Second), Text: "after", Status: store.StatusSent, Read: true}); err != nil {
		t.Fatal(err)
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	last := len(m.messages) - 1
	if m.messages[last].ID != "AFTER" {
		t.Fatalf("want AFTER last, got %s", m.messages[last].ID)
	}
	m = step(m, key("k"))
	if m.messages[m.sel].ID != "LONG" {
		t.Fatal("k should select the long message")
	}
	if !strings.Contains(m.vp.View(), "akhir") {
		t.Fatal("moving up into a long message should show its end first")
	}
	before := m.vp.YOffset
	m = step(m, key("k"))
	if m.messages[m.sel].ID != "LONG" || m.vp.YOffset >= before {
		t.Fatalf("k inside a long message should scroll up within it, sel=%s offset %d -> %d", m.messages[m.sel].ID, before, m.vp.YOffset)
	}
	for i := 0; i < 20; i++ {
		m = step(m, key("k"))
	}
	if m.messages[m.sel].ID == "LONG" {
		t.Fatal("after reaching the top of the long message, k should move to the previous message")
	}
	checkFrame(t, m.View(), 110, 30)
}

func TestFoldLongMessage(t *testing.T) {
	m := testModel(t)
	long := strings.Repeat("baris\n", 40) + "akhir"
	if err := m.wa.Store.PutMessage(store.Message{ChatJID: "628111@s.whatsapp.net", ID: "LONG", SenderJID: "628111@s.whatsapp.net", TS: time.Now(), Text: long, Status: store.StatusSent, Read: true}); err != nil {
		t.Fatal(err)
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 40}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	if m.messages[m.sel].ID != "LONG" {
		t.Fatalf("want LONG selected, got %s", m.messages[m.sel].ID)
	}
	view := m.vp.View()
	if !strings.Contains(view, "26 more lines") || strings.Contains(view, "akhir") {
		t.Fatal("long message should be folded to 15 lines with a '26 more lines' marker")
	}
	m = step(m, key(" "))
	if m.menu == nil || m.menu.items[0].label != "Expand message" {
		t.Fatal("menu should offer Expand message first")
	}
	m = step(m, key("z"))
	if strings.Contains(m.vp.View(), "more lines") {
		t.Fatal("after expand, the marker should be gone")
	}
	for i := 0; i < 15; i++ {
		m = step(m, key("j"))
	}
	if !strings.Contains(m.vp.View(), "akhir") {
		t.Fatal("after expand, j should scroll down to the end of the message")
	}
	m = step(m, key(" "), key("z"))
	if !strings.Contains(m.vp.View(), "more lines") {
		t.Fatal("collapse should fold it again")
	}
	checkFrame(t, m.View(), 110, 40)
}

func TestHLDoNotSwitchBoxes(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("l"))
	if m.focus != focusList || m.openJID != "" {
		t.Fatal("l in the chat list should not open a chat")
	}
	m = step(m, key("enter"), key("h"))
	if m.focus != focusChat {
		t.Fatal("h in messages should not go back to the chat list")
	}
}

func TestMediaLabels(t *testing.T) {
	m := testModel(t)
	chat := "628111@s.whatsapp.net"
	put := func(id, kind, text string, sec int) {
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: chat, ID: id, SenderJID: chat, TS: time.Now().Add(time.Duration(sec) * time.Second), Text: text, Status: store.StatusSent, Read: true, MediaType: kind, Media: []byte{}}); err != nil {
			t.Fatal(err)
		}
	}
	put("VID", wa.MediaVideo, "[video 0:42 · 3.1 MB] lihat ini", 1)
	put("VN", wa.MediaAudio, "[voice note 0:12]", 2)
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 40}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	view := m.vp.View()
	for _, want := range []string{"▷ voice note 0:12", "space → Play"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q", want)
		}
	}
	m = step(m, key(" "))
	labels := ""
	for _, it := range m.menu.items {
		labels += it.label + "|"
	}
	if !strings.Contains(labels, "Save to ~/Downloads/WhatsApp") {
		t.Errorf("voice note menu should offer Save, got %s", labels)
	}
	m = step(m, key("esc"), key("k"), key(" "))
	labels = ""
	for _, it := range m.menu.items {
		labels += it.label + "|"
	}
	if !strings.Contains(labels, "Play video") {
		t.Errorf("video menu should offer Play video, got %s", labels)
	}
}

func linkedProfile(t *testing.T, name, phone string) *profile.Profile {
	t.Helper()
	m := testModel(t)
	m.wa.WA.Store.ID = &types.JID{User: phone, Server: types.DefaultUserServer}
	return &profile.Profile{Name: name, Dir: t.TempDir(), WA: m.wa}
}

func TestProfiles(t *testing.T) {
	work := linkedProfile(t, "work", "62811")
	personal := linkedProfile(t, profile.Default, "62822")
	if err := work.WA.Store.PutMessage(store.Message{ChatJID: "62999@s.whatsapp.net", ID: "W1", SenderJID: "62999@s.whatsapp.net", TS: time.Now(), Text: "rapat jam 3", Status: store.StatusSent}); err != nil {
		t.Fatal(err)
	}
	m := New([]*profile.Profile{personal, work}, config.Default(), nil, Options{})
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m = step(m, ChangedMsg{Profile: "work"})
	if m.unread["work"] != 3 {
		t.Fatalf("background profile unread should be counted, got %d", m.unread["work"])
	}
	frame := m.View()
	checkFrame(t, frame, 110, 34)
	for _, want := range []string{"[1] Profile", "[2] Chats", "● default", "others", "3"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame should contain %q", want)
		}
	}
	m = step(m, key("g"), key("t"))
	if m.activeProfile().Name != "work" || len(m.chats) != 3 || m.names[m.chats[0].JID] == "" {
		t.Fatalf("gt should switch to work and load its chats, got %s with %d chats", m.activeProfile().Name, len(m.chats))
	}
	if !strings.Contains(m.View(), "● work") {
		t.Fatal("profile box should show the active profile")
	}
	m = step(m, key("1"))
	if m.focus != focusProfiles {
		t.Fatal("1 should focus the profiles box")
	}
	m = step(m, key("enter"))
	if m.menu == nil || m.menu.title != "Profiles" || len(m.menu.items) != 3 {
		t.Fatal("enter should list the profiles plus Add profile")
	}
	m = step(m, key("1"))
	if m.activeProfile().Name != profile.Default {
		t.Fatal("picking 1 in the list should switch to default")
	}
	m = step(m, key("1"), key(" "))
	if m.menu == nil || !strings.HasPrefix(m.menu.title, "Profile") {
		t.Fatal("space in profiles should open the profile menu")
	}
	labels := ""
	for _, it := range m.menu.items {
		labels += it.label + "|"
	}
	for _, want := range []string{"Rename", "Reconnect", "Log out", "Add profile"} {
		if !strings.Contains(labels, want) {
			t.Errorf("profile menu should have %q, got %s", want, labels)
		}
	}
	if strings.Contains(labels, "Remove") {
		t.Error("default profile must not offer Remove")
	}
	m = step(m, key("esc"), tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.focus != focusList {
		t.Fatal("ctrl+j from profiles should go to chats")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.focus != focusProfiles {
		t.Fatal("ctrl+k from chats should go to profiles")
	}
	m = step(m, key(":"))
	for _, k := range keysOf("profile work") {
		m = step(m, k)
	}
	m = step(m, key("enter"))
	if m.activeProfile().Name != "work" {
		t.Fatal(":profile work should switch")
	}
	checkFrame(t, m.View(), 110, 34)
	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + m.View())
	}
}

func TestChatMenu(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m = step(m, key(" "))
	if m.menu == nil || m.menu.title != "Group · BE Squad" {
		t.Fatalf("space on a group should open the group menu, got %+v", m.menu)
	}
	labels := strings.Join(labelsOf(m.menu.items), "|")
	for _, want := range []string{"Open", "Unpin", "Mute…", "Archive", "Mark as read", "Leave group", "Delete chat"} {
		if !strings.Contains(labels, want) {
			t.Errorf("group menu should have %q, got %s", want, labels)
		}
	}
	m = step(m, key("m"))
	if m.menu == nil || len(m.menu.items) != 3 {
		t.Fatal("Mute… should offer 8 hours / 1 week / Always")
	}
	m = step(m, key("esc"), key("j"), key(" "))
	labels = strings.Join(labelsOf(m.menu.items), "|")
	if !strings.Contains(labels, "Copy number +628111") || strings.Contains(labels, "Leave group") {
		t.Errorf("DM menu should copy the number and not offer Leave, got %s", labels)
	}
	m = step(m, key("d"))
	if m.menu == nil || !strings.HasPrefix(m.menu.title, "Delete chat with") || m.menu.items[0].label != "Cancel" {
		t.Fatal("Delete should ask to confirm, with Cancel first")
	}
	m = step(m, key("n"))
	if m.menu != nil || len(m.chats) != 2 {
		t.Fatal("cancel must not delete anything")
	}
	m = step(m, key(" "), key("o"))
	if m.openJID != "628111@s.whatsapp.net" || m.focus != focusChat {
		t.Fatal("Open should open the chat")
	}
	checkFrame(t, m.View(), 110, 34)
}

func TestMuteIcons(t *testing.T) {
	m := testModel(t)
	if err := m.wa.Store.SetMutedUntil("1203630@g.us", store.MutedForever); err != nil {
		t.Fatal(err)
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{}, key("enter"))
	frame := m.View()
	checkFrame(t, frame, 110, 34)
	if strings.Count(frame, iconMute) < 2 {
		t.Error("muted chat should show the mute icon in the chat list and the messages title")
	}
	if !strings.Contains(frame, iconMute+" muted") {
		t.Error("messages title should say muted next to the icon")
	}
}

func TestArchivedRow(t *testing.T) {
	m := testModel(t)
	if err := m.wa.Store.SetArchived("628111@s.whatsapp.net", true); err != nil {
		t.Fatal(err)
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	if len(m.chats) != 2 || m.chats[0].JID != rowArchived || m.chats[0].Unread != 1 {
		t.Fatalf("chat list should start with an Archived (1) row, got %+v", m.chats)
	}
	if m.cursor != 1 {
		t.Fatal("the cursor should start on the first real chat, not the Archived row")
	}
	if !strings.Contains(m.View(), "Archived") {
		t.Fatal("Archived row should be visible")
	}
	m = step(m, key("k"), key("enter"))
	if !m.showArchived || m.chats[0].JID != rowBack || m.chats[1].JID != "628111@s.whatsapp.net" {
		t.Fatalf("enter on Archived should show archived chats with a Back row, got %+v", m.chats)
	}
	m = step(m, key("k"), key("enter"))
	if m.showArchived {
		t.Fatal("enter on Back should return to normal chats")
	}
	m = step(m, key("k"), key(" "))
	if m.menu != nil {
		t.Fatal("the Archived row has no chat menu")
	}
	m = step(m, key("J"))
	m = step(m, stepChatMsg{m.stepSeq})
	if m.openJID != "1203630@g.us" {
		t.Fatalf("J from the Archived row should open the first real chat, got %q", m.openJID)
	}
	checkFrame(t, m.View(), 110, 34)
}

func TestViewProfileFromMessage(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m = step(m, key("j"), key("enter"), key(" "))
	if m.menu == nil || !strings.Contains(strings.Join(labelsOf(m.menu.items), "|"), "View profile · +628111") {
		t.Fatalf("a DM message menu should start with View profile, got %v", labelsOf(m.menu.items))
	}
	m = step(m, key("i"))
	if m.info == nil || m.info.title != "+628111" {
		t.Fatalf("i should open the person's profile, got %+v", m.info)
	}
	m = step(m, key("esc"), key("k"), key(" "))
	if m.menu == nil || !strings.Contains(strings.Join(labelsOf(m.menu.items), "|"), "View my profile") {
		t.Fatalf("own message should offer View my profile, got %v", labelsOf(m.menu.items))
	}
	m = step(m, key("esc"), key("q"), key("k"), key("enter"), key("k"), key("k"), key(" "))
	if m.menu == nil || !strings.Contains(strings.Join(labelsOf(m.menu.items), "|"), "View profile · Sari") {
		t.Fatalf("a group message should open the sender's profile, got %v", labelsOf(m.menu.items))
	}
}

func TestReplyJumps(t *testing.T) {
	m := testModel(t)
	chat := "628111@s.whatsapp.net"
	for i, r := range []struct{ id, text string }{{"R1", "setuju"}, {"R2", "aku juga"}} {
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: chat, ID: r.id, SenderJID: chat, TS: time.Now().Add(time.Duration(i+1) * time.Minute),
			Text: r.text, QuoteID: "1", QuoteSender: chat, QuoteText: "Halo, besok jadi meeting?", Read: true}); err != nil {
			t.Fatal(err)
		}
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 40}, ChangedMsg{}, key("j"), key("enter"))
	if !strings.Contains(m.vp.View(), "↩ 2 replies") {
		t.Fatal("the original message should show how many replies it has")
	}
	m = step(m, key(" "))
	if !strings.Contains(strings.Join(labelsOf(m.menu.items), "|"), "Go to replied message") {
		t.Fatalf("a reply should offer Go to replied message, got %v", labelsOf(m.menu.items))
	}
	m = step(m, key("g"))
	if m.messages[m.sel].ID != "1" {
		t.Fatalf("g should jump to the quoted original, got %s", m.messages[m.sel].ID)
	}
	m = step(m, key(" "), key("G"))
	if m.menu == nil || m.menu.title != "Replies (2)" || len(m.menu.items) != 2 {
		t.Fatalf("original with 2 replies should list them, got %+v", m.menu)
	}
	m = step(m, key("enter"))
	if m.messages[m.sel].ID != "R1" {
		t.Fatalf("picking a reply should jump to it, got %s", m.messages[m.sel].ID)
	}
	checkFrame(t, m.View(), 110, 40)
}

func TestLoadOlderMessages(t *testing.T) {
	m := testModel(t)
	chat := "628999@s.whatsapp.net"
	base := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 450; i++ {
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: chat, ID: fmt.Sprintf("M%03d", i), SenderJID: chat, TS: base.Add(time.Duration(i) * time.Minute), Text: fmt.Sprintf("pesan %d", i), Read: true}); err != nil {
			t.Fatal(err)
		}
	}
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m.open(chat)
	m.focus = focusChat
	if len(m.messages) != firstPage || m.messages[len(m.messages)-1].ID != "M449" {
		t.Fatalf("opening should load the newest %d messages, got %d", firstPage, len(m.messages))
	}
	m.sel = 0
	m.renderMessages(false)
	m.revealSelection()
	m = step(m, key("k"))
	if len(m.messages) != firstPage+olderPage-50 || m.messages[m.sel].ID != "M250" {
		t.Fatalf("k at the top should load older messages and keep the selection, got %d msgs, sel %s", len(m.messages), m.messages[m.sel].ID)
	}
	m = step(m, key("g"), key("g"))
	if m.messages[0].ID != "M000" || m.sel != 0 {
		t.Fatalf("gg should load everything back to the first message, got %s", m.messages[0].ID)
	}
	m = step(m, key("G"))
	m.vp.SetYOffset(0)
	m.renderMessages(false)
	checkFrame(t, m.View(), 110, 34)
}

func TestDeletedMessagesKeepContent(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	m = step(m, key("enter")) // BE Squad (pinned, first); Andi's "LGTM" was deleted for everyone
	if !strings.HasSuffix(m.openJID, "@g.us") {
		t.Fatalf("opened %q", m.openJID)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "⊘ deleted by sender") || !strings.Contains(view, "LGTM") {
		t.Errorf("deleted message should keep its text:\n%s", view)
	}
	// the store keeps it either way
	msgs, err := m.wa.Store.Messages(m.openJID, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range msgs {
		if msg.ID == "5" && (!msg.Deleted || msg.Text != "LGTM") {
			t.Errorf("stored: deleted=%t text=%q", msg.Deleted, msg.Text)
		}
	}
	m = step(m, key(":"))
	for _, r := range "set show_deleted=off" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = step(m, key("enter"))
	if m.cfg.ShowDeleted {
		t.Fatal(":set show_deleted=off didn't apply")
	}
	view = ansi.Strip(m.View())
	if strings.Contains(view, "LGTM") || !strings.Contains(view, "message deleted") {
		t.Errorf("show_deleted=off should hide the text:\n%s", view)
	}
}

func TestHeaderShowsWhoTheChatIsWith(t *testing.T) {
	const w, h = 110, 24
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	group := "1203630@g.us"
	m.members[group] = []wa.Mentionable{{JID: "1@lid", User: "1", Name: "Sari"}, {JID: "2@lid", User: "2", Name: "Andi"}}
	m.membersAt[group] = time.Now()
	m = step(m, key("enter"))
	view := ansi.Strip(m.View())
	checkFrame(t, m.View(), w, h)
	head := strings.Join(strings.Split(view, "\n")[:4], "\n")
	if !strings.Contains(head, "BE Squad") || !strings.Contains(head, "3 members · You, Sari, Andi") {
		t.Errorf("group header:\n%s", head)
	}
	m = step(m, key("esc"), key("j"), key("enter"))
	head = strings.Join(strings.Split(ansi.Strip(m.View()), "\n")[:4], "\n")
	if !strings.Contains(head, "+628111") || !strings.Contains(head, "not in your contacts") || strings.Count(head, "+628111") != 1 {
		t.Errorf("DM header:\n%s", head)
	}
}

func TestBigCompose(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	m = step(m, key("enter"), key("i"))
	small, vpSmall := m.composeH(), m.vp.Height
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlG})
	if !m.composeBig || m.composeH() <= small*2 || m.vp.Height >= vpSmall {
		t.Fatalf("ctrl+g should grow Compose: compose %d→%d, messages %d→%d", small, m.composeH(), vpSmall, m.vp.Height)
	}
	if m.vp.Height < 4 {
		t.Errorf("messages should keep a few lines, got %d", m.vp.Height)
	}
	checkFrame(t, m.View(), w, h)
	m = step(m, tea.WindowSizeMsg{Width: w, Height: 50})
	checkFrame(t, m.View(), w, 50)
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlG})
	if m.composeBig || m.composeH() != small {
		t.Errorf("ctrl+g again should shrink back to %d, got %d", small, m.composeH())
	}
}

func TestStepChatOpensWhereItStops(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	if len(m.chats) < 2 {
		t.Fatalf("test needs two chats, got %d", len(m.chats))
	}
	before := m.openJID
	m = step(m, key("J"))
	first := m.stepSeq
	m = step(m, key("J"))
	if m.openJID != before {
		t.Fatalf("J should not open a chat while still moving, opened %q", m.openJID)
	}
	m = step(m, stepChatMsg{first})
	if m.openJID != before {
		t.Fatal("a stale settle should not open the chat passed over")
	}
	want := m.stepTo
	m = step(m, stepChatMsg{m.stepSeq})
	if want == "" || m.openJID != want {
		t.Fatalf("settling should open %q, got %q", want, m.openJID)
	}

	// another key right after J acts on the chat J stopped on
	m = step(m, key("K"))
	want = m.stepTo
	m = step(m, key("i"))
	if want == "" || m.openJID != want || m.stepTo != "" {
		t.Fatalf("a key after K should open %q first, got %q", want, m.openJID)
	}
}
