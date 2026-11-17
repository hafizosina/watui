package wa

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"golang.org/x/image/draw"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/store"
)

// MaxUpload is WhatsApp's limit for a document; media kinds are smaller.
const MaxUpload = 2 << 30

const maxInlineMedia = 16 << 20 // bigger pictures/videos/audio go as documents

// SendKind says how a file will be sent: image, video, audio or document.
func SendKind(path string, asDocument bool) (kind, mimetype string, err error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	if st.IsDir() {
		return "", "", fmt.Errorf("%s is a folder", filepath.Base(path))
	}
	mimetype = detectMime(path)
	if asDocument || st.Size() > maxInlineMedia {
		return MediaDoc, mimetype, nil
	}
	switch mimetype {
	case "image/jpeg", "image/png":
		return MediaImage, mimetype, nil
	case "video/mp4", "video/3gpp":
		return MediaVideo, mimetype, nil
	case "audio/ogg", "audio/mpeg", "audio/mp4", "audio/aac", "audio/x-m4a":
		return MediaAudio, mimetype, nil
	}
	return MediaDoc, mimetype, nil
}

func detectMime(path string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); t != "" {
		t, _, _ = strings.Cut(t, ";")
		return strings.TrimSpace(t)
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	t, _, _ := strings.Cut(http.DetectContentType(head[:n]), ";")
	return strings.TrimSpace(t)
}

// SendFile uploads the file at path and sends it to the chat, as a picture,
// video or voice/audio message when WhatsApp shows those inline, otherwise
// (or when asDocument is set) as a document. caption may be empty.
func (c *Client) SendFile(ctx context.Context, chatJID, path, caption string, asDocument bool) error {
	kind, mimetype, err := SendKind(path, asDocument)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(path))
	}
	if len(data) > MaxUpload {
		return fmt.Errorf("%s is bigger than WhatsApp's 2 GB limit", filepath.Base(path))
	}
	appInfo := map[string]whatsmeow.MediaType{
		MediaImage: whatsmeow.MediaImage, MediaVideo: whatsmeow.MediaVideo,
		MediaAudio: whatsmeow.MediaAudio, MediaDoc: whatsmeow.MediaDocument,
	}[kind]
	up, err := c.WA.Upload(ctx, data, appInfo)
	if err != nil {
		return fmt.Errorf("upload %s: %w", filepath.Base(path), err)
	}
	var capt *string
	if caption != "" {
		capt = proto.String(caption)
	}
	content := &waE2E.Message{}
	switch kind {
	case MediaImage:
		im := &waE2E.ImageMessage{
			Caption: capt, Mimetype: proto.String(mimetype),
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
		}
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			b := img.Bounds()
			im.Width, im.Height = proto.Uint32(uint32(b.Dx())), proto.Uint32(uint32(b.Dy()))
			im.JPEGThumbnail = thumbnail(img)
		}
		content.ImageMessage = im
	case MediaVideo:
		vm := &waE2E.VideoMessage{
			Caption: capt, Mimetype: proto.String(mimetype),
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
			Seconds: proto.Uint32(probeSeconds(ctx, path)),
		}
		if img := firstFrame(ctx, path); img != nil {
			b := img.Bounds()
			vm.Width, vm.Height = proto.Uint32(uint32(b.Dx())), proto.Uint32(uint32(b.Dy()))
			vm.JPEGThumbnail = thumbnail(img)
		}
		content.VideoMessage = vm
	case MediaAudio:
		content.AudioMessage = &waE2E.AudioMessage{
			Mimetype: proto.String(mimetype),
			URL:      &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
			Seconds: proto.Uint32(probeSeconds(ctx, path)),
		}
	default:
		name := filepath.Base(path)
		dm := &waE2E.DocumentMessage{
			Caption: capt, Mimetype: proto.String(mimetype),
			Title: proto.String(name), FileName: proto.String(name),
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
		}
		if strings.HasPrefix(mimetype, "image/") {
			if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
				dm.JPEGThumbnail = thumbnail(img)
			}
		}
		content.DocumentMessage = dm
	}

	mediaType, raw := mediaOf(content)
	id := c.WA.GenerateMessageID()
	c.cacheSent(id, kind, mimetype, path, data)
	return c.sendAndStore(ctx, chatJID, content, store.Message{
		ID: id, Text: messageText(content), MediaType: mediaType, Media: raw,
	})
}

// cacheSent puts our own copy where MediaFile looks, so the sent file opens
// and previews without downloading it back.
func (c *Client) cacheSent(id, kind, mimetype, path string, data []byte) {
	dir, err := cacheDir()
	if err != nil {
		return
	}
	ext := extFor(kind, mimetype)
	if kind == MediaDoc {
		if e := filepath.Ext(path); e != "" && len(e) <= 10 {
			ext = strings.ToLower(e)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, safeName(id)+ext), data, 0o600)
}

func thumbnail(img image.Image) []byte {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	const side = 100
	w, h := side, b.Dy()*side/b.Dx()
	if b.Dy() > b.Dx() {
		w, h = b.Dx()*side/b.Dy(), side
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(1, w), max(1, h)))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 60}); err != nil {
		return nil
	}
	return buf.Bytes()
}

func probeSeconds(ctx context.Context, path string) uint32 {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f < 0 {
		return 0
	}
	return uint32(f + 0.5)
}

func firstFrame(ctx context.Context, path string) image.Image {
	out, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-i", path,
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-").Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		return nil
	}
	return img
}
