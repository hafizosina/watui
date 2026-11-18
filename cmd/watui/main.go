package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hafizosina/watui/internal/config"
	"github.com/hafizosina/watui/internal/notify"
	"github.com/hafizosina/watui/internal/player"
	"github.com/hafizosina/watui/internal/profile"
	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/ui"
	"github.com/hafizosina/watui/internal/wa"
)

// version is set at build time (-X main.version=v0.1.0, see Makefile and the
// release workflow). `go install …@v0.1.0` doesn't set it, so fall back to the
// module version Go records in the binary.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
}

const usage = `watui — WhatsApp in the terminal, vim style

usage:
  watui                  open the TUI with all profiles (asks for a QR scan the first time)
  watui login            link the default profile (QR code)
  watui add <name>       add another WhatsApp account as a profile (QR code)
  watui profiles         list profiles
  watui logout [name]    unlink a profile (default: "default")
  watui version          print the version

Inside watui: [1] Profiles box, gt/gT next/previous profile, :profile <name>.
`

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	var err error
	switch cmd {
	case "":
		err = run()
	case "login":
		err = add(profile.Default)
	case "add":
		if arg == "" {
			err = errors.New("usage: watui add <name>")
		} else {
			err = add(arg)
		}
	case "profiles":
		err = listProfiles()
	case "logout":
		if arg == "" {
			arg = profile.Default
		}
		err = logout(arg)
	case "version", "-v", "--version":
		fmt.Println("watui", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "watui:", err)
		os.Exit(1)
	}
}

func dataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "watui")
	return dir, os.MkdirAll(dir, 0o700)
}

func execOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func locked() (string, *instanceLock, error) {
	root, err := dataDir()
	if err != nil {
		return "", nil, err
	}
	lock, err := lockOrTakeOver(root)
	return root, lock, err
}

func run() error {
	root, lock, err := locked()
	if err != nil {
		return err
	}
	defer lock.release()

	var profiles []*profile.Profile
	defer func() {
		for _, p := range profiles {
			p.Close()
		}
	}()
	for _, name := range profile.List(root) {
		p, err := profile.Open(root, name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "watui:", err)
			continue
		}
		profiles = append(profiles, p)
	}
	if len(profiles) == 0 {
		return errors.New("no profile could be opened")
	}

	anyLinked := false
	for _, p := range profiles {
		anyLinked = anyLinked || p.Linked()
	}
	if !anyLinked {
		if err := profiles[0].Login(os.Stdout, nil); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)
	}
	for _, p := range profiles {
		if p.Linked() && !p.WA.WA.IsConnected() {
			if err := p.Connect(); err != nil {
				p.Log.Errorf("connect: %v", err)
			}
		}
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "watui: config:", cfgErr)
		time.Sleep(2 * time.Second)
	}

	var prog *tea.Program
	notifier := notify.New(func(key, action string) {
		if prog == nil {
			return
		}
		name, jid := ui.SplitNoteKey(key)
		if callID, ok := strings.CutPrefix(action, "decline:"); ok {
			prog.Send(ui.DeclineCallMsg{Profile: name, CallID: callID})
			return
		}
		prog.Send(ui.OpenChatMsg{Profile: name, JID: jid})
	})
	ui.ApplySounds(notifier, cfg)
	watch := func(p *profile.Profile) {
		name := p.Name
		p.WA.OnCall(func(ev wa.CallEvent) {
			if prog != nil {
				prog.Send(ui.CallMsg{Profile: name, Ev: ev})
			}
		})
		p.WA.OnIncoming(func(msg store.Message) {
			if prog != nil {
				prog.Send(ui.IncomingMsg{Profile: name, Msg: msg})
			}
		})
		go func() {
			// show a change at once, then merge whatever arrives in the next 150 ms
			// into one follow-up refresh (Changed holds at most one pending signal)
			for range p.WA.Changed() {
				prog.Send(ui.ChangedMsg{Profile: name})
				time.Sleep(150 * time.Millisecond)
			}
		}()
	}
	pl := player.New(func() {
		if prog != nil {
			prog.Send(ui.ChangedMsg{})
		}
	})
	defer pl.Stop()

	active := profiles[:0:0]
	for _, p := range profiles {
		if p.Linked() {
			active = append(active, p)
		}
	}
	ordered := append(active, unlinked(profiles)...)
	model := ui.New(ordered, cfg, pl, ui.Options{
		Root:     root,
		Notifier: notifier,
		Watch: func(p *profile.Profile) {
			profiles = append(profiles, p)
			watch(p)
		},
	})
	prog = tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())
	for _, p := range profiles {
		watch(p)
	}
	os.Stdout.WriteString("\x1b[?7l")
	defer os.Stdout.WriteString("\x1b[?7h")
	final, err := prog.Run()
	if fm, ok := final.(ui.Model); ok {
		fm.SaveDraft() // unsent text in Compose survives quitting
	}
	return err
}

func unlinked(ps []*profile.Profile) []*profile.Profile {
	var out []*profile.Profile
	for _, p := range ps {
		if !p.Linked() {
			out = append(out, p)
		}
	}
	return out
}

func add(name string) error {
	if err := profile.ValidName(name); err != nil {
		return err
	}
	root, lock, err := locked()
	if err != nil {
		return err
	}
	defer lock.release()
	p, err := profile.Open(root, name)
	if err != nil {
		return err
	}
	defer p.Close()
	if p.Linked() {
		fmt.Printf("Profile %q is already linked. Use `watui logout %s` first to link again.\n", name, name)
		return nil
	}
	if err := p.Login(os.Stdout, nil); err != nil {
		if name != profile.Default {
			p.Close()
			_ = profile.Remove(root, name)
		}
		return err
	}
	time.Sleep(3 * time.Second)
	fmt.Printf("Profile %q is ready. Run watui to use it.\n", name)
	return nil
}

func listProfiles() error {
	root, err := dataDir()
	if err != nil {
		return err
	}
	for _, name := range profile.List(root) {
		dir := profile.DirFor(root, name)
		label := name
		if data, err := os.ReadFile(filepath.Join(dir, "label")); err == nil {
			label = fmt.Sprintf("%s (%s)", strings.TrimSpace(string(data)), name)
		}
		_, err := os.Stat(filepath.Join(dir, "whatsapp.db"))
		state := "not linked"
		if err == nil {
			state = "has login data"
		}
		fmt.Printf("  %-28s %s\n", label, state)
	}
	return nil
}

func logout(name string) error {
	root, lock, err := locked()
	if err != nil {
		return err
	}
	defer lock.release()
	p, err := profile.Open(root, name)
	if err != nil {
		return err
	}
	defer p.Close()
	if !p.Linked() {
		fmt.Printf("Profile %q is not linked.\n", name)
		return nil
	}
	if err := p.Connect(); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	if err := p.Logout(); err != nil {
		return err
	}
	fmt.Printf("Unlinked profile %q. Its saved chats stay in %s.\n", name, profile.DirFor(root, name))
	return nil
}
