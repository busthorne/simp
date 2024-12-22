package simp

import (
	"embed"
	"strings"
	"testing"
)

//go:embed testdata/*.cable.md
var cables embed.FS

func TestParseCable(t *testing.T) {
	sources, err := cables.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{}
	for _, source := range sources {
		name := strings.Split(source.Name(), ".")[0]
		b, err := cables.ReadFile("cables/" + source.Name())
		if err != nil {
			t.Fatal(err)
		}
		tests[name] = string(b)
	}

	for test, s := range tests {
		t.Run(test, func(t *testing.T) {
			got, err := ParseCable(s)
			if err != nil {
				t.Error(err)
			}
			t.Log(got)
		})
	}
}
