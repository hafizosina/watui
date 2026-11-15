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
		if m.GetVideoMessage().GetGifPlayback() {
			return withCaption("[gif]", m.GetVideoMessage().GetCaption())
		}
		return withCaption("[video]", m.GetVideoMessage().GetCaption())
	case m.GetPtvMessage() != nil:
		return "[video note]"
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return fmt.Sprintf("[voice note %ds]", m.GetAudioMessage().GetSeconds())
		}
		return "[audio]"
	case m.GetDocumentMessage() != nil:
		return withCaption("[document: "+m.GetDocumentMessage().GetFileName()+"]", m.GetDocumentMessage().GetCaption())
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
