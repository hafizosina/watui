package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSingleInstance(t *testing.T) {
	dir := t.TempDir()
	first, err := lockInstance(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	_, err = lockInstance(dir)
	if err == nil {
		t.Fatal("second instance should be refused")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("already running (pid %d", os.Getpid())) {
		t.Fatalf("error should name the running pid, got: %v", err)
	}
	t.Log(err)
	first.release()
	again, err := lockInstance(dir)
	if err != nil {
		t.Fatalf("after release the lock should be free: %v", err)
	}
	again.release()
}

func TestTakeOver(t *testing.T) {
	dir := t.TempDir()
	helper := exec.Command(os.Args[0], "-test.run=TestHelperHoldLock")
	helper.Env = append(os.Environ(), "WATUI_LOCK_DIR="+dir)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer helper.Process.Kill()
	for i := 0; i < 100; i++ {
		l, err := lockInstance(dir)
		if err != nil {
			break
		}
		l.release()
		time.Sleep(50 * time.Millisecond)
	}
	_, err := lockInstance(dir)
	var running *runningError
	if !errors.As(err, &running) || running.pid != helper.Process.Pid {
		t.Fatalf("want lock held by helper pid %d, got %v", helper.Process.Pid, err)
	}
	lock, forced, err := takeOver(dir, running.pid)
	if err != nil {
		t.Fatalf("lock should be free after the other copy is closed: %v", err)
	}
	if forced {
		t.Error("a copy that exits on SIGTERM should not need SIGKILL")
	}
	lock.release()
}

func TestHelperHoldLock(t *testing.T) {
	dir := os.Getenv("WATUI_LOCK_DIR")
	if dir == "" {
		t.Skip("helper only")
	}
	if _, err := lockInstance(dir); err != nil {
		os.Exit(3)
	}
	time.Sleep(30 * time.Second)
}
