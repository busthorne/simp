package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/busthorne/keyring"
)

var (
	ƒ                  = fmt.Sprintf
	ø                  = fmt.Errorf
	nonAlphanumeric    = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	errNonAlphanumeric = errors.New("does not conform to " +
		nonAlphanumeric.String())
)

func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	err, collect := validate("")
	collect(c.Daemon.Validate(), "daemon")
	collect(c.History.Validate(), "history")

	type count struct{}
	type duplicates map[string]count
	auths := duplicates{}
	providers := duplicates{}
	models := duplicates{}
	defaults := map[string]int{}
	for i, a := range c.Auth {
		if a.Name == "default" {
			a.Default = true
			c.Auth[i] = a
		}
		collect(a.Validate(), ƒ(`auth "%s" "%s"`, a.Type, a.Name))

		id := a.Type + ":" + a.Name
		if _, ok := auths[id]; ok {
			collect(ø(`duplicate auth "%s" "%s"`, a.Name, a.Type))
		}
		auths[id] = count{}
		if n, ok := defaults[a.Type]; ok {
			defaults[a.Type] = n + 1
		} else {
			if a.Default {
				n = 1
			}
			defaults[a.Type] = n
		}
	}
	for typ, n := range defaults {
		switch n {
		case 1:
		case 0:
			collect(ø("no default %s block configured", typ))
		default:
			collect(ø("multiple default %s blocks configured", typ))
		}
	}
	for _, p := range c.Providers {
		collect(p.Validate(), ƒ(`provider "%s" "%s"`, p.Driver, p.Name))

		id := p.Driver + ":" + p.Name
		if _, ok := providers[id]; ok {
			collect(ø(`duplicate provider "%s" "%s"`, p.Driver, p.Name))
		}
		providers[id] = count{}
		for _, m := range p.Models {
			if _, ok := models[m.Name]; ok {
				collect(ø("model %s is already in use as name or alias", m.Name))
			}
			models[m.Name] = count{}
			for _, a := range m.Alias {
				if _, ok := models[a]; ok {
					collect(ø("model %s is already in use as name or alias", a))
				}
				models[a] = count{}
			}
		}
	}
	err.Title = ƒ("%d errors, 0 warnings", err.Count())
	return err.Invalid()
}

func (a *Auth) Validate() error {
	if nonAlphanumeric.MatchString(a.Name) {
		return ø("%s: %w", a.Name, errNonAlphanumeric)
	}
	backends := keyring.AvailableBackends()
	for _, b := range backends {
		if b == keyring.BackendType(a.Backend) {
			return nil
		}
	}
	return ø("available backends: %v", backends)
}

func (p *Provider) Validate() error {
	if nonAlphanumeric.MatchString(p.Name) {
		return ø("%s: %w", p.Name, errNonAlphanumeric)
	}
	err, collect := validate("")

	if p.Driver == "vertex" {
		if p.Project == "" {
			collect(ø("project is required for vertex driver"))
		}
		if p.Region == "" {
			collect(ø("region is required for vertex driver"))
		}
		if p.Batch && p.Dataset == "" {
			collect(ø("biquery dataset is required for vertex batching"))
		}
	}
	for _, m := range p.Models {
		collect(m.Validate(), ƒ(`model "%s" "%s"`, p.Name, m.Name))
	}
	// TODO: validate
	return err.Invalid()
}

func (m *Model) Validate() error {
	// TODO: validate
	return nil
}

func (d *Daemon) Validate() error {
	switch {
	case d == nil:
		return nil
	case d.DaemonAddr != "" && d.ListenAddr != "":
		return errors.New("you either use daemon_addr, or start one on listen_addr")
	case d.DaemonAddr == "" && d.ListenAddr == "":
		return errors.New("neither daemon_addr nor listen_addr is set")
	}
	return nil
}

// globToRegex converts a glob pattern to a regular expression
func globToRegex(pattern string) string {
	parts := strings.Split(pattern, "/")
	for i, part := range parts {
		if part == "**" {
			parts[i] = ".*"
		} else if strings.Contains(part, "*") {
			parts[i] = "([^/]+)"
		}
	}
	return "^" + strings.Join(parts, "/") + "($|/.*)"
}

func (h *History) Validate() error {
	if h == nil {
		return nil
	}
	err, collect := validate("")
	for i, hp := range h.Paths {
		if hp.Path == "" {
			collect(ø("path %d is empty", i))
			continue
		}
		if glob, err := Glob(hp.Path); err != nil {
			collect(ø("invalid glob in path %s: %w", hp.Path, err))
		} else {
			h.Paths[i].Glob = glob
		}
	}
	return err.Invalid()
}

func Glob(path string) (*regexp.Regexp, error) {
	return regexp.Compile(globToRegex(path))
}
