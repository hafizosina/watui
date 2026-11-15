package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var audioArgs []string

type State struct {
	MsgID    string
	Position time.Duration
	Duration time.Duration
	Paused   bool
	Speed    float64
}

type Player struct {
	notify func()

	mu       sync.Mutex
	cmd      *exec.Cmd
	sock     string
	msgID    string
	duration time.Duration
	played   time.Duration
	since    time.Time
	paused   bool
	speed    float64
}

func New(notify func()) *Player {
	return &Player{notify: notify, speed: 1}
}

func HasMPV() bool {
	_, err := exec.LookPath("mpv")
	return err == nil
}

func (p *Player) State() (State, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return State{}, false
	}
	return State{MsgID: p.msgID, Position: p.position(), Duration: p.duration, Paused: p.paused, Speed: p.speed}, true
}

func (p *Player) position() time.Duration {
	pos := p.played
	if !p.paused {
		pos += time.Duration(float64(time.Since(p.since)) * p.speed)
	}
	if p.duration > 0 && pos > p.duration {
		pos = p.duration
	}
	return pos
}

func (p *Player) PlayAudio(msgID, path string, duration time.Duration) error {
	if !HasMPV() {
		return errors.New("install mpv to play audio")
	}
	p.Stop()
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("watui-mpv-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(sock)
	args := append([]string{"--no-video", "--really-quiet", "--no-terminal", "--input-ipc-server=" + sock}, audioArgs...)
	cmd := exec.Command("mpv", append(args, path)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.mu.Lock()
	p.cmd, p.sock, p.msgID, p.duration = cmd, sock, msgID, duration
	p.played, p.since, p.paused, p.speed = 0, time.Now(), false, 1
	p.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		if p.cmd == cmd {
			p.cmd, p.msgID = nil, ""
		}
		p.mu.Unlock()
		_ = os.Remove(sock)
		p.notify()
	}()
	go p.notify()
	return nil
}

func (p *Player) send(args ...any) error {
	p.mu.Lock()
	sock := p.sock
	running := p.cmd != nil
	p.mu.Unlock()
	if !running {
		return errors.New("nothing is playing")
	}
	var conn net.Conn
	var err error
	for i := 0; i < 20; i++ {
		if conn, err = net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	data, err := json.Marshal(map[string]any{"command": args})
	if err != nil {
		return err
	}
	_, err = conn.Write(append(data, '\n'))
	return err
}

func (p *Player) TogglePause() error {
	if err := p.send("cycle", "pause"); err != nil {
		return err
	}
	p.mu.Lock()
	if p.paused {
		p.since = time.Now()
	} else {
		p.played = p.position()
	}
	p.paused = !p.paused
	p.mu.Unlock()
	go p.notify()
	return nil
}

func (p *Player) CycleSpeed() (float64, error) {
	p.mu.Lock()
	next := map[float64]float64{1: 1.5, 1.5: 2, 2: 1}[p.speed]
	if next == 0 {
		next = 1
	}
	p.mu.Unlock()
	if err := p.send("set_property", "speed", next); err != nil {
		return 0, err
	}
	p.mu.Lock()
	p.played = p.position()
	p.since = time.Now()
	p.speed = next
	p.mu.Unlock()
	go p.notify()
	return next, nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	cmd := p.cmd
	p.cmd, p.msgID = nil, ""
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// OpenVideo plays a video in its own mpv window (looping for GIFs), or the desktop default player.
func OpenVideo(path string, loop bool) error {
	if HasMPV() {
		args := []string{"--force-window=yes", "--keep-open=yes", "--really-quiet", "--no-terminal"}
		if loop {
			args = append(args, "--loop-file=inf")
		}
		cmd := exec.Command("mpv", append(args, path)...)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
