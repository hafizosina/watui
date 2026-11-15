package wa

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const PicturePrefix = "pp:"

type Person struct {
	JID       string
	Phone     string
	Name      string
	PushName  string
	Business  string
	About     string
	PictureID string
	IsMe      bool
	Groups    []string
}

type Member struct {
	JID   string
	Phone string
	Name  string
	Admin bool
	Owner bool
	IsMe  bool
}

type Group struct {
	JID       string
	Name      string
	Topic     string
	Created   time.Time
	CreatedBy string
	Members   []Member
	PictureID string
	Announce  bool
	Locked    bool
}

// PictureKey is the image key for a profile picture (changes when the picture changes).
func PictureKey(jid, pictureID string) string {
	return PicturePrefix + jid + "|" + pictureID
}

func (c *Client) pictureID(ctx context.Context, jid types.JID) string {
	info, err := c.WA.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: false})
	if err != nil || info == nil {
		return ""
	}
	c.mu.Lock()
	c.picURLs[PictureKey(jid.String(), info.ID)] = info.URL
	c.mu.Unlock()
	return info.ID
}

func (c *Client) PersonInfo(ctx context.Context, jidStr string) (Person, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return Person{}, err
	}
	jid = c.normalize(ctx, jid)
	p := Person{JID: jid.String(), IsMe: jid == c.ownJID()}
	if jid.Server == types.DefaultUserServer {
		p.Phone = "+" + jid.User
	}
	if info, ok := c.contact(ctx, jid); ok {
		p.Name = info.FullName
		if p.Name == "" {
			p.Name = info.FirstName
		}
		p.PushName = info.PushName
		p.Business = info.BusinessName
	}
	if p.IsMe {
		p.PushName = c.WA.Store.PushName
	}
	if users, err := c.WA.GetUserInfo(ctx, []types.JID{jid}); err == nil {
		if u, ok := users[jid]; ok {
			p.About = u.Status
			if u.VerifiedName != nil && u.VerifiedName.Details != nil && p.Business == "" {
				p.Business = u.VerifiedName.Details.GetVerifiedName()
			}
		}
	}
	p.PictureID = c.pictureID(ctx, jid)
	if !p.IsMe {
		p.Groups = c.commonGroups(ctx, jid)
	}
	return p, nil
}

func (c *Client) commonGroups(ctx context.Context, jid types.JID) []string {
	groups, err := c.WA.GetJoinedGroups(ctx)
	if err != nil {
		return nil
	}
	var names []string
	for _, g := range groups {
		for _, p := range g.Participants {
			pn := p.PhoneNumber
			if pn.IsEmpty() {
				pn = c.normalize(ctx, p.JID)
			}
			if pn.ToNonAD() == jid {
				names = append(names, g.Name)
				break
			}
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}

func (c *Client) MyInfo(ctx context.Context) (Person, error) {
	own := c.ownJID()
	if own.IsEmpty() {
		return Person{}, errors.New("this profile is not linked")
	}
	return c.PersonInfo(ctx, own.String())
}

func (c *Client) GroupInfo(ctx context.Context, jidStr string) (Group, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return Group{}, err
	}
	info, err := c.WA.GetGroupInfo(ctx, jid)
	if err != nil {
		return Group{}, err
	}
	g := Group{
		JID:      jid.String(),
		Name:     info.Name,
		Topic:    info.Topic,
		Created:  info.GroupCreated,
		Announce: info.IsAnnounce,
		Locked:   info.IsLocked,
	}
	owner := info.OwnerPN
	if owner.IsEmpty() {
		owner = c.normalize(ctx, info.OwnerJID)
	}
	if !owner.IsEmpty() {
		g.CreatedBy = c.contactName(ctx, owner, "")
	}
	own := c.ownJID()
	for _, p := range info.Participants {
		pn := p.PhoneNumber
		if pn.IsEmpty() {
			pn = c.normalize(ctx, p.JID)
		}
		pn = pn.ToNonAD()
		m := Member{JID: pn.String(), Admin: p.IsAdmin || p.IsSuperAdmin, Owner: p.IsSuperAdmin, IsMe: pn == own}
		if pn.Server == types.DefaultUserServer {
			m.Phone = "+" + pn.User
		}
		m.Name = c.contactName(ctx, pn, p.DisplayName)
		if m.IsMe {
			m.Name = "You"
		}
		g.Members = append(g.Members, m)
	}
	sort.SliceStable(g.Members, func(i, j int) bool {
		a, b := g.Members[i], g.Members[j]
		if a.IsMe != b.IsMe {
			return a.IsMe
		}
		if a.Admin != b.Admin {
			return a.Admin
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	g.PictureID = c.pictureID(ctx, jid)
	return g, nil
}

func (c *Client) loadPicture(ctx context.Context, key string) (image.Image, error) {
	c.mu.Lock()
	url := c.picURLs[key]
	c.mu.Unlock()
	if url == "" {
		return nil, errors.New("no profile picture")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile picture: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return decode(data)
}
