package wa

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// WhatsApp writes a mention as "@" + the user part of the mentioned JID, which is
// often a LID (a hidden id, not a phone number). The phone app shows "@Name" instead.
var mentionRe = regexp.MustCompile(`@(\d{5,20})\b`)

// negative lookups are retried after this, so names show up once contacts sync.
const mentionRetry = time.Minute

type mentionName struct {
	name string
	at   time.Time
}

// ResolveMentions replaces "@<id>" in text with "@<name>" where the id is known.
func (c *Client) ResolveMentions(text string) string {
	text, _ = c.ResolveMentionLabels(text)
	return text
}

// ResolveMentionLabels is ResolveMentions, also returning the "@<name>" labels it put in.
func (c *Client) ResolveMentionLabels(text string) (string, []string) {
	if !mentionRe.MatchString(text) {
		return text, nil
	}
	var labels []string
	text = mentionRe.ReplaceAllStringFunc(text, func(s string) string {
		if name := c.mentionName(s[1:]); name != "" {
			labels = append(labels, "@"+name)
			return "@" + name
		}
		return s
	})
	return text, labels
}

func (c *Client) mentionName(id string) string {
	c.mu.Lock()
	if c.mentions == nil {
		c.mentions = map[string]mentionName{}
	}
	cached, ok := c.mentions[id]
	c.mu.Unlock()
	if ok && (cached.name != "" || time.Since(cached.at) < mentionRetry) {
		return cached.name
	}
	name := c.lookupMention(context.Background(), id)
	c.mu.Lock()
	c.mentions[id] = mentionName{name: name, at: time.Now()}
	c.mu.Unlock()
	return name
}

func (c *Client) lookupMention(ctx context.Context, id string) string {
	lid := types.NewJID(id, types.HiddenUserServer)
	pn := types.NewJID(id, types.DefaultUserServer)
	if c.WA.Store.ID != nil {
		if own := c.ownJID(); own.User == id {
			return "You"
		}
		if own := c.WA.Store.GetLID(); !own.IsEmpty() && own.User == id {
			return "You"
		}
	}
	mapped := false
	if c.WA.Store.LIDs != nil {
		if p, err := c.WA.Store.LIDs.GetPNForLID(ctx, lid); err == nil && !p.IsEmpty() {
			pn, mapped = p.ToNonAD(), true
		}
	}
	for _, jid := range []types.JID{pn, lid} {
		if info, ok := c.contact(ctx, jid); ok {
			for _, n := range []string{info.FullName, info.FirstName, info.PushName, info.BusinessName} {
				if n != "" {
					return n
				}
			}
		}
	}
	if c.Store != nil {
		if n, err := c.Store.SenderName(pn.String(), lid.String()); err == nil && n != "" {
			return n
		}
	}
	if mapped {
		return "+" + pn.User
	}
	return ""
}

// backfillForMe flags unread mentions/replies stored before watui kept track of them,
// so the chat list highlights them too.
func (c *Client) backfillForMe() {
	own := c.ownJID()
	if own.IsEmpty() {
		return
	}
	at := []string{"@" + own.User}
	if lid := c.WA.Store.GetLID(); !lid.IsEmpty() {
		at = append(at, "@"+lid.User)
	}
	if err := c.Store.MarkForMe(at, own.String()); err != nil {
		c.log.Warnf("mark mentions: %v", err)
		return
	}
	c.notify()
}

// Mentionable is a group member you can @mention.
type Mentionable struct {
	JID   string // as the group addresses them (LID or phone JID); goes in MentionedJID
	User  string // JID user part; the message text carries "@" + User
	Name  string
	Phone string // "+62…" when known
}

// GroupMembers lists the people you can mention in a group (not you), sorted by name.
// It asks the server for the member list, and teaches ResolveMentions their names.
func (c *Client) GroupMembers(ctx context.Context, chatJID string) ([]Mentionable, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return nil, err
	}
	info, err := c.WA.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, err
	}
	own, ownLID := c.ownJID(), c.WA.Store.GetLID().ToNonAD()
	var out []Mentionable
	for _, p := range info.Participants {
		id := p.JID.ToNonAD()
		pn := p.PhoneNumber.ToNonAD()
		if pn.IsEmpty() {
			pn = c.normalize(ctx, id)
		}
		if id == own || id == ownLID || pn == own {
			continue
		}
		m := Mentionable{JID: id.String(), User: id.User}
		if pn.Server == types.DefaultUserServer {
			m.Phone = "+" + pn.User
		}
		m.Name = c.contactName(ctx, pn, p.DisplayName)
		if strings.HasPrefix(m.Name, "+") || m.Name == pn.User || m.Name == id.User {
			if n, _ := c.Store.SenderName(pn.String(), id.String()); n != "" {
				m.Name = n
			}
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	c.mu.Lock()
	if c.mentions == nil {
		c.mentions = map[string]mentionName{}
	}
	for _, m := range out {
		if !strings.HasPrefix(m.Name, "+") {
			c.mentions[m.User] = mentionName{name: m.Name, at: time.Now()}
		}
	}
	c.mu.Unlock()
	return out, nil
}
