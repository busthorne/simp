package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
)

func TestParsePath(t *testing.T) {
	dir, err := os.MkdirTemp("", "simp-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	type list = []string
	want := Config{
		Default: &Default{
			Model: "r1",
		},
		Daemon: &Daemon{
			ListenAddr: "localhost:8080",
			AutoTLS:    true,
			AllowedIPs: list{"127.0.0.1/32", "10.0.0.0/8"},
		},
		Auth: []Auth{
			{
				Type:    "openai",
				Name:    "api",
				Backend: "api",
			},
		},
		Providers: []Provider{
			{
				Driver: "openai",
				Name:   "api",
				Models: []Model{
					{Name: "gpt-4o", Alias: list{"4o"}},
					{Name: "o3-mini", Alias: list{"o3"}, Thinking: true},
				},
			},
			{
				Driver: "openai",
				Name:   "jina",
				Models: []Model{
					{Name: "jina-clip-v2", Alias: list{"jc2"}, Embedding: true, Images: true},
				},
			},
		},
		History: &History{
			Location: "history",
			Paths: []HistoryPath{
				{Path: "/", Group: "root"},
			},
		},

		Diagnostics: make(map[string]hcl.Diagnostics),
	}
	b, err := Configure("simp", want)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, "simp.hcl"), b, 0600)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePath(dir)
	if err != nil {
		t.Error(err)
		for _, d := range got.Diagnostics {
			t.Error(d)
		}
		return
	}
	if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("-want +got\n%s", diff)
	}
}
