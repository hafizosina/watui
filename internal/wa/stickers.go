package wa

import (
	"context"
	"encoding/hex"
	"errors"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/hafizosina/watui/internal/store"
)

type StickerRef struct {
	ChatJID string
	ID      string
}

// RecentStickers lists distinct stickers seen recently, newest first.
func (c *Client) RecentStickers(limit int) ([]StickerRef, error) {
	msgs, err := c.Store.RecentMedia(MediaSticker, limit*4)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []StickerRef
	for _, m := range msgs {
		_, data, err := c.Store.Media(m.ChatJID, m.ID)
		if err != nil {
			continue
		}
		var st waE2E.StickerMessage
		if proto.Unmarshal(data, &st) != nil || st.GetDirectPath() == "" {
			continue
		}
		key := hex.EncodeToString(st.GetFileSHA256())
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, StickerRef{ChatJID: m.ChatJID, ID: m.ID})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// SendSticker re-sends a sticker we already have to another chat.
func (c *Client) SendSticker(ctx context.Context, toChat string, ref StickerRef) error {
	kind, data, err := c.Store.Media(ref.ChatJID, ref.ID)
	if err != nil {
		return err
	}
	if kind != MediaSticker {
		return errors.New("not a sticker")
	}
	var st waE2E.StickerMessage
	if err := proto.Unmarshal(data, &st); err != nil {
		return err
	}
	st.ContextInfo = nil
	clean, err := proto.Marshal(&st)
	if err != nil {
		return err
	}
	return c.sendAndStore(ctx, toChat, &waE2E.Message{StickerMessage: &st}, store.Message{
		Text: "[sticker]", MediaType: MediaSticker, Media: clean,
	})
}
