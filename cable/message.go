package cable

import (
	"strings"

	"github.com/sashabaranov/go-openai"
)

type Message struct {
	Role     string
	Models   []string
	Contents []Content
	Ops      []Operator

	content string
}

func (m *Message) String() string {
	var s strings.Builder
	for _, c := range m.Contents {
		if c.Text == "" {
			continue
		}
		s.WriteString(c.Text + "\n\n")
	}
	return s.String()
}

type Content struct {
	Text string

	Thinking bool
}

func (c *Cable) Messages() []openai.ChatCompletionMessage {
	t := []openai.ChatCompletionMessage{}
	for _, m := range c.Thread {
		mm := openai.ChatCompletionMessage{Role: m.Role}
		mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
			Type: "text",
			Text: strings.Trim(m.String(), "\n "),
		})

		for _, op := range m.Ops {
			switch op.Op() {
			case OperatorAttach:
				op, ok := op.(*Attachment)
				if !ok {
					continue
				}
				s := strings.TrimLeft(op.String(), string(OperatorAttach))
				mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
					Type:     openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{URL: s},
				})
			}
		}
		t = append(t, mm)
	}
	if len(t) == 0 {
		return nil
	}
	return t
}
