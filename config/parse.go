package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

func ParsePath(path string) (*Config, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	c := &Config{
		Diagnostics: make(map[string]hcl.Diagnostics),
	}

	parser := hclparse.NewParser()
	for _, f := range files {
		var fc Config
		diagnose := func(err error) {
			c.Diagnostics[f.Name()] = append(c.Diagnostics[f.Name()], &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  err.Error(),
			})
		}

		if f.IsDir() || filepath.Ext(f.Name()) != ".hcl" {
			continue
		}
		hclf, d := parser.ParseHCLFile(filepath.Join(path, f.Name()))
		if d.HasErrors() {
			c.Diagnostics[f.Name()] = d
			continue
		}
		// TODO: need to parse all before decoding to build eval context?
		d = gohcl.DecodeBody(hclf.Body, nil, &fc)
		if d.HasErrors() {
			c.Diagnostics[f.Name()] = d
			continue
		}
		if fc.Default != nil {
			if c.Default != nil {
				diagnose(fmt.Errorf("duplicate default block"))
			}
			c.Default = fc.Default
		}
		if fc.Daemon != nil {
			if c.Daemon != nil {
				diagnose(fmt.Errorf("duplicate daemon block"))
			}
			c.Daemon = fc.Daemon
		}
		if fc.History != nil {
			if c.History != nil {
				diagnose(fmt.Errorf("duplicate history block"))
			}
			c.History = fc.History
		}
		if fc.Auth != nil {
			c.Auth = append(c.Auth, fc.Auth...)
		}
		if fc.Providers != nil {
			c.Providers = append(c.Providers, fc.Providers...)
		}
	}
	if n := len(c.Diagnostics); n > 0 {
		errors, warnings := 0, 0
		for _, sticks := range c.Diagnostics {
			for _, d := range sticks {
				switch d.Severity {
				case hcl.DiagError:
					errors++
				case hcl.DiagWarning:
					warnings++
				}
			}
		}
		err = fmt.Errorf("%d errors, %d warnings occured in %d files", errors, warnings, n)
	}
	return c, err
}
