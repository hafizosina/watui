package wa

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/store"
)

type State int

const (
	StateConnecting State = iota
	StateConnected
	StateDisconnected
	StateLoggedOut
)

type Client struct {
	WA    *whatsmeow.Client
	Store *store.Store
	log   waLog.Logger

	changed chan struct{}

	mu       sync.Mutex
	state    State
	syncing  bool
	lastSync time.Time
	groups   map[types.JID]bool
}

func New(wa *whatsmeow.Client, st *store.Store, log waLog.Logger) *Client {
	c := &Client{
		WA:      wa,
		Store:   st,
		log:     log,
		changed: make(chan struct{}, 1),
		groups:  map[types.JID]bool{},
	}
	wa.AddEventHandler(c.handle)
	return c
}

func (c *Client) Changed() <-chan struct{} {
	return c.changed
}

func (c *Client) Notify() {
	c.notify()
}

func (c *Client) notify() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

func (c *Client) State() (State, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	syncing := c.syncing && time.Since(c.lastSync) < 5*time.Second
	return c.state, syncing
}

func (c *Client) setState(s State) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	c.notify()
}

func (c *Client) handle(evt any) {
	ctx := context.Background()
	switch e := evt.(type) {
	case *events.Connected:
		c.setState(StateConnected)
		go c.loadGroupNames(ctx)
	case *events.Disconnected:
		c.setState(StateDisconnected)
	case *events.StreamReplaced:
		c.setState(StateDisconnected)
	case *events.LoggedOut:
		c.setState(StateLoggedOut)
	case *events.Message:
		c.onMessage(ctx, e, false)
		c.notify()
	case *events.HistorySync:
		c.mu.Lock()
		c.syncing = true
		c.lastSync = time.Now()
		c.mu.Unlock()
		c.onHistorySync(ctx, e.Data)
		c.notify()
	case *events.Receipt:
		c.onReceipt(ctx, e)
		c.notify()
	case *events.Pin:
		jid := c.normalize(ctx, e.JID)
		c.ensureChat(ctx, jid)
		c.check(c.Store.SetPinned(jid.String(), e.Action.GetPinned()))
		c.notify()
	case *events.Archive:
		jid := c.normalize(ctx, e.JID)
		c.ensureChat(ctx, jid)
		c.check(c.Store.SetArchived(jid.String(), e.Action.GetArchived()))
		c.notify()
	case *events.Mute:
		jid := c.normalize(ctx, e.JID)
		c.ensureChat(ctx, jid)
		until := time.Time{}
		if e.Action.GetMuted() {
			until = time.Unix(e.Action.GetMuteEndTimestamp(), 0)
			if e.Action.GetMuteEndTimestamp() <= 0 {
				until = store.MutedForever
			}
		}
		c.check(c.Store.SetMutedUntil(jid.String(), until))
		c.notify()
	case *events.MarkChatAsRead:
		if e.Action.GetRead() {
			jid := c.normalize(ctx, e.JID)
			c.check(c.Store.MarkChatRead(jid.String()))
			c.notify()
		}
	case *events.Star:
		chat := c.normalize(ctx, e.ChatJID)
		c.check(c.Store.SetStarred(chat.String(), e.MessageID, e.Action.GetStarred()))
		c.notify()
	case *events.Contact, *events.PushName:
		c.notify()
	case *events.GroupInfo:
		if e.Name != nil {
			c.check(c.Store.SetChatName(e.JID.String(), e.Name.Name))
			c.notify()
		}
	case *events.JoinedGroup:
		c.ensureChat(ctx, e.JID)
		c.check(c.Store.SetChatName(e.JID.String(), e.GroupName.Name))
		c.notify()
	}
}

func (c *Client) check(err error) {
	if err != nil {
		c.log.Errorf("store: %v", err)
	}
}

