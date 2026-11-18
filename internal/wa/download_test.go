package wa

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCountingFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "x.part"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := &progress{}
	c := &countingFile{f: f, p: p}

	if _, err := io.Copy(c, strings.NewReader(strings.Repeat("a", 5000))); err != nil {
		t.Fatal(err)
	}
	if got := p.done.Load(); got != 5000 {
		t.Fatalf("io.Copy should go through Write: done = %d", got)
	}
	// checking and decrypting afterwards (seek, read, WriteAt) is not download progress
	if _, err := c.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	if _, err := c.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := c.WriteAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if got := p.done.Load(); got != 5000 {
		t.Fatalf("decrypting changed progress: done = %d", got)
	}
	// a retry seeks back to 0 and writes again
	if _, err := c.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(buf); err != nil {
		t.Fatal(err)
	}
	if got := p.done.Load(); got != 100 {
		t.Fatalf("a retry should restart progress: done = %d", got)
	}
}

func TestDownloadProgress(t *testing.T) {
	if _, _, ok := DownloadProgress("M1"); ok || Downloading() {
		t.Fatal("nothing should be downloading")
	}
	stop := FakeDownload("M1", 10, 40)
	done, total, ok := DownloadProgress("M1")
	if !ok || done != 10 || total != 40 || !Downloading() {
		t.Fatalf("got %d/%d ok=%v", done, total, ok)
	}
	stop()
	if Downloading() {
		t.Fatal("stop should end it")
	}
}

func TestWatchStall(t *testing.T) {
	old := downloadStall
	downloadStall = 60 * time.Millisecond
	defer func() { downloadStall = old }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &progress{}
	go func() { // bytes keep arriving for a while: no cancel
		for i := 1; i <= 10; i++ {
			p.done.Store(int64(i))
			time.Sleep(20 * time.Millisecond)
		}
	}()
	start := time.Now()
	watchStall(ctx, cancel, p)
	if ctx.Err() == nil {
		t.Fatal("watchStall returned without cancelling")
	}
	if took := time.Since(start); took < 200*time.Millisecond {
		t.Fatalf("cancelled after %v while data was still arriving", took)
	}
}
