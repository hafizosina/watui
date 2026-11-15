//go:build screenshots

package ui

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/profile"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

const shotW, shotH = 150, 42

func emptyProfile(t *testing.T, name, phone string) *profile.Profile {
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
	device.ID = &types.JID{User: phone, Server: types.DefaultUserServer}
	device.PushName = "Demo User"
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/msg.db")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return &profile.Profile{Name: name, Dir: t.TempDir(), WA: wa.New(whatsmeow.NewClient(device, waLog.Noop), st, waLog.Noop)}
}

func demoModel(t *testing.T) Model {
	work := emptyProfile(t, "work", "6281200000001")
	personal := emptyProfile(t, "personal", "6281200000002")
	st := personal.WA.Store
	now := time.Now()
	at := func(d time.Duration) time.Time { return now.Add(-d) }
	team := "120363000000001@g.us"
	sari := "6281111111111@s.whatsapp.net"
	andi := "6282222222222@s.whatsapp.net"
	budi := "6283333333333@s.whatsapp.net"
	me := "6281200000002@s.whatsapp.net"
	for _, c := range [][2]string{{team, "Backend Team"}, {sari, "Sari"}, {andi, "Andi"}, {budi, "Budi"}, {"6284444444444@s.whatsapp.net", "Dewi"}, {"120363000000002@g.us", "Family"}} {
		_ = st.EnsureChat(c[0], strings.HasSuffix(c[0], "@g.us"))
		_ = st.SetChatName(c[0], c[1])
	}
	msgs := []store.Message{
		{ChatJID: team, ID: "1", SenderJID: andi, SenderName: "Andi", TS: at(3 * time.Hour), Text: "Morning! Deploy to UAT is scheduled for 14:00 today 🚀", Read: true},
		{ChatJID: team, ID: "2", SenderJID: sari, SenderName: "Sari", TS: at(3*time.Hour - time.Minute), Text: "Nice. I'll run the regression suite after it's up.", Read: true, Reactions: map[string]string{andi: "👍", me: "👍"}},
		{ChatJID: team, ID: "3", SenderJID: me, FromMe: true, TS: at(2 * time.Hour), Text: "Migration script is ready, PR is up for review.", Status: store.StatusRead, Read: true, QuoteID: "1", QuoteSender: andi, QuoteText: "Morning! Deploy to UAT is scheduled for 14:00 today 🚀", Starred: true},
		{ChatJID: team, ID: "4", SenderJID: andi, SenderName: "Andi", TS: at(90 * time.Minute), Text: "[voice note 0:42]", MediaType: wa.MediaAudio, Read: true},
		{ChatJID: team, ID: "5", SenderJID: sari, SenderName: "Sari", TS: at(80 * time.Minute), Text: "[document: regression-report.pdf · 1.2 MB] Results from last night", MediaType: wa.MediaDoc, Read: true},
		{ChatJID: team, ID: "6", SenderJID: budi, SenderName: "Budi", TS: at(30 * time.Minute), Text: "Heads up: the payment callback changed, see the forwarded note below.", Read: true},
		{ChatJID: team, ID: "7", SenderJID: budi, SenderName: "Budi", TS: at(29 * time.Minute), Text: "Callback URL moved to /v2/notify. Old one stays until Friday.", Forwarded: true, Read: true},
		{ChatJID: team, ID: "8", SenderJID: me, FromMe: true, TS: at(5 * time.Minute), Text: "Thanks, updating the config now.", Status: store.StatusDelivered, Read: true},
		{ChatJID: sari, ID: "s1", SenderJID: sari, TS: at(20 * time.Minute), Text: "Lunch later?", Read: false},
		{ChatJID: andi, ID: "a1", SenderJID: andi, TS: at(26 * time.Hour), Text: "[missed voice call]"},
		{ChatJID: budi, ID: "b1", SenderJID: me, FromMe: true, TS: at(50 * time.Hour), Text: "See you at the meetup!", Status: store.StatusRead, Read: true},
		{ChatJID: "6284444444444@s.whatsapp.net", ID: "d1", SenderJID: "6284444444444@s.whatsapp.net", TS: at(80 * time.Hour), Text: "Happy birthday 🎉🎂", Read: true},
		{ChatJID: "120363000000002@g.us", ID: "f1", SenderJID: andi, TS: at(100 * time.Hour), Text: "[image] Holiday photos", Read: true},
	}
	for _, msg := range msgs {
		if err := st.PutMessage(msg); err != nil {
			t.Fatal(err)
		}
		for e, who := range msg.Reactions {
			_ = st.SetReaction(msg.ChatJID, msg.ID, e, who)
		}
	}
	_ = st.SetPinned(team, true)
	_ = st.SetMutedUntil("120363000000002@g.us", store.MutedForever)
	m := New([]*profile.Profile{personal, work}, config.Default(), nil, Options{})
	m = step(m, tea.WindowSizeMsg{Width: shotW, Height: shotH}, ChangedMsg{}, ChangedMsg{Profile: "work"})
	m.unread["work"] = 4
	return m
}

