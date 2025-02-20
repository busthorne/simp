package cable

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/busthorne/simp"
)

type Cable struct {
	Thread []Message

	ws  string
	alt map[int][]int
}

// Captures: whitespace, mark, annotation
var (
	ErrNotCable = errors.New("cable: not a cable")

	marx = regexp.MustCompile(`(?m)^([\t ]*)(>{3,}|<{3,})(?: +(\S+))?$`)
)

// ParseCable attempts to parse a cable from a string.
//
// It returns an error if the input is not a cable, or appears
// to be malformed.
func ParseCable(s string) (*Cable, error) {
	c := &Cable{alt: make(map[int][]int)}
	c.ws = "\t"
	trim := strings.TrimSpace
	marx := marx.FindAllStringSubmatchIndex(s, -1)
	// If no marks found, treat entire input as user message
	if len(marx) == 0 {
		m := Message{Role: "user", content: trim(s)}
		if err := c.parseTail(m); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotCable, err)
		}
		return c, ErrNotCable
	}
	// Handle system message if present
	if marx[0][0] > 0 {
		if s := s[:marx[0][0]]; trim(s) != "" {
			m := Message{Role: "system", content: s}
			if err := c.parseTail(m); err != nil {
				return nil, err
			}
		}
	}
	// Record the whitespace pattern.
	if len(marx) > 0 {
		c.ws = s[marx[0][2]:marx[0][3]]
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
			m.Models = strings.Split(s[pos[6]:pos[7]], ",")
		}
		// Extract content
		head, tail := pos[1], len(s)
		if i < len(marx)-1 {
			tail = marx[i+1][0]
		}
		m.content = s[head:tail]
		if err := c.parseTail(m); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Cable) parseTail(m Message) error {
	s := strings.Trim(m.content, "\n")
	m.content = s
	alt := 0
	for strings.HasPrefix(s, c.ws) {
		s = s[len(c.ws):]
		i := strings.Index(s, "\n")
		if i < 0 {
			i = len(s)
		}
		op, err := ParseOperator(s[:i])
		if err != nil {
			return fmt.Errorf("cable: %s message at pos %d: %w",
				m.Role, len(c.Thread), err)
		}
		if op, ok := op.(*Alternate); ok {
			alt = op.Index
		}
		m.Ops = append(m.Ops, op)
		if i == len(s) {
			s = ""
		} else {
			s = s[i+1:]
		}
	}
	if s = strings.TrimSpace(s); s == "" {
		return fmt.Errorf("cable: %s message at pos %d is empty: %w",
			m.Role, len(c.Thread), io.ErrUnexpectedEOF)
	}
	sort.Stable(Operators(m.Ops))

	if seq, ok := c.alt[alt]; ok || alt == 0 {
		c.alt[alt] = append(seq, len(c.Thread))
	} else {
		c.alt[alt] = append(c.alt[c.dominant()], len(c.Thread))
	}

	m.Contents = append(m.Contents, Content{Text: s})
	c.Thread = append(c.Thread, m)
	return nil
}

func (c *Cable) dominant() int {
	for _, v := range slices.Backward(c.Thread) {
		if v.Role != "user" {
			continue
		}
		ops := v.Ops
		for _, op := range ops {
			if alt, ok := op.(*Alternate); ok {
				return alt.Index
			}
		}
	}
	return 0
}

func (c *Cable) AppendUser(s string) {
	c.Thread = append(c.Thread, Message{Role: "user", Contents: []Content{{Text: s}}})
}

func (c *Cable) Empty() bool {
	return len(c.Thread) == 0
}

func (c *Cable) Tab(s string) (tab string) {
	if c.ws == "" {
		c.ws = "\t"
	}
	tab = c.ws
	if s != "" {
		tab += s
	}
	return
}

func (c *Cable) String() string {
	var s strings.Builder
	for i, m := range c.Thread {
		if i == 0 && m.Role == "system" {
			goto ops
		}
		s.WriteString(c.ws)
		switch m.Role {
		case "system":
			continue
		case "user":
			s.WriteString(simp.GuidelineInput)
		case "assistant":
			s.WriteString(simp.GuidelineOutput)
		}
		if len(m.Models) > 0 {
			s.WriteString(" " + strings.Join(m.Models, ","))
		}
		s.WriteString("\n")
	ops:
		for _, op := range m.Ops {
			s.WriteString(c.Tab(op.String()) + "\n")
		}
		s.WriteString(m.String())
	}
	return s.String()
}
