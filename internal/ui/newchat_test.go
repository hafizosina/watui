package ui

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/profile"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

func modelWithContacts(t *testing.T) Model {
	t.Helper()
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+t.TempDir()+"/wa.db?_foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	device := container.NewDevice()
	device.ID = &types.JID{User: "6281200000000", Device: 1, Server: types.DefaultUserServer}
	device.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{1}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
	if err := container.PutDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{{"6281111111111", "Budi Santoso"}, {"6282222222222", "Sari Dewi"}, {"6283333333333", "Andi"}} {
		if err := device.Contacts.PutContactName(ctx, types.NewJID(c[0], types.DefaultUserServer), c[1], ""); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/msg.db")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	client := wa.New(whatsmeow.NewClient(device, waLog.Noop), st, waLog.Noop)
	m := New([]*profile.Profile{{Name: profile.Default, Dir: t.TempDir(), WA: client}}, config.Default(), nil, Options{})
	return step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
}

func typeInto(m Model, s string) Model {
	for _, k := range keysOf(s) {
		m = step(m, k)
	}
	return m
}

func TestNewChat(t *testing.T) {
	m := modelWithContacts(t)
	m = step(m, key("n"))
	if m.menu == nil || !strings.HasPrefix(m.menu.title, "New chat · 3 contacts") {
		t.Fatalf("n should open the new chat picker, got %+v", m.menu)
	}
	m = typeInto(m, "sari")
	if v := m.menu.visible(); len(v) != 1 || !strings.Contains(v[0].label, "Sari Dewi") {
		t.Fatalf("searching 'sari' should leave Sari, got %d items", len(v))
	}
	m = step(m, key("enter"))
	if m.openJID != "6282222222222@s.whatsapp.net" || m.mode != modeInsert {
		t.Fatalf("picking a contact should open the chat in compose, got %q mode %v", m.openJID, m.mode)
	}
	if m.names[m.openJID] != "Sari Dewi" {
		t.Fatalf("new chat should be named after the contact, got %q", m.names[m.openJID])
	}

	m = step(m, key("esc"), key("esc"), key("q"), key("n"))
	m = typeInto(m, "0811 1111")
	v := m.menu.visible()
	if len(v) < 2 || v[0].label != "Chat with +628111111" {
		t.Fatalf("a number should offer 'Chat with' first and still match Budi, got %v", labelsOf(v))
	}

	m = step(m, key("esc"), key("n"))
	m = step(m, key("enter"))
	if m.menu == nil || !strings.HasPrefix(m.menu.title, "New group") {
		t.Fatal("+ New group… should open the member picker")
	}
	m = typeInto(m, "budi")
	m = step(m, key("enter"))
	if !strings.Contains(m.menu.title, "Budi Santoso") || m.menu.filter != "budi" {
		t.Fatalf("toggling should keep the picker open with the filter, got %q filter %q", m.menu.title, m.menu.filter)
	}
	m.menu.filter = ""
	m = step(m, key("enter"))
	if m.mode != modeCommand || m.prompt.Value() != "group " || len(m.pendingGroup) != 1 {
		t.Fatalf("Next should ask for the group name, got mode %v prompt %q members %v", m.mode, m.prompt.Value(), m.pendingGroup)
	}
	checkFrame(t, m.View(), 110, 34)
}

func labelsOf(items []menuItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

func TestMentionsShowNames(t *testing.T) {
	m := modelWithContacts(t)
	ctx := context.Background()
	client := m.wa
	lid := types.NewJID("197426845585449", types.HiddenUserServer)
	if err := client.WA.Store.LIDs.PutLIDMapping(ctx, lid, types.NewJID("6282222222222", types.DefaultUserServer)); err != nil {
		t.Fatal(err)
	}
	stranger := types.NewJID("191869594144898", types.HiddenUserServer)
	if err := client.Store.PutMessage(store.Message{ChatJID: "120363@g.us", ID: "x", SenderJID: stranger.String(), SenderName: "Fahmi"}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"payload gini mas @197426845585449 ?": "payload gini mas @Sari Dewi ?", // LID → phone → contact
		"cc @6281111111111":                   "cc @Budi Santoso",              // phone number
		"kenapa @191869594144898 ?":           "kenapa @Fahmi ?",               // push name from their messages
		"hi @6281200000000":                   "hi @You",
		"unknown @99999999999 stays":          "unknown @99999999999 stays",
		"email a@b.com, at 10:30":             "email a@b.com, at 10:30",
	}
	for in, want := range cases {
		if got := client.ResolveMentions(in); got != want {
			t.Errorf("ResolveMentions(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatListHighlightsMentions(t *testing.T) {
	m := modelWithContacts(t)
	st := m.wa.Store
	group, quiet, old := "120363001@g.us", "120363002@g.us", "120363003@g.us"
	now := time.Now()
	for _, msg := range []store.Message{
		{ChatJID: group, ID: "a", SenderJID: "6281111111111@s.whatsapp.net", TS: now, Text: "cek ya @You", MentionsMe: true},
		{ChatJID: quiet, ID: "b", SenderJID: "6281111111111@s.whatsapp.net", TS: now, Text: "halo semua"},
		{ChatJID: old, ID: "c", SenderJID: "6281111111111@s.whatsapp.net", TS: now, Text: "tolong @6281200000000"},
	} {
		if err := st.PutMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	forMe := func() map[string]int {
		chats, err := st.Chats()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, c := range chats {
			out[c.JID] = c.UnreadForMe
		}
		return out
	}
	if got := forMe(); got[group] != 1 || got[quiet] != 0 || got[old] != 0 {
		t.Fatalf("UnreadForMe = %v, want only %s", got, group)
	}
	// messages stored before for_me existed get flagged from their text
	if err := st.MarkForMe([]string{"@6281200000000"}, "6281200000000@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}
	if got := forMe(); got[old] != 1 {
		t.Fatalf("backfill: UnreadForMe[old] = %d, want 1", got[old])
	}
	chats, _ := st.Chats()
	for _, c := range chats {
		row := ansi.Strip(m.chatRow(c, false, 40))
		if has := strings.Contains(row, " @ "); has != (c.UnreadForMe > 0) {
			t.Errorf("%s row %q: @ badge = %t", c.JID, row, has)
		}
	}
	if err := st.MarkChatRead(group); err != nil {
		t.Fatal(err)
	}
	if got := forMe(); got[group] != 0 {
		t.Fatalf("after read: UnreadForMe = %d, want 0", got[group])
	}
}