func TestScreenshots(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	dir := filepath.Join("..", "..", "docs", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shot := func(name string, m Model) {
		page := ansiToHTML(m.View())
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fresh := func() Model { return demoModel(t) }
	openChat := func() Model {
		m := step(fresh(), key("2"), key("enter"))
		m.flash = ""
		return m
	}
	shot("01-chat", openChat())
	compose := step(openChat(), key("i"))
	compose = typeInto(compose, "Deployed 🎉 thanks all :tad")
	shot("02-compose-emoji", compose)
	shot("03-message-menu", step(openChat(), key("k"), key("k"), key("k"), key("k"), key(" ")))
	shot("04-chat-menu", step(fresh(), key("2"), key(" ")))
	info := openChat()
	info.info = &infoState{title: "Backend Team"}
	info = step(info, infoLoadedMsg{group: &wa.Group{JID: "120363000000001@g.us", Name: "Backend Team",
		Topic: "Backend squad · deploys, incidents, reviews.\nPlease keep threads short 🙏", Created: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), CreatedBy: "Andi",
		Members: []wa.Member{
			{JID: "6281200000002@s.whatsapp.net", Name: "You", Phone: "+6281200000002", IsMe: true, Admin: true},
			{JID: "6282222222222@s.whatsapp.net", Name: "Andi", Phone: "+6282222222222", Admin: true, Owner: true},
			{JID: "6283333333333@s.whatsapp.net", Name: "Budi", Phone: "+6283333333333"},
			{JID: "6281111111111@s.whatsapp.net", Name: "Sari", Phone: "+6281111111111"},
		}}})
	shot("05-group-info", info)
	shot("06-profiles", step(fresh(), key("1"), key("enter")))
	shot("07-help", step(openChat(), key("?")))
	cmdShot := typeInto(step(openChat(), key(":")), "s")
	shot("08-commands", cmdShot)
	for _, name := range []string{"catppuccin-mocha", "gruvbox-dark", "nord"} {
		m := openChat()
		if err := m.setTheme(name); err != nil {
			t.Fatal(err)
		}
		shot("theme-"+name, m)
	}
	ApplyTheme(withName(builtinThemes[DefaultTheme], DefaultTheme))
	_ = fmt.Sprint()
}

func withName(t Theme, name string) Theme {
	t.Name = name
	return t
}

type sgr struct {
	fg, bg                       string
	bold, italic, under, reverse bool
}

func (s sgr) css() string {
	fg, bg := s.fg, s.bg
	if s.reverse {
		fg, bg = bg, fg
		if fg == "" {
			fg = "#1a1b26"
		}
		if bg == "" {
			bg = "#c0caf5"
		}
	}
	var b strings.Builder
	if fg != "" {
		b.WriteString("color:" + fg + ";")
	}
	if bg != "" {
		b.WriteString("background:" + bg + ";")
	}
	if s.bold {
		b.WriteString("font-weight:bold;")
	}
	if s.italic {
		b.WriteString("font-style:italic;")
	}
	if s.under {
		b.WriteString("text-decoration:underline;")
	}
	return b.String()
}

func ansiToHTML(view string) string {
	var out strings.Builder
	out.WriteString(`<!doctype html><html><head><meta charset="utf-8"><style>
body{margin:0;background:BGCOLOR;}
pre{margin:0;padding:14px 16px;font-family:"Hack Nerd Font Mono","BlexMono Nerd Font Mono",monospace;font-size:14px;line-height:17px;color:FGCOLOR;background:BGCOLOR;display:inline-block}
span{white-space:pre}
i{font-style:inherit;display:inline-block;width:1ch;height:17px;line-height:17px;overflow:visible;text-align:center;vertical-align:top}
i.w{width:2ch}
</style></head><body><pre>`)
	cur := sgr{}
	open := false
	flush := func() {
		if open {
			out.WriteString("</span>")
			open = false
		}
		if css := cur.css(); css != "" {
			out.WriteString(`<span style="` + css + `">`)
			open = true
		}
	}
	rs := []rune(view)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
			j := i + 2
			for j < len(rs) && !(rs[j] >= '@' && rs[j] <= '~') {
				j++
			}
			if j < len(rs) && rs[j] == 'm' {
				applySGR(&cur, string(rs[i+2:j]))
				flush()
			}
			i = j
			continue
		}
		if r == 0x1b {
			continue
		}
		if r >= 0x10EEEE {
			out.WriteString(" ")
			continue
		}
		if r >= 0x300 && r <= 0x36f && i > 0 && rs[i-1] >= 0x10EEEE {
			continue
		}
		if r == '\n' {
			out.WriteString("\n")
			continue
		}
		cluster := string(r)
		for i+1 < len(rs) && rs[i+1] != 0x1b && rs[i+1] != '\n' && (lipgloss.Width(cluster+string(rs[i+1])) == lipgloss.Width(cluster) && lipgloss.Width(cluster) > 0) {
			i++
			cluster += string(rs[i])
		}
		switch lipgloss.Width(cluster) {
		case 0:
			continue
		case 2:
			out.WriteString(`<i class="w">` + html.EscapeString(cluster) + `</i>`)
		default:
			if r == ' ' || r == '\n' {
				out.WriteString(html.EscapeString(cluster))
			} else {
				out.WriteString(`<i>` + html.EscapeString(cluster) + `</i>`)
			}
		}
	}
	if open {
		out.WriteString("</span>")
	}
	out.WriteString("</pre></body></html>")
	return strings.NewReplacer("BGCOLOR", string(colBg), "FGCOLOR", string(colFg)).Replace(out.String())
}

func applySGR(s *sgr, params string) {
	if params == "" {
		*s = sgr{}
		return
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		p := strings.SplitN(parts[i], ":", 2)[0]
		n, _ := strconv.Atoi(p)
		switch {
		case n == 0:
			*s = sgr{}
		case n == 1:
			s.bold = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.under = true
		case n == 7:
			s.reverse = true
		case n == 22:
			s.bold = false
		case n == 23:
			s.italic = false
		case n == 24:
			s.under = false
		case n == 27:
			s.reverse = false
		case n == 39:
			s.fg = ""
		case n == 49:
			s.bg = ""
		case (n == 38 || n == 48) && i+4 < len(parts) && parts[i+1] == "2":
			r, _ := strconv.Atoi(parts[i+2])
			g, _ := strconv.Atoi(parts[i+3])
			b, _ := strconv.Atoi(parts[i+4])
			col := fmt.Sprintf("#%02x%02x%02x", r, g, b)
			if n == 38 {
				s.fg = col
			} else {
				s.bg = col
			}
			i += 4
		}
	}
}
