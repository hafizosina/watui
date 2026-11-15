package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/player"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/ui"
	"github.com/zhenzhu/watui/internal/wa"
)

var version = "dev"

const usage = `watui — WhatsApp in the terminal, vim style

usage:
  watui          open the TUI (asks for a QR scan the first time)
  watui login    link this computer to your phone (QR code)
  watui logout   unlink this computer from your WhatsApp account
  watui version  print the version
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "":
		err = run(false)
	case "login":
		err = run(true)
	case "logout":
		err = logout()
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

type app struct {
	client *whatsmeow.Client
	store  *store.Store
	log    waLog.Logger
	close  func()
}

func execOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func open() (*app, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	lock, err := lockOrTakeOver(dir)
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "watui.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	level := zerolog.WarnLevel
	if os.Getenv("WATUI_DEBUG") != "" {
		level = zerolog.DebugLevel
	}
	log := waLog.Zerolog(zerolog.New(logFile).Level(level).With().Timestamp().Logger())

	waStore.SetOSInfo("watui", [3]uint32{0, 1, 0})
	waStore.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	waStore.DeviceProps.RequireFullSync = proto.Bool(false)

	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+filepath.Join(dir, "whatsapp.db")+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", log.Sub("db"))
	if err != nil {
		return nil, fmt.Errorf("open whatsapp store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("load device: %w", err)
	}

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "messages.db")+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	st, err := store.Open(db)
	if err != nil {
		return nil, fmt.Errorf("open message store: %w", err)
	}

	client := whatsmeow.NewClient(device, log.Sub("client"))
	return &app{
		client: client,
		store:  st,
		log:    log,
		close: func() {
			client.Disconnect()
			_ = db.Close()
			_ = container.Close()
			_ = logFile.Close()
			lock.release()
		},
	}, nil
}

func login(cli *whatsmeow.Client) error {
	qrChan, err := cli.GetQRChannel(context.Background())
	if err != nil {
		return err
	}
	if err := cli.Connect(); err != nil {
		return err
	}
	fmt.Println("On your phone: WhatsApp → Settings → Linked devices → Link a device, then scan:")
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Print("\033[H\033[2J")
			fmt.Println("On your phone: WhatsApp → Settings → Linked devices → Link a device, then scan:")
			fmt.Println()
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			fmt.Println("\nThe code refreshes every ~20 seconds. Ctrl+C to cancel.")
		case "success":
			fmt.Println("\nLinked! Loading your chats…")
			return nil
		case "timeout":
			return errors.New("QR code timed out, run `watui login` again")
		default:
			if evt.Error != nil {
				return evt.Error
			}
		}
	}
	return errors.New("login cancelled")
}

func run(forceLogin bool) error {
	a, err := open()
	if err != nil {
		return err
	}
	defer a.close()

	client := wa.New(a.client, a.store, a.log)

	if a.client.Store.ID == nil {
		if err := login(a.client); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)
	} else {
		if forceLogin {
			fmt.Println("Already linked. Use `watui logout` first to link again.")
			return nil
		}
		if err := a.client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "watui: config:", cfgErr)
		time.Sleep(2 * time.Second)
	}
	images, err := media.NewManager(client.LoadImage, client.Notify)
	if err != nil {
		a.log.Warnf("images disabled: %v", err)
		images = nil
	}
	if images != nil {
		defer images.Close()
	}

	pl := player.New(client.Notify)
	defer pl.Stop()

	p := tea.NewProgram(ui.New(client, cfg, images, pl), tea.WithAltScreen(), tea.WithReportFocus())
	go func() {
		for range client.Changed() {
			time.Sleep(150 * time.Millisecond)
			p.Send(ui.ChangedMsg{})
		}
	}()
	os.Stdout.WriteString("\x1b[?7l")
	defer os.Stdout.WriteString("\x1b[?7h")
	_, err = p.Run()
	return err
}

func logout() error {
	a, err := open()
	if err != nil {
		return err
	}
	defer a.close()
	if a.client.Store.ID == nil {
		fmt.Println("Not linked.")
		return nil
	}
	if err := a.client.Connect(); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	if err := a.client.Logout(context.Background()); err != nil {
		return err
	}
	fmt.Println("Unlinked. Saved chats stay in ~/.local/share/watui (delete that folder to remove them).")
	return nil
}
