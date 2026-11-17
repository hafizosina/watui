package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type instanceLock struct {
	f *os.File
}

type runningError struct {
	pid  int
	exe  string
	tty  string
	pane string
}

func (e *runningError) Error() string {
	return "watui is already running" + e.summary() + ".\nClose it first (two copies share one WhatsApp login and kick each other off)"
}

func (e *runningError) summary() string {
	if e.pid <= 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("pid %d", e.pid)}
	for _, p := range []string{e.exe, e.tty, e.pane} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func lockInstance(dir string) (*instanceLock, error) {
	path := filepath.Join(dir, "watui.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		data, _ := os.ReadFile(path)
		f.Close()
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return nil, describe(pid)
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return &instanceLock{f: f}, nil
}

func (l *instanceLock) release() {
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	_ = l.f.Close()
}

func describe(pid int) *runningError {
	e := &runningError{pid: pid}
	if pid <= 0 {
		return e
	}
	e.exe, e.tty = processInfo(pid)
	if e.tty != "" {
		e.pane = tmuxPaneFor(e.tty)
	}
	return e
}

// processInfo finds a process's program and terminal: from /proc on Linux,
// from ps elsewhere (macOS).
func processInfo(pid int) (exe, tty string) {
	if p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		exe = p
		if t, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", pid)); err == nil && strings.HasPrefix(t, "/dev/pts/") {
			tty = t
		}
		return exe, tty
	}
	out, err := execOutput("ps", "-o", "tty=", "-o", "comm=", "-p", strconv.Itoa(pid))
	if err != nil {
		return "", ""
	}
	return parsePS(out)
}

// parsePS reads "ttys003 /Applications/kitty.app/…/watui" (ps -o tty= -o comm=).
func parsePS(out string) (exe, tty string) {
	t, comm, _ := strings.Cut(strings.TrimSpace(out), " ")
	exe = strings.TrimSpace(comm)
	if t != "" && t != "??" && t != "?" {
		tty = "/dev/" + t
	}
	return exe, tty
}

func tmuxPaneFor(tty string) string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := execOutput("tmux", "list-panes", "-a", "-F", "#{pane_tty} #{session_name}:#{window_index} (#{window_name})")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if t, rest, ok := strings.Cut(line, " "); ok && t == tty {
			return "window " + rest
		}
	}
	return ""
}

// lockOrTakeOver asks, on a terminal, whether to close the running copy, then waits for its lock.
func lockOrTakeOver(dir string) (*instanceLock, error) {
	lock, err := lockInstance(dir)
	var running *runningError
	if err == nil || !errors.As(err, &running) || running.pid <= 0 || !isTerminal() {
		return lock, err
	}
	return takeOverPrompt(dir, running)
}

func takeOver(dir string, pid int) (*instanceLock, bool, error) {
	_ = unix.Kill(pid, unix.SIGTERM)
	if lock, err := waitForLock(dir, 5*time.Second); err == nil {
		return lock, false, nil
	}
	_ = unix.Kill(pid, unix.SIGKILL)
	lock, err := waitForLock(dir, 2*time.Second)
	if err != nil {
		return nil, true, fmt.Errorf("could not close pid %d: %w", pid, err)
	}
	return lock, true, nil
}

func waitForLock(dir string, timeout time.Duration) (*instanceLock, error) {
	deadline := time.Now().Add(timeout)
	for {
		lock, err := lockInstance(dir)
		if err == nil || time.Now().After(deadline) {
			return lock, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
