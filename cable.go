package simp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
)

var (
	MarkUser = ">>>"
	MarkAsst = "<<<"

	// captures: whitespace, mark, annotation
	marx = regexp.MustCompile(`(?m)^([\t ]*)(>{3,}|<{3,})(?: +(\S+))?$`)
	// captures: png, jpg, jpeg, gif, webp urls
	imgrx = regexp.MustCompile(`https?://\S+\.(?:png|jpg|jpeg|gif|webp)`)
)

type Message struct {
	Role       string
	Content    string
	Annotation string
}

type Cable struct {
	Thread     []Message
	Whitespace string
}

func (c *Cable) AppendUser(s string) {
	c.Thread = append(c.Thread, Message{Role: "user", Content: s})
}

func (c Cable) Messages() []openai.ChatCompletionMessage {
	t := []openai.ChatCompletionMessage{}
	for _, m := range c.Thread {
		mm := openai.ChatCompletionMessage{Role: m.Role}
		imgs := imgrx.FindAllString(m.Content, -1)
		if len(imgs) == 0 {
			mm.Content = m.Content
			t = append(t, mm)
			continue
		}
		mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
			Type: "text",
			Text: m.Content,
		})
		for _, img := range imgs {
			mm.MultiContent = append(mm.MultiContent, openai.ChatMessagePart{
				Type:     "image_url",
				ImageURL: &openai.ChatMessageImageURL{URL: img},
			})
		}
		t = append(t, mm)
	}
	return t
}

// ParseCable attempts to parse a string into a Cable structure.
//
// It returns an error if the input is not a cable, or appears
// to be malformed.
func ParseCable(s string) (c Cable, err error) {
	c.Whitespace = "\t"
	trim := strings.TrimSpace
	s = trim(s)
	marx := marx.FindAllStringSubmatchIndex(s, -1)
	if len(marx) == 0 {
		// If no marks found, treat entire input as user message
		c.Thread = []Message{{Role: "user", Content: s}}
		return
	}
	// Handle system message if present
	if marx[0][0] > 0 {
		if s := trim(s[:marx[0][0]]); s != "" {
			c.Thread = append(c.Thread, Message{Role: "system", Content: s})
		}
	}
	// Record the whitespace pattern.
	if len(marx) > 0 {
		c.Whitespace = s[marx[0][2]:marx[0][3]]
	}
	// Process each mark and its following content
	for i, pos := range marx {
		var m Message

		// Determine role based on direction
		mark := s[pos[4]:pos[5]]
		m.Role = "user"
		if mark[0] == '<' {
			m.Role = "assistant"
		}
		// Extract annotation if present
		if pos[7] > pos[6] {
			m.Annotation = s[pos[6]:pos[7]]
		}
		// Extract content
		head, tail := pos[1], len(s)
		if i < len(marx)-1 {
			tail = marx[i+1][0]
		}
		m.Content = trim(s[head:tail])
		if m.Content == "" {
			return c, fmt.Errorf("empty message content at mark %d", i)
		}
		c.Thread = append(c.Thread, m)
	}
	return
}
