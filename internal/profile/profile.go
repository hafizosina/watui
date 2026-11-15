package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

const Default = "default"

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,23}$`)

type Profile struct {
	Name   string
	Dir    string
	WA     *wa.Client
	Images *media.Manager
	Log    waLog.Logger
	close  func()
	once   sync.Once
}

func init() {
	waStore.SetOSInfo("watui", [3]uint32{0, 1, 0})
	waStore.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	waStore.DeviceProps.RequireFullSync = proto.Bool(false)
}

func ValidName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("profile name %q: use lowercase letters, digits, - or _ (max 24)", name)
	}
	return nil
}

func DirFor(root, name string) string {
	if name == Default {
		return root
	}
	return filepath.Join(root, "profiles", name)
}

// List returns the profile names that exist on disk, default first.
func List(root string) []string {
	names := []string{Default}
	entries, err := os.ReadDir(filepath.Join(root, "profiles"))
	if err != nil {
		return names
	}
	var extra []string
	for _, e := range entries {
		if e.IsDir() && ValidName(e.Name()) == nil && e.Name() != Default {
			extra = append(extra, e.Name())
		}
	}
	sort.Strings(extra)
	return append(names, extra...)
}

func Open(root, name string) (*Profile, error) {
	if err := ValidName(name); err != nil {
		return nil, err
	}
	dir := DirFor(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	log := waLog.Zerolog(zerolog.New(logFile).Level(level).With().Timestamp().Str("profile", name).Logger())

	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+filepath.Join(dir, "whatsapp.db")+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", log.Sub("db"))
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("%s: open whatsapp store: %w", name, err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: load device: %w", name, err)
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "messages.db")+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	st, err := store.Open(db)
	if err != nil {
		return nil, fmt.Errorf("%s: open message store: %w", name, err)
	}

	raw := whatsmeow.NewClient(device, log.Sub("client"))
	client := wa.New(raw, st, log)
	p := &Profile{Name: name, Dir: dir, WA: client, Log: log}
	if images, err := media.NewManager(client.LoadImage, client.Notify); err == nil {
		p.Images = images
	} else {
		log.Warnf("images disabled: %v", err)
	}
	p.close = func() {
		if p.Images != nil {
			p.Images.Close()
		}
		raw.Disconnect()
		_ = db.Close()
		_ = container.Close()
		_ = logFile.Close()
	}
	return p, nil
}

func (p *Profile) Label() string {
	data, err := os.ReadFile(filepath.Join(p.Dir, "label"))
	if err == nil {
		if l := strings.TrimSpace(string(data)); l != "" {
			return l
		}
	}
	return p.Name
}

func (p *Profile) SetLabel(label string) error {
	label = strings.TrimSpace(label)
	if label == "" || label == p.Name {
		return os.Remove(filepath.Join(p.Dir, "label"))
	}
	return os.WriteFile(filepath.Join(p.Dir, "label"), []byte(label+"\n"), 0o600)
}

const notifyOffFile = "notify-off"

// NotifyEnabled reports whether desktop notifications are on for this profile.
func (p *Profile) NotifyEnabled() bool {
	_, err := os.Stat(filepath.Join(p.Dir, notifyOffFile))
	return err != nil
}

func (p *Profile) SetNotify(on bool) error {
	path := filepath.Join(p.Dir, notifyOffFile)
	if on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, nil, 0o600)
}

func (p *Profile) Reconnect() error {
	p.WA.WA.Disconnect()
	return p.WA.WA.Connect()
}

func (p *Profile) Linked() bool {
	return p.WA.WA.Store.ID != nil
}

func (p *Profile) Connect() error {
	return p.WA.WA.Connect()
}

func (p *Profile) Close() {
	p.once.Do(func() {
		if p.close != nil {
			p.close()
		}
	})
}

// Login shows a QR code on out until the phone links this profile, or cancel is closed.
func (p *Profile) Login(out io.Writer, cancel <-chan struct{}) error {
	cli := p.WA.WA
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		select {
		case <-cancel:
			stop()
		case <-ctx.Done():
		}
	}()
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := cli.Connect(); err != nil {
		return err
	}
	header := fmt.Sprintf("Linking profile %q — on the phone for this account: WhatsApp → Settings → Linked devices → Link a device, then scan:", p.Name)
	fmt.Fprintln(out, header)
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Fprint(out, "\033[H\033[2J")
			fmt.Fprintln(out, header)
			fmt.Fprintln(out)
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, out)
			fmt.Fprintln(out, "\nThe code refreshes every ~20 seconds.")
			if cancel != nil {
				fmt.Fprintln(out, "Press Enter to cancel.")
			} else {
				fmt.Fprintln(out, "Ctrl+C to cancel.")
			}
		case "success":
			fmt.Fprintln(out, "\nLinked! Loading chats…")
			return nil
		case "timeout":
			cli.Disconnect()
			return errors.New("QR code timed out")
		default:
			if evt.Error != nil {
				cli.Disconnect()
				return evt.Error
			}
		}
	}
	cli.Disconnect()
	return errors.New("login cancelled")
}

func (p *Profile) Logout() error {
	if !p.Linked() {
		return errors.New("not linked")
	}
	return p.WA.WA.Logout(context.Background())
}

// Remove deletes a profile that was never linked (e.g. a cancelled add).
func Remove(root, name string) error {
	if name == Default {
		return errors.New("can't remove the default profile")
	}
	return os.RemoveAll(DirFor(root, name))
}
