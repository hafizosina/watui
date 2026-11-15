package wa

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/store"
)

const (
	EditWindow   = 15 * time.Minute
	RevokeWindow = 48 * time.Hour
)

func contextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetContextInfo()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetContextInfo()
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage().GetContextInfo()
	}
	return nil
}

func (c *Client) ownJID() types.JID {
	return c.WA.Store.GetJID().ToNonAD()
}

func (c *Client) MediaSeconds(chatJID, msgID string) uint32 {
	_, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return 0
	}
	switch m := msg.(type) {
	case *waE2E.AudioMessage:
		return m.GetSeconds()
	case *waE2E.VideoMessage:
		return m.GetSeconds()
	}
	return 0
}

func (c *Client) IsGIF(chatJID, msgID string) bool {
	_, msg, err := c.mediaMessage(chatJID, msgID)
	if err != nil {
		return false
	}
	v, ok := msg.(*waE2E.VideoMessage)
	return ok && v.GetGifPlayback()
}

func (c *Client) OwnJID() string {
	return c.ownJID().String()
}

func parse(jid string) (types.JID, error) {
	return types.ParseJID(jid)
}

func (c *Client) sendAndStore(ctx context.Context, chatJID string, content *waE2E.Message, local store.Message) error {
	jid, err := parse(chatJID)
	if err != nil {
		return err
	}
	local.ChatJID = chatJID
	local.ID = c.WA.GenerateMessageID()
	local.SenderJID = c.ownJID().String()
	local.FromMe = true
	local.TS = time.Now()
	local.Status = store.StatusPending
	local.Read = true
	c.check(c.Store.PutMessage(local))
	c.notify()
	_, err = c.WA.SendMessage(ctx, jid, content, whatsmeow.SendRequestExtra{ID: local.ID})
	status := store.StatusSent
	if err != nil {
		status = store.StatusFailed
	}
	c.check(c.Store.SetStatus(chatJID, local.ID, status))
	c.notify()
	return err
}

func (c *Client) Reply(ctx context.Context, chatJID, text string, to store.Message) error {
	content := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String(text),
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String(to.ID),
			Participant:   proto.String(to.SenderJID),
			QuotedMessage: &waE2E.Message{Conversation: proto.String(to.Text)},
		},
	}}
	return c.sendAndStore(ctx, chatJID, content, store.Message{
		Text:        text,
		QuoteID:     to.ID,
		QuoteSender: to.SenderJID,
		QuoteText:   to.Text,
	})
}

func CanEdit(m store.Message) bool {
	return m.FromMe && !m.Deleted && m.MediaType == "" && time.Since(m.TS) < EditWindow
}

func CanRevoke(m store.Message) bool {
	return m.FromMe && !m.Deleted && time.Since(m.TS) < RevokeWindow
}

func CanForward(m store.Message) bool {
	if m.Deleted {
		return false
	}
	return m.MediaType != "" || !strings.HasPrefix(m.Text, "[")
}

func (c *Client) Edit(ctx context.Context, m store.Message, text string) error {
	jid, err := parse(m.ChatJID)
	if err != nil {
		return err
	}
	edit := c.WA.BuildEdit(jid, m.ID, &waE2E.Message{Conversation: proto.String(text)})
	if _, err := c.WA.SendMessage(ctx, jid, edit); err != nil {
		return err
	}
	c.check(c.Store.EditMessage(m.ChatJID, m.ID, text))
	c.notify()
	return nil
}

func (c *Client) Revoke(ctx context.Context, m store.Message) error {
	jid, err := parse(m.ChatJID)
	if err != nil {
		return err
	}
	if _, err := c.WA.RevokeMessage(ctx, jid, m.ID); err != nil {
		return err
	}
	c.check(c.Store.DeleteMessage(m.ChatJID, m.ID))
	c.notify()
	return nil
}

func (c *Client) React(ctx context.Context, m store.Message, emoji string) error {
	jid, err := parse(m.ChatJID)
	if err != nil {
		return err
	}
	sender, err := parse(m.SenderJID)
	if err != nil {
		return err
	}
	if _, err := c.WA.SendMessage(ctx, jid, c.WA.BuildReaction(jid, sender, m.ID, emoji)); err != nil {
		return err
	}
	c.check(c.Store.SetReaction(m.ChatJID, m.ID, c.ownJID().String(), emoji))
	c.notify()
	return nil
}

func (c *Client) Star(ctx context.Context, m store.Message, starred bool) error {
	jid, err := parse(m.ChatJID)
	if err != nil {
		return err
	}
	sender, err := parse(m.SenderJID)
	if err != nil {
		return err
	}
	if err := c.WA.SendAppState(ctx, appstate.BuildStar(jid, sender, m.ID, m.FromMe, starred)); err != nil {
		return err
	}
	c.check(c.Store.SetStarred(m.ChatJID, m.ID, starred))
	c.notify()
	return nil
}

func (c *Client) Pin(ctx context.Context, m store.Message, pin bool, duration time.Duration) error {
	jid, err := parse(m.ChatJID)
	if err != nil {
		return err
	}
	sender, err := parse(m.SenderJID)
	if err != nil {
		return err
	}
	kind := waE2E.PinInChatMessage_PIN_FOR_ALL
	if !pin {
		kind = waE2E.PinInChatMessage_UNPIN_FOR_ALL
	}
	content := &waE2E.Message{
		PinInChatMessage: &waE2E.PinInChatMessage{
			Key:               c.WA.BuildMessageKey(jid, sender, m.ID),
			Type:              kind.Enum(),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
	if pin {
		content.MessageContextInfo = &waE2E.MessageContextInfo{
			MessageAddOnDurationInSecs: proto.Uint32(uint32(duration.Seconds())),
		}
	}
	_, err = c.WA.SendMessage(ctx, jid, content)
	return err
}

func (c *Client) Forward(ctx context.Context, m store.Message, toChatJID string) error {
	if !CanForward(m) {
		return errors.New("this kind of message can't be forwarded")
	}
	fwd := &waE2E.ContextInfo{IsForwarded: proto.Bool(true), ForwardingScore: proto.Uint32(1)}
	local := store.Message{Text: m.Text, Forwarded: true}
	content := &waE2E.Message{}
	kind, data, err := c.Store.Media(m.ChatJID, m.ID)
	if err != nil {
		return err
	}
	switch kind {
	case MediaImage:
		var img waE2E.ImageMessage
		if err := proto.Unmarshal(data, &img); err != nil {
			return err
		}
		img.ContextInfo = fwd
		content.ImageMessage = &img
	case MediaVideo:
		var vid waE2E.VideoMessage
		if err := proto.Unmarshal(data, &vid); err != nil {
			return err
		}
		vid.ContextInfo = fwd
		content.VideoMessage = &vid
	case MediaAudio:
		var au waE2E.AudioMessage
		if err := proto.Unmarshal(data, &au); err != nil {
			return err
		}
		au.ContextInfo = fwd
		content.AudioMessage = &au
	case MediaSticker:
		var st waE2E.StickerMessage
		if err := proto.Unmarshal(data, &st); err != nil {
			return err
		}
		st.ContextInfo = fwd
		content.StickerMessage = &st
	default:
		content.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: proto.String(m.Text), ContextInfo: fwd}
	}
	if kind != "" {
		local.MediaType, local.Media = kind, data
	}
	return c.sendAndStore(ctx, toChatJID, content, local)
}
