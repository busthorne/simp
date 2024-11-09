package simp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseCable(t *testing.T) {
	const (
		system = "system"
		user   = "user"
		asst   = "assistant"
	)
	var (
		all = func(msg ...Message) []Message { return msg }
		msg = func(role, s string, ann ...string) Message {
			a := ""
			if ann != nil {
				a = ann[0]
			}
			return Message{Role: role, Content: s, Annotation: a}
		}
	)
	var tests = []struct {
		Source     string
		Want       []Message
		Error      error
		Whitespace string
	}{
		{
			Source:     "a\n\t>>>\nb",
			Want:       all(msg(system, "a"), msg(user, "b")),
			Whitespace: "\t",
		},
		{
			Source:     "  >>>>\na",
			Want:       all(msg(user, "a")),
			Whitespace: "  ",
		},
		{
			Source:     "\n\t>>>>>\na",
			Want:       all(msg(user, "a")),
			Whitespace: "\t",
		},
		{
			Source:     "a\n\t\t>>>\nb\n\t<<< gpt-4\nc",
			Want:       all(msg(system, "a"), msg(user, "b"), msg(asst, "c", "gpt-4")),
			Whitespace: "\t\t",
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			s := test.Source
			s_ := strings.ReplaceAll(s, "\t", "TAB")
			s_ = strings.ReplaceAll(s_, " ", "_")
			t.Logf("source:\n%s", s_)
			got, err := ParseCable(s)
			if err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(test.Want, got.Thread); diff != "" {
				t.Errorf("mismatch (-want +got):\n%v", diff)
			}
			// if got.Whitespace != test.Whitespace {
			// 	t.Errorf("whitespace mismatch: want %q, got %q", test.Whitespace, got.Whitespace)
			// }
		})
	}
}
