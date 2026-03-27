package protocol

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const DefaultUIDSelf = "0"

type Message interface {
	Type() ThreadType
	ThreadID() string
	MessageID() string
	SenderID() string
	Text() string
	IsSelf() bool
}

type TMessage struct {
	MsgID   string  `json:"msgId"`
	UIDFrom string  `json:"uidFrom"`
	IDTo    string  `json:"idTo"`
	Content Content `json:"content"`
}

type Content struct {
	String *string
	Raw    json.RawMessage
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.String = &s
		c.Raw = nil
		return nil
	}
	c.String = nil
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

func (c Content) Text() string {
	if c.String != nil {
		return *c.String
	}
	return ""
}

type Attachment struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

func (c Content) ParseAttachment() *Attachment {
	if len(c.Raw) == 0 {
		return nil
	}
	var att Attachment
	if err := json.Unmarshal(c.Raw, &att); err != nil {
		return &Attachment{}
	}
	return &att
}

func (a *Attachment) IsImage() bool {
	if a == nil || a.Href == "" {
		return false
	}
	path := strings.SplitN(a.Href, "?", 2)[0]
	ext := strings.ToLower(filepath.Ext(path))
	if imageExts[ext] {
		return true
	}
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/jpg/") || strings.Contains(lower, "/png/") ||
		strings.Contains(lower, "/gif/") || strings.Contains(lower, "/webp/")
}

func (c Content) AttachmentText() string {
	att := c.ParseAttachment()
	if att == nil {
		return ""
	}
	if att.IsImage() {
		if att.Title != "" {
			return fmt.Sprintf("[User sent an image: %s]", att.Title)
		}
		return "[User sent an image]"
	}
	if att.Href != "" {
		if att.Title != "" {
			return fmt.Sprintf("[User sent a file: %s]", att.Title)
		}
		return "[User sent a file]"
	}
	return "[User sent a non-text message]"
}

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

type UserMessage struct {
	Data     TMessage
	threadID string
	isSelf   bool
}

func NewUserMessage(selfUID string, data TMessage) UserMessage {
	msg := UserMessage{Data: data, threadID: data.UIDFrom}
	msg.isSelf = data.UIDFrom == DefaultUIDSelf
	if data.UIDFrom == DefaultUIDSelf {
		msg.threadID = data.IDTo
		msg.Data.UIDFrom = selfUID
	}
	if data.IDTo == DefaultUIDSelf {
		msg.Data.IDTo = selfUID
	}
	return msg
}

func (m UserMessage) Type() ThreadType { return ThreadTypeUser }

func (m UserMessage) ThreadID() string { return m.threadID }

func (m UserMessage) MessageID() string { return m.Data.MsgID }

func (m UserMessage) SenderID() string { return m.Data.UIDFrom }

func (m UserMessage) Text() string {
	text := strings.TrimSpace(m.Data.Content.Text())
	if text != "" {
		return text
	}
	return strings.TrimSpace(m.Data.Content.AttachmentText())
}

func (m UserMessage) IsSelf() bool { return m.isSelf }
