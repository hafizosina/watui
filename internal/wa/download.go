package wa

import (
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// downloadStall cancels a media download that received nothing for this long.
// There is no limit on the whole download: a 2 GB document takes minutes.
var downloadStall = 60 * time.Second

type progress struct {
	done, total atomic.Int64
}

var downloads sync.Map // message ID → *progress, while it downloads

// DownloadProgress reports a media download in flight: bytes received so far and
// the file's size (0 when WhatsApp didn't say).
func DownloadProgress(msgID string) (done, total int64, ok bool) {
	v, ok := downloads.Load(msgID)
	if !ok {
		return 0, 0, false
	}
	p := v.(*progress)
	return p.done.Load(), p.total.Load(), true
}

// Downloading reports whether any media download is in flight.
func Downloading() bool {
	busy := false
	downloads.Range(func(_, _ any) bool {
		busy = true
		return false
	})
	return busy
}

// downloadToFile streams msg's media into path, decrypted, and tracks progress
// under msgID.
func (c *Client) downloadToFile(ctx context.Context, msgID string, msg downloadable, path string) error {
	p := &progress{}
	if f, ok := msg.(interface{ GetFileLength() uint64 }); ok {
		p.total.Store(int64(f.GetFileLength()))
	}
	downloads.Store(msgID, p)
	defer downloads.Delete(msgID)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// The caller's deadline is for a small file; a big one only stops when it stalls.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	go watchStall(ctx, cancel, p)

	err = c.WA.DownloadToFile(ctx, msg, &countingFile{f: f, p: p})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		if ctx.Err() != nil {
			return errDownloadStalled
		}
	}
	return err
}

type stalledError struct{}

func (stalledError) Error() string { return "download stalled: nothing received for a minute" }

var errDownloadStalled error = stalledError{}

func watchStall(ctx context.Context, cancel func(), p *progress) {
	t := time.NewTicker(downloadStall / 6)
	defer t.Stop()
	last, since := p.done.Load(), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if d := p.done.Load(); d != last {
				last, since = d, now
			} else if now.Sub(since) >= downloadStall {
				cancel()
				return
			}
		}
	}
}

// countingFile is the file whatsmeow downloads into. Its sequential writes are
// the bytes arriving from WhatsApp; decrypting afterwards uses WriteAt and
// doesn't count. Wrapping (not embedding) *os.File keeps io.Copy off ReadFrom,
// which would bypass Write.
type countingFile struct {
	f   *os.File
	pos int64
	p   *progress
}

func (c *countingFile) Write(b []byte) (int, error) {
	n, err := c.f.Write(b)
	c.pos += int64(n)
	c.p.done.Store(c.pos)
	return n, err
}

func (c *countingFile) Seek(offset int64, whence int) (int64, error) {
	pos, err := c.f.Seek(offset, whence)
	if err == nil {
		c.pos = pos // a retry starts over from 0
	}
	return pos, err
}

func (c *countingFile) Read(b []byte) (int, error) {
	n, err := c.f.Read(b)
	c.pos += int64(n)
	return n, err
}

func (c *countingFile) ReadAt(b []byte, off int64) (int, error)  { return c.f.ReadAt(b, off) }
func (c *countingFile) WriteAt(b []byte, off int64) (int, error) { return c.f.WriteAt(b, off) }
func (c *countingFile) Truncate(size int64) error                { return c.f.Truncate(size) }
func (c *countingFile) Stat() (os.FileInfo, error)               { return c.f.Stat() }

var _ io.ReadWriteSeeker = (*countingFile)(nil)

// FakeDownload shows msgID as downloading done of total bytes until stop is
// called; for tests of the progress display.
func FakeDownload(msgID string, done, total int64) (stop func()) {
	p := &progress{}
	p.done.Store(done)
	p.total.Store(total)
	downloads.Store(msgID, p)
	return func() { downloads.Delete(msgID) }
}
