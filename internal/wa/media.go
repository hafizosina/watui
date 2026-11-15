package wa

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"go.mau.fi/whatsmeow/proto/waE2E"
	_ "golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/media"
)

const (
	MediaImage   = "image"
	MediaSticker = "sticker"
	MediaVideo   = "video"
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
	default:
		return "", nil
	}
	data, err := proto.Marshal(part)
	if err != nil {
		return "", nil
	}
	return kind, data
}

func (c *Client) LoadImage(ctx context.Context, chatJID, msgID string, q media.Quality) (image.Image, error) {
	kind, data, err := c.Store.Media(chatJID, msgID)
	if err != nil {
		return nil, err
	}
	var raw []byte
	switch kind {
	case MediaImage:
		var msg waE2E.ImageMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		if q == media.Thumbnail {
			raw = msg.GetJPEGThumbnail()
		} else if raw, err = c.WA.Download(ctx, &msg); err != nil {
			return nil, err
		}
	case MediaSticker:
		var msg waE2E.StickerMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		if msg.GetIsAnimated() {
			return nil, errors.New("animated sticker")
		}
		if raw, err = c.WA.Download(ctx, &msg); err != nil {
			return nil, err
		}
	case MediaVideo:
		var msg waE2E.VideoMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		raw = msg.GetJPEGThumbnail()
	default:
		return nil, errors.New("no picture in this message")
	}
	if len(raw) == 0 {
		return nil, errors.New("no preview available")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}
