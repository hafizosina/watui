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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	_ "golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"

	"github.com/hafizosina/watui/internal/media"
)

const (
	MediaImage   = "image"
	MediaSticker = "sticker"
	MediaVideo   = "video"
	MediaAudio   = "audio"
	MediaDoc     = "document"
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
	case m.GetDocumentMessage() != nil:
		kind, part = MediaDoc, m.GetDocumentMessage()
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
	case MediaDoc:
		msg = &waE2E.DocumentMessage{}
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
	ext := extFor(kind, msg.GetMimetype())
	if d, ok := msg.(*waE2E.DocumentMessage); ok {
		if e := filepath.Ext(d.GetFileName()); e != "" && len(e) <= 10 {
			ext = strings.ToLower(e)
		}
	}
	path := filepath.Join(dir, safeName(msgID)+ext)

	lock, _ := downloadLocks.LoadOrStore(path, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	if _, err := os.Stat(path); err == nil {
		return path, kind, nil
	}
	tmp := path + ".part"
	if err := c.downloadToFile(ctx, msgID, msg, tmp); err != nil {
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

func still(img image.Image, err error) ([]media.Frame, error) {
	if err != nil {
		return nil, err
	}
	return []media.Frame{{Img: img}}, nil
}

func decode(raw []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

func (c *Client) LoadImage(ctx context.Context, chatJID, msgID string, q media.Quality) ([]media.Frame, error) {
	if strings.HasPrefix(msgID, PicturePrefix) {
		return still(c.loadPicture(ctx, msgID))
	}
	kind, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return nil, err
	}
	switch m := msg.(type) {
	case *waE2E.ImageMessage:
		if q == media.Thumbnail {
			if len(m.GetJPEGThumbnail()) == 0 {
				return nil, errors.New("no preview available")
			}
			return still(decode(m.GetJPEGThumbnail()))
		}
		raw, err := c.WA.Download(ctx, m)
		if err != nil {
			return nil, err
		}
		return still(decode(raw))
	case *waE2E.StickerMessage:
		path, _, err := c.MediaFile(ctx, chatJID, msgID)
		if err != nil {
			return nil, err
		}
		if m.GetIsAnimated() || m.GetIsLottie() {
			return animatedWebP(ctx, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		img, err := decode(raw)
		if err != nil {
			return animatedWebP(ctx, path)
		}
		return []media.Frame{{Img: img}}, nil
	case *waE2E.VideoMessage:
		if m.GetGifPlayback() && q == media.Full {
			return c.gifFrames(ctx, chatJID, msgID)
		}
		if q == media.Full {
			return still(c.videoFrame(ctx, chatJID, msgID))
		}
		if len(m.GetJPEGThumbnail()) == 0 {
			return nil, errors.New("no preview (Space → Show image grabs a frame)")
		}
		return still(decode(m.GetJPEGThumbnail()))
	}
	return nil, fmt.Errorf("%s has no picture", kind)
}

// animatedWebP splits an animated sticker into frames with ImageMagick.
func animatedWebP(ctx context.Context, path string) ([]media.Frame, error) {
	if _, err := exec.LookPath("convert"); err != nil {
		return nil, errors.New("install imagemagick to show animated stickers")
	}
	dir, err := os.MkdirTemp("", "watui-anim-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if out, err := exec.CommandContext(ctx, "convert", path, "-coalesce", filepath.Join(dir, "f_%03d.png")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("imagemagick: %v %s", err, strings.TrimSpace(string(out)))
	}
	var delays []string
	if out, err := exec.CommandContext(ctx, "identify", "-format", "%T ", path).Output(); err == nil {
		delays = strings.Fields(string(out))
	}
	return readFrames(dir, func(i int) time.Duration {
		if i < len(delays) {
			if cs, err := strconv.Atoi(delays[i]); err == nil && cs > 0 {
				return time.Duration(cs) * 10 * time.Millisecond
			}
		}
		return 100 * time.Millisecond
	})
}

// gifFrames turns a WhatsApp GIF (a short mp4) into frames with ffmpeg.
func (c *Client) gifFrames(ctx context.Context, chatJID, msgID string) ([]media.Frame, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errors.New("install ffmpeg to play GIFs inline")
	}
	path, _, err := c.MediaFile(ctx, chatJID, msgID)
	if err != nil {
		return nil, err
	}
	return videoFrames(ctx, path)
}

func videoFrames(ctx context.Context, path string) ([]media.Frame, error) {
	dir, err := os.MkdirTemp("", "watui-gif-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if out, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-i", path,
		"-vf", "fps=10,scale='min(480,iw)':-2", "-frames:v", "60", filepath.Join(dir, "f_%03d.png")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v %s", err, strings.TrimSpace(string(out)))
	}
	return readFrames(dir, func(int) time.Duration { return 100 * time.Millisecond })
}

func readFrames(dir string, delay func(i int) time.Duration) ([]media.Frame, error) {
	names, err := filepath.Glob(filepath.Join(dir, "f_*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var frames []media.Frame
	for i, name := range names {
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		img, err := decode(raw)
		if err != nil {
			return nil, err
		}
		frames = append(frames, media.Frame{Img: img, Delay: delay(i)})
	}
	if len(frames) == 0 {
		return nil, errors.New("no frames decoded")
	}
	return frames, nil
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

// FileName is the document's original name, if the message has one.
func (c *Client) FileName(chatJID, msgID string) string {
	_, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return ""
	}
	if d, ok := msg.(*waE2E.DocumentMessage); ok {
		return filepath.Base(d.GetFileName())
	}
	return ""
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
	name := when + "-" + safeName(msgID) + filepath.Ext(src)
	if orig := c.FileName(chatJID, msgID); orig != "" && orig != "." && orig != "/" {
		name = orig
	}
	dst := uniquePath(filepath.Join(dir, name))
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return "", err
	}
	return dst, out.Close()
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		p := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
}