func (c *Client) normalize(ctx context.Context, jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server == types.HiddenUserServer && c.WA.Store.LIDs != nil {
		pn, err := c.WA.Store.LIDs.GetPNForLID(ctx, jid)
		if err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return jid
}

func (c *Client) ensureChat(ctx context.Context, jid types.JID) {
	c.check(c.Store.EnsureChat(jid.String(), jid.Server == types.GroupServer))
}

func skipChat(jid types.JID) bool {
	return jid == types.StatusBroadcastJID || jid.Server == types.BroadcastServer || jid.Server == types.NewsletterServer
}

func (c *Client) onMessage(ctx context.Context, e *events.Message, history bool) {
	chat := c.normalize(ctx, e.Info.Chat)
	if skipChat(chat) {
		return
	}
	if pm := e.Message.GetProtocolMessage(); pm != nil {
		c.onProtocol(ctx, chat, pm)
		return
	}
	if r := e.Message.GetReactionMessage(); r != nil {
		sender := c.normalize(ctx, e.Info.Sender)
		if e.Info.IsFromMe {
			sender = c.ownJID()
		}
		c.check(c.Store.SetReaction(chat.String(), r.GetKey().GetID(), sender.String(), r.GetText()))
		return
	}
	if e.Message.GetEncReactionMessage() != nil {
		return
	}
	text := messageText(e.Message)
	if text == "" {
		return
	}
	mediaType, mediaData := mediaOf(e.Message)
	var quoteID, quoteSender, quoteText string
	ci := contextInfo(e.Message)
	if ci.GetStanzaID() != "" {
		quoteID = ci.GetStanzaID()
		quoteText = messageText(ci.GetQuotedMessage())
		if p, err := types.ParseJID(ci.GetParticipant()); err == nil {
			quoteSender = c.normalize(ctx, p).String()
		}
	}
	sender := c.normalize(ctx, e.Info.Sender)
	status := store.StatusSent
	if e.Info.IsFromMe {
		status = store.StatusDelivered
	}
	if history && e.SourceWebMsg != nil {
		status = webStatus(e.SourceWebMsg.GetStatus())
	}
	c.ensureChat(ctx, chat)
	if !chat.IsEmpty() && chat.Server == types.DefaultUserServer && !e.Info.IsFromMe {
		c.check(c.Store.SetChatNameIfEmpty(chat.String(), c.contactName(ctx, sender, e.Info.PushName)))
	}
	c.check(c.Store.PutMessage(store.Message{
		ChatJID:     chat.String(),
		ID:          e.Info.ID,
		SenderJID:   sender.String(),
		SenderName:  e.Info.PushName,
		FromMe:      e.Info.IsFromMe,
		TS:          e.Info.Timestamp,
		Text:        text,
		Status:      status,
		Read:        e.Info.IsFromMe || history,
		MediaType:   mediaType,
		Media:       mediaData,
		QuoteID:     quoteID,
		QuoteSender: quoteSender,
		QuoteText:   quoteText,
		Forwarded:   ci.GetIsForwarded(),
	}))
}

func webStatus(s waWeb.WebMessageInfo_Status) store.Status {
	switch s {
	case 0:
		return store.StatusFailed
	case 1:
		return store.StatusPending
	case 2:
		return store.StatusSent
	case 3:
		return store.StatusDelivered
	default:
		return store.StatusRead
	}
}

func (c *Client) onProtocol(ctx context.Context, chat types.JID, p *waE2E.ProtocolMessage) {
	id := p.GetKey().GetID()
	switch p.GetType() {
	case waE2E.ProtocolMessage_REVOKE:
		c.check(c.Store.DeleteMessage(chat.String(), id))
	case waE2E.ProtocolMessage_MESSAGE_EDIT:
		if text := messageText(p.GetEditedMessage()); text != "" {
			c.check(c.Store.EditMessage(chat.String(), id, text))
		}
	}
}

func (c *Client) onHistorySync(ctx context.Context, data *waHistorySync.HistorySync) {
	for _, pn := range data.GetPushnames() {
		if c.WA.Store.Contacts == nil {
			break
		}
		jid, err := types.ParseJID(pn.GetID())
		if err == nil && pn.GetPushname() != "" {
			_, _, _ = c.WA.Store.Contacts.PutPushName(ctx, jid, pn.GetPushname())
		}
	}
	for _, conv := range data.GetConversations() {
		raw, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		chat := c.normalize(ctx, raw)
		if skipChat(chat) {
			continue
		}
		c.ensureChat(ctx, chat)
		name := conv.GetName()
		if name == "" {
			name = conv.GetDisplayName()
		}
		c.check(c.Store.SetChatNameIfEmpty(chat.String(), name))
		if conv.GetPinned() > 0 {
			c.check(c.Store.SetPinned(chat.String(), true))
		}
		if conv.GetArchived() {
			c.check(c.Store.SetArchived(chat.String(), true))
		}
		if conv.GetMuteEndTime() > 0 {
			c.check(c.Store.SetMutedUntil(chat.String(), time.Unix(int64(conv.GetMuteEndTime()), 0)))
		}
		if ts := conv.GetConversationTimestamp(); ts > 0 {
			c.check(c.Store.TouchChat(chat.String(), time.Unix(int64(ts), 0)))
		}

		var parsed []*events.Message
		for _, hm := range conv.GetMessages() {
			evt, err := c.WA.ParseWebMessage(raw, hm.GetMessage())
			if err != nil {
				continue
			}
			parsed = append(parsed, evt)
		}
		sort.Slice(parsed, func(i, j int) bool { return parsed[i].Info.Timestamp.After(parsed[j].Info.Timestamp) })
		unread := int(conv.GetUnreadCount())
		var unreadIDs []string
		for _, evt := range parsed {
			c.onMessage(ctx, evt, true)
			if !evt.Info.IsFromMe && len(unreadIDs) < unread {
				unreadIDs = append(unreadIDs, evt.Info.ID)
			}
		}
		if unread > 0 {
			c.check(c.Store.SetSyncUnread(chat.String(), unread))
			c.check(c.Store.MarkUnreadByIDs(chat.String(), unreadIDs))
		}
	}
}

func (c *Client) onReceipt(ctx context.Context, e *events.Receipt) {
	switch e.Type {
	case types.ReceiptTypeDelivered:
		c.check(c.Store.SetStatusByIDs(e.MessageIDs, store.StatusDelivered))
	case types.ReceiptTypeRead, types.ReceiptTypePlayed:
		if e.IsFromMe {
			chat := c.normalize(ctx, e.Chat)
			c.check(c.Store.MarkReadByIDs(chat.String(), e.MessageIDs))
			return
		}
		c.check(c.Store.SetStatusByIDs(e.MessageIDs, store.StatusRead))
	case types.ReceiptTypeReadSelf:
		chat := c.normalize(ctx, e.Chat)
		c.check(c.Store.MarkReadByIDs(chat.String(), e.MessageIDs))
	}
}

func (c *Client) loadGroupNames(ctx context.Context) {
	groups, err := c.WA.GetJoinedGroups(ctx)
	if err != nil {
		c.log.Warnf("joined groups: %v", err)
		return
	}
	for _, g := range groups {
		c.ensureChat(ctx, g.JID)
		c.check(c.Store.SetChatName(g.JID.String(), g.Name))
	}
	c.notify()
}

func (c *Client) contact(ctx context.Context, jid types.JID) (types.ContactInfo, bool) {
	if c.WA.Store.Contacts == nil {
		return types.ContactInfo{}, false
	}
	info, err := c.WA.Store.Contacts.GetContact(ctx, jid)
	return info, err == nil && info.Found
}

func (c *Client) contactName(ctx context.Context, jid types.JID, fallback string) string {
	if info, ok := c.contact(ctx, jid); ok {
		for _, n := range []string{info.FullName, info.FirstName, info.PushName, info.BusinessName} {
			if n != "" {
				return n
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	if jid.Server == types.DefaultUserServer {
		return "+" + jid.User
	}
	return jid.User
}

func (c *Client) Me() string {
	if c.WA.Store.PushName != "" {
		return c.WA.Store.PushName
	}
	if c.WA.Store.ID != nil {
		return "+" + c.WA.Store.ID.User
	}
	return ""
}

func (c *Client) ContactName(jid string, fallback string) string {
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return fallback
	}
	return c.contactName(context.Background(), parsed, fallback)
}

func (c *Client) ChatName(chat store.Chat) string {
	jid, err := types.ParseJID(chat.JID)
	if err != nil {
		return chat.JID
	}
	if jid.Server == types.DefaultUserServer {
		if jid == c.WA.Store.GetJID().ToNonAD() {
			return "You"
		}
		info, ok := c.contact(context.Background(), jid)
		if ok && (info.FullName != "" || info.FirstName != "") {
			if info.FullName != "" {
				return info.FullName
			}
			return info.FirstName
		}
	}
	if chat.Name != "" {
		return chat.Name
	}
	return c.contactName(context.Background(), jid, "")
}

func (c *Client) Send(ctx context.Context, chatJID, text string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	id := c.WA.GenerateMessageID()
	own := c.WA.Store.GetJID().ToNonAD()
	msg := store.Message{
		ChatJID:   chatJID,
		ID:        id,
		SenderJID: own.String(),
		FromMe:    true,
		TS:        time.Now(),
		Text:      text,
		Status:    store.StatusPending,
		Read:      true,
	}
	c.check(c.Store.PutMessage(msg))
	c.notify()
	_, err = c.WA.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)}, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		c.check(c.Store.SetStatus(chatJID, id, store.StatusFailed))
		c.notify()
		return err
	}
	c.check(c.Store.SetStatus(chatJID, id, store.StatusSent))
	c.notify()
	return nil
}

