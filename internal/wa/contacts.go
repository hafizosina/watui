package wa

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const MaxGroupName = 25

type Contact struct {
	JID   string
	Name  string
	Phone string
}

// Contacts returns everyone in the synced address book that has a phone number, sorted by name.
func (c *Client) Contacts(ctx context.Context) ([]Contact, error) {
	if c.WA.Store.Contacts == nil {
		return nil, nil
	}
	all, err := c.WA.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	own := c.ownJID()
	seen := map[string]bool{}
	var out []Contact
	for jid, info := range all {
		pn := c.normalize(ctx, jid)
		if pn.Server != types.DefaultUserServer || pn.User == "" || pn == own || seen[pn.String()] {
			continue
		}
		name := ""
		for _, n := range []string{info.FullName, info.FirstName, info.PushName, info.BusinessName} {
			if n != "" {
				name = n
				break
			}
		}
		seen[pn.String()] = true
		out = append(out, Contact{JID: pn.String(), Name: name, Phone: "+" + pn.User})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if (a == "") != (b == "") {
			return b == ""
		}
		if a != b {
			return a < b
		}
		return out[i].Phone < out[j].Phone
	})
	return out, nil
}

// PhoneDigits turns what the user typed into international digits, using the
// account's own country code for numbers that start with 0.
func (c *Client) PhoneDigits(input string) string {
	var b strings.Builder
	for _, r := range input {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if strings.HasPrefix(digits, "0") && !strings.HasPrefix(strings.TrimSpace(input), "+") {
		own := c.ownJID().User
		for _, cc := range []string{"62", "60", "65", "1", "44", "91", "61", "81", "86"} {
			if strings.HasPrefix(own, cc) {
				return cc + strings.TrimPrefix(digits, "0")
			}
		}
	}
	return digits
}

// CheckNumber returns the chat JID for a phone number, or an error if it has no WhatsApp.
func (c *Client) CheckNumber(ctx context.Context, digits string) (string, error) {
	if len(digits) < 8 {
		return "", errors.New("that number looks too short")
	}
	res, err := c.WA.IsOnWhatsApp(ctx, []string{"+" + digits})
	if err != nil {
		return "", err
	}
	for _, r := range res {
		if r.IsIn {
			return r.JID.ToNonAD().String(), nil
		}
	}
	return "", errors.New("+" + digits + " is not on WhatsApp")
}

// StartChat makes a chat show up in the list even before the first message.
func (c *Client) StartChat(jid, name string) error {
	if err := c.Store.EnsureChat(jid, strings.HasSuffix(jid, "@g.us")); err != nil {
		return err
	}
	if err := c.Store.SetChatNameIfEmpty(jid, name); err != nil {
		return err
	}
	if err := c.Store.TouchChat(jid, time.Now()); err != nil {
		return err
	}
	c.notify()
	return nil
}

func (c *Client) CreateGroup(ctx context.Context, name string, members []string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("the group needs a name")
	}
	if len([]rune(name)) > MaxGroupName {
		return "", errors.New("group names can be at most 25 characters")
	}
	var jids []types.JID
	for _, m := range members {
		jid, err := types.ParseJID(m)
		if err != nil {
			return "", err
		}
		jids = append(jids, jid)
	}
	if len(jids) == 0 {
		return "", errors.New("pick at least one member")
	}
	info, err := c.WA.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: jids})
	if err != nil {
		return "", err
	}
	jid := info.JID.String()
	return jid, c.StartChat(jid, name)
}

// InContacts reports whether jid is saved in your phone's address book.
func (c *Client) InContacts(jid string) bool {
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return false
	}
	info, ok := c.contact(context.Background(), parsed)
	return ok && (info.FullName != "" || info.FirstName != "")
}
