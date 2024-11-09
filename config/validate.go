package config

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/busthorne/keyring"
)

var (
	ƒ                  = fmt.Sprintf
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
	for _, a := range c.Auth {
		collect(a.Validate(), ƒ(`auth "%s" "%s"`, a.Type, a.Name))
	}
	for _, p := range c.Providers {
		collect(p.Validate(), ƒ(`provider "%s" "%s"`, p.Driver, p.Name))
	}
	err.Title = ƒ("%d errors, 0 warnings", len(err.Errors))
	return err.Invalid()
}

func (a *Auth) Validate() error {
	if nonAlphanumeric.MatchString(a.Name) {
		return fmt.Errorf("%s: %w", a.Name, errNonAlphanumeric)
	}
	backends := keyring.AvailableBackends()
	for _, b := range backends {
		if b == keyring.BackendType(a.Backend) {
			return nil
		}
	}
	return fmt.Errorf("available backends: %v", backends)
}

func (p *Provider) Validate() error {
	if nonAlphanumeric.MatchString(p.Name) {
		return fmt.Errorf("%s: %w", p.Name, errNonAlphanumeric)
	}
	err, collect := validate("")
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

func (h *History) Validate() error {
	if h == nil {
		return nil
	}
	// TODO: validate
	return nil
}