func (c *Client) MarkRead(ctx context.Context, chatJID string) error {
	unread, err := c.Store.UnreadIncoming(chatJID)
	if err != nil {
		return err
	}
	if err := c.Store.MarkChatRead(chatJID); err != nil {
		return err
	}
	c.notify()
	if len(unread) == 0 {
		return nil
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	bySender := map[string][]types.MessageID{}
	for _, m := range unread {
		bySender[m.SenderJID] = append(bySender[m.SenderJID], m.ID)
	}
	for senderJID, ids := range bySender {
		sender := types.EmptyJID
		if chat.Server == types.GroupServer {
			sender, _ = types.ParseJID(senderJID)
		}
		if err := c.WA.MarkRead(ctx, ids, time.Now(), chat, sender); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) SetPinned(ctx context.Context, chatJID string, pinned bool) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	if err := c.WA.SendAppState(ctx, appstate.BuildPin(jid, pinned)); err != nil {
		return err
	}
	c.check(c.Store.SetPinned(chatJID, pinned))
	c.notify()
	return nil
}

func (c *Client) SetArchived(ctx context.Context, chatJID string, archived bool) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	last, ok, err := c.Store.LastMessage(chatJID)
	if err != nil {
		return err
	}
	var key *waCommon.MessageKey
	ts := time.Now()
	if ok {
		ts = last.TS
		key = &waCommon.MessageKey{
			RemoteJID: proto.String(chatJID),
			FromMe:    proto.Bool(last.FromMe),
			ID:        proto.String(last.ID),
		}
	}
	if err := c.WA.SendAppState(ctx, appstate.BuildArchive(jid, archived, ts, key)); err != nil {
		return err
	}
	c.check(c.Store.SetArchived(chatJID, archived))
	c.notify()
	return nil
}

func (c *Client) SetMuted(ctx context.Context, chatJID string, muted bool) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	if err := c.WA.SendAppState(ctx, appstate.BuildMute(jid, muted, 0)); err != nil {
		return err
	}
	until := time.Time{}
	if muted {
		until = store.MutedForever
	}
	c.check(c.Store.SetMutedUntil(chatJID, until))
	c.notify()
	return nil
}
