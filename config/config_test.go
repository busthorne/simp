package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParsePath(t *testing.T) {
	dir, err := os.MkdirTemp("", "simp-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	want := Config{}
	b, err := Configure("simp", want)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, "simp.hcl"), b, 0644)
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("-want +got\n%s", diff)
	}
}
