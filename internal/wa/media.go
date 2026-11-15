package wa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow/proto/waE2E"
	_ "golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/media"
)

const (
	MediaImage   = "image"
	MediaSticker = "sticker"
	MediaVideo   = "video"
	MediaAudio   = "audio"
)

func mediaOf(m *waE2E.Message) (string, []byte) {
	var kind string
	var part proto.Message
	switch {
	case m.GetImageMessage() != nil:
		kind, part = MediaImage, m.GetImageMessage()
	case m.GetStickerMessage() != nil:
		kind, part = MediaSticker, m.GetStickerMessage()
	case m.GetVideoMessage() != nil:
		kind, part = MediaVideo, m.GetVideoMessage()
	case m.GetPtvMessage() != nil:
		kind, part = MediaVideo, m.GetPtvMessage()
	case m.GetAudioMessage() != nil:
		kind, part = MediaAudio, m.GetAudioMessage()
	default:
		return "", nil
	}
	data, err := proto.Marshal(part)
	if err != nil {
		return "", nil
	}
	return kind, data
}

type downloadable interface {
	proto.Message
	GetDirectPath() string
	GetMediaKey() []byte
	GetFileSHA256() []byte
	GetFileEncSHA256() []byte
	GetMimetype() string
}

func (c *Client) mediaMessage(chatJID, msgID string) (string, downloadable, error) {
	kind, data, err := c.Store.Media(chatJID, msgID)
	if err != nil {
		return "", nil, err
	}
	var msg downloadable
	switch kind {
	case MediaImage:
		msg = &waE2E.ImageMessage{}
	case MediaSticker:
		msg = &waE2E.StickerMessage{}
	case MediaVideo:
		msg = &waE2E.VideoMessage{}
	case MediaAudio:
		msg = &waE2E.AudioMessage{}
	default:
		return "", nil, errors.New("no media in this message")
	}
	if err := proto.Unmarshal(data, msg); err != nil {
		return "", nil, err
	}
	return kind, msg, nil
}

func extFor(kind, mime string) string {
	mime = strings.ToLower(mime)
	switch {
	case strings.Contains(mime, "ogg"), strings.Contains(mime, "opus"):
		return ".ogg"
	case strings.Contains(mime, "mpeg") && kind == MediaAudio:
		return ".mp3"
	case strings.Contains(mime, "mp4") && kind == MediaAudio, strings.Contains(mime, "aac"):
		return ".m4a"
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "webm"):
		return ".webm"
	}
	switch kind {
	case MediaImage:
		return ".jpg"
	case MediaVideo:
		return ".mp4"
	case MediaAudio:
		return ".ogg"
	case MediaSticker:
		return ".webp"
	}
	return ".bin"
}

func cacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "watui", "media")
	return dir, os.MkdirAll(dir, 0o700)
}

var downloadLocks sync.Map

// MediaFile downloads (once) and returns the path of the decrypted media file.
func (c *Client) MediaFile(ctx context.Context, chatJID, msgID string) (string, string, error) {
	kind, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return "", "", err
	}
	dir, err := cacheDir()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, safeName(msgID)+extFor(kind, msg.GetMimetype()))

	lock, _ := downloadLocks.LoadOrStore(path, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	if _, err := os.Stat(path); err == nil {
		return path, kind, nil
	}
	data, err := c.WA.Download(ctx, msg)
	if err != nil {
		return "", "", err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", "", err
	}
	return path, kind, os.Rename(tmp, path)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' {
			return '_'
		}
		return r
	}, s)
}

func (c *Client) LoadImage(ctx context.Context, chatJID, msgID string, q media.Quality) (image.Image, error) {
	kind, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return nil, err
	}
	var raw []byte
	switch m := msg.(type) {
	case *waE2E.ImageMessage:
		if q == media.Thumbnail {
			raw = m.GetJPEGThumbnail()
		} else if raw, err = c.WA.Download(ctx, m); err != nil {
			return nil, err
		}
	case *waE2E.StickerMessage:
		if m.GetIsAnimated() {
			return nil, errors.New("animated sticker")
		}
		if raw, err = c.WA.Download(ctx, m); err != nil {
			return nil, err
		}
	case *waE2E.VideoMessage:
		if q == media.Full {
			return c.videoFrame(ctx, chatJID, msgID)
		}
		raw = m.GetJPEGThumbnail()
		if len(raw) == 0 {
			return nil, errors.New("no preview (Space → Show image grabs a frame)")
		}
	default:
		return nil, fmt.Errorf("%s has no picture", kind)
	}
	if len(raw) == 0 {
		return nil, errors.New("no preview available")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

func (c *Client) videoFrame(ctx context.Context, chatJID, msgID string) (image.Image, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errors.New("install ffmpeg to grab a video frame")
	}
	path, _, err := c.MediaFile(ctx, chatJID, msgID)
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-ss", "0.5", "-i", path,
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil || len(out) == 0 {
		out, err = exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-i", path,
			"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg: %w", err)
		}
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	return img, err
}

// SaveMedia copies the media file to ~/Downloads/WhatsApp and returns the new path.
func (c *Client) SaveMedia(ctx context.Context, chatJID, msgID string, when string) (string, error) {
	src, _, err := c.MediaFile(ctx, chatJID, msgID)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads", "WhatsApp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, when+"-"+safeName(msgID)+filepath.Ext(src))
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	return dst, os.WriteFile(dst, data, 0o644)
}
