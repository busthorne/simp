package cable

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

//go:embed testdata/*.c.md
var cables embed.FS

func TestParseCable(t *testing.T) {
	sources, err := cables.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{}
	for _, source := range sources {
		k := ""
		if s := strings.Split(source.Name(), "."); len(s) != 3 || s[1] != "c" {
			continue
		} else {
			k = s[0]
		}
		b, err := cables.ReadFile(filepath.Join("testdata", source.Name()))
		if err != nil {
			t.Fatal(err)
		}
		tests[k] = string(b)
	}

	trim := func(s string) string {
		return strings.Trim(s, "\n")
	}

	for test, want := range tests {
		t.Run(test, func(t *testing.T) {
			got, err := ParseCable(want)
			if err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(trim(want), trim(got.String())); diff != "" {
				t.Logf("thread:\n%+v", got.Thread)
				t.Error(diff)
			}
			if m := got.Thread[0]; m.Role == "system" {
				gota := [][]int{}
				system := strings.Trim(m.content, "`\n")
				if err := json.Unmarshal([]byte(system), &gota); err != nil {
					t.Logf("system prompt is not alternate sequence: %q", system)
					t.Fatal(err)
				}
				wanta := make([][]int, len(gota))
				for i := range gota {
					if alts, ok := got.alt[i]; ok {
						wanta[i] = alts
					} else {
						t.Fatalf("no alternate sequence for #%d", i)
					}
				}
				if diff := cmp.Diff(wanta, gota); diff != "" {
					t.Logf("thread:\n%+v", got.Thread)
					t.Error(diff)
				}
			}
		})
	}
}
