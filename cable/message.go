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

type Content struct {
	Text string

	Thinking bool
}

func (m *Message) Alt() int {
	for _, op := range m.Ops {
		if op.Op() != OperatorAlternate {
			continue
		}
		if op, ok := op.(*Alternate); ok {
			return op.Index
		}
	}
	return 0
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

type Prompt struct {
	Alt      int
	Model    string
	Messages []openai.ChatCompletionMessage
}

func (c *Cable) Polyprompt() []Prompt {
	n := len(c.Thread)
	if n == 0 || c.Thread[n-1].Role != "user" {
		return nil
	}
	if alt := c.Thread[n-1].Alt(); alt != 0 {
		return []Prompt{c.Prompt(alt)}
	}

	lastTurn := dominant(c.Thread[:n-1])

	return []Prompt{c.Prompt(lastTurn)}
}

func (c *Cable) Prompt(alternate int) (p Prompt) {
	seq, ok := c.alt[alternate]
	if !ok {
		return
	}
	p.Alt = alternate
	for _, i := range seq {
		m := c.Thread[i]
		mm := openai.ChatCompletionMessage{Role: m.Role}
		switch len(m.Contents) {
		case 0:
			// TODO: handle empty content
		case 1:
			mm.Content = trim(m.String())
		default:
			mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
				Type: "text",
				Text: trim(m.String()),
			})
			for _, op := range m.Ops {
				switch op.Op() {
				case OperatorAttach:
					op, ok := op.(*Attachment)
					if !ok {
						continue
					}
					mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
						Type:     openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{URL: op.String()},
					})
				}
			}
		}
		p.Messages = append(p.Messages, mm)
	}
	return
}

func (c *Cable) Messages() []openai.ChatCompletionMessage {
	t := []openai.ChatCompletionMessage{}
	for _, m := range c.Thread {
		mm := openai.ChatCompletionMessage{Role: m.Role}

		t = append(t, mm)
	}
	if len(t) == 0 {
		return nil
	}
	return t
}
