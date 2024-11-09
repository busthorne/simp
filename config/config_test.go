package config

import (
	"testing"

	"github.com/spewerspew/spew"
)

func TestParsePath(t *testing.T) {
	cfg, err := ParsePath(".")
	if err != nil {
		t.Error(err)
		for _, d := range cfg.Diagnostics {
			t.Error(d)
		}
		return
	}
	t.Logf("\n%s", spew.Sdump(cfg))
}
