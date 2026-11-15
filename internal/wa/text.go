package wa

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func messageText(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	switch {
	case m.GetConversation() != "":
		return m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return withCaption("[image]", m.GetImageMessage().GetCaption())
	case m.GetVideoMessage() != nil:
		v := m.GetVideoMessage()
		if v.GetGifPlayback() {
			return withCaption("[gif]", v.GetCaption())
		}
		return withCaption("[video "+Duration(v.GetSeconds())+" · "+Size(v.GetFileLength())+"]", v.GetCaption())
	case m.GetPtvMessage() != nil:
		return "[video note " + Duration(m.GetPtvMessage().GetSeconds()) + "]"
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return "[voice note " + Duration(m.GetAudioMessage().GetSeconds()) + "]"
		}
		return "[audio " + Duration(m.GetAudioMessage().GetSeconds()) + "]"
	case m.GetDocumentMessage() != nil:
		d := m.GetDocumentMessage()
		name := d.GetFileName()
		if name == "" {
			name = d.GetTitle()
		}
		return withCaption("[document: "+name+" · "+Size(d.GetFileLength())+"]", d.GetCaption())
	case m.GetStickerMessage() != nil:
		return "[sticker]"
	case m.GetLocationMessage() != nil:
		loc := m.GetLocationMessage()
		return fmt.Sprintf("[location %.5f,%.5f] %s", loc.GetDegreesLatitude(), loc.GetDegreesLongitude(), loc.GetName())
	case m.GetLiveLocationMessage() != nil:
		return "[live location]"
	case m.GetContactMessage() != nil:
		return "[contact: " + m.GetContactMessage().GetDisplayName() + "]"
	case m.GetContactsArrayMessage() != nil:
		return "[contacts]"
	case m.GetPollCreationMessage() != nil:
		return pollText(m.GetPollCreationMessage())
	case m.GetPollCreationMessageV2() != nil:
		return pollText(m.GetPollCreationMessageV2())
	case m.GetPollCreationMessageV3() != nil:
		return pollText(m.GetPollCreationMessageV3())
	case m.GetButtonsResponseMessage() != nil:
		return m.GetButtonsResponseMessage().GetSelectedDisplayText()
	case m.GetListResponseMessage() != nil:
		return m.GetListResponseMessage().GetTitle()
	case m.GetTemplateMessage() != nil:
		return "[template message]"
	case m.GetCall() != nil:
		return "[call]"
	}
	return ""
}

func pollText(p *waE2E.PollCreationMessage) string {
	var b strings.Builder
	b.WriteString("[poll] ")
	b.WriteString(p.GetName())
	for _, o := range p.GetOptions() {
		b.WriteString("\n  • ")
		b.WriteString(o.GetOptionName())
	}
	return b.String()
}

func withCaption(tag, caption string) string {
	if caption == "" {
		return tag
	}
	return tag + " " + caption
}

func Duration(seconds uint32) string {
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func Size(bytes uint64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%d KB", bytes>>10)
	}
	return fmt.Sprintf("%d B", bytes)
}
