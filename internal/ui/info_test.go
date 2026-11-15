package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhenzhu/watui/internal/wa"
)

func TestInfoViews(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 120, Height: 40}, ChangedMsg{})
	m = step(m, key(" "))
	if m.menu == nil || m.menu.items[1].label != "Group info" {
		t.Fatalf("group chat menu should offer Group info, got %v", labelsOf(m.menu.items))
	}
	m = step(m, key("esc"))
	m.info = &infoState{title: "BE Squad", loading: true}
	m.focus = focusChat
	group := &wa.Group{
		JID: "1203630@g.us", Name: "BE Squad", Topic: "Backend team\nDeploy every Friday",
		Created: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), CreatedBy: "Andi",
		Members: []wa.Member{
			{JID: "62800@s.whatsapp.net", Name: "You", IsMe: true, Phone: "+62800"},
			{JID: "62801@s.whatsapp.net", Name: "Andi", Phone: "+62801", Admin: true, Owner: true},
			{JID: "62802@s.whatsapp.net", Name: "Sari", Phone: "+62802"},
		},
	}
	m = step(m, infoLoadedMsg{group: group})
	frame := m.View()
	checkFrame(t, frame, 120, 40)
	for _, want := range []string{"[3] Info · BE Squad", "Open group chat", "Deploy every Friday", "01 Mar 2024 by Andi", "3 members", "owner", "Sari"} {
		if !strings.Contains(frame, want) {
			t.Errorf("group info should show %q", want)
		}
	}
	m = step(m, key("j"), key("j"), key("enter"))
	if m.info != nil || m.openJID != "62802@s.whatsapp.net" || m.mode != modeInsert {
		t.Fatalf("enter on a member should open a chat with them, got %q", m.openJID)
	}

	m = step(m, key("esc"), key("esc"))
	m.info = &infoState{title: "Budi", loading: true}
	m = step(m, infoLoadedMsg{person: &wa.Person{JID: "628111@s.whatsapp.net", Phone: "+628111", Name: "Budi Santoso", PushName: "budi.s", About: "Sibuk", Groups: []string{"BE Squad", "Phase 2 Team"}}})
	frame = m.View()
	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + frame)
	}
	if strings.Contains(frame, "[4] Compose") {
		t.Error("profile details must not show the Compose box")
	}
	m = step(m, key("i"))
	if m.mode == modeInsert || m.info == nil {
		t.Error("i on a profile must not start typing")
	}
	for _, want := range []string{"Budi Santoso", "+628111", "~budi.s", "Message Budi Santoso", "About", "Sibuk", "Groups in common (2)", "Phase 2 Team", "BS"} {
		if !strings.Contains(frame, want) {
			t.Errorf("person info should show %q", want)
		}
	}
	m = step(m, key("enter"))
	if m.info != nil || m.openJID != "628111@s.whatsapp.net" || m.mode != modeInsert {
		t.Fatalf("enter on a person should open a chat with them, got %q", m.openJID)
	}
	m = step(m, key("esc"), key("esc"))
	m.info = &infoState{title: "BE Squad"}
	m = step(m, infoLoadedMsg{group: group}, key("o"))
	if m.info != nil || m.openJID != "1203630@g.us" {
		t.Fatalf("o on a group should open the group chat, got %q", m.openJID)
	}
	m = step(m, key("esc"), key("esc"))
	m = step(m, key(":"))
	m = typeInto(m, "info")
	m = step(m, key("enter"), infoLoadedMsg{group: group})
	if m.info == nil {
		t.Fatal(":info should open the details of the open chat")
	}
	m = step(m, key("esc"))
	if m.info != nil {
		t.Fatal("esc should close the info view")
	}
	if os.Getenv("WATUI_SHOW") != "" {
		m.info = &infoState{title: "BE Squad"}
		m = step(m, infoLoadedMsg{group: group})
		t.Log("\n" + m.View())
	}
}
