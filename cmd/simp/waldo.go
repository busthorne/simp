package main

import (
	"fmt"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/busthorne/simp/driver"
)

// findWaldo will check if the daemon is configured, and will simply create a daemon driver;
// if unsuccessful, it will search the configured models, cached models from provider lists,
// and will try to refresh the outdated lists!
func findWaldo(alias string) (simp.Driver, config.Model, error) {
	m := config.Model{Name: alias}
	if d := cfg.Daemon; !*daemon && d != nil {
		drv, err := driver.NewDaemon(*d)
		if err != nil {
			return nil, m, fmt.Errorf("daemon driver: %w", err)
		}
		if err := drv.Ping(); err != nil {
			return nil, m, fmt.Errorf("daemon ping: %w", err)
		}
		return drv, m, nil
	}
	m, p, ok := cfg.LookupModel(alias)
	if ok {
		d, err := drive(p)
		if err != nil {
			return nil, m, fmt.Errorf("provider %s: %w", p.Name, err)
		}
		return d, m, nil
	}
	// TODO: search cache for model list
	return nil, m, simp.ErrNotFound
}

func findBaldo(alias string) (simp.BatchDriver, config.Model, error) {
	d, m, err := findWaldo(alias)
	if err != nil {
		return nil, m, err
	}
	if bd, ok := d.(simp.BatchDriver); ok {
		return bd, m, nil
	}
	return nil, m, simp.ErrNotFound
}

func drive(p config.Provider) (d simp.Driver, err error) {
	if p.APIKey == "" {
		ring, err := keyringFor(p, cfg)
		if err != nil {
			return nil, err
		}
		item, err := ring.Get("apikey")
		if err != nil {
			return nil, err
		}
		p.APIKey = string(item.Data)
	}
	switch p.Driver {
	case "openai":
		d, err = driver.NewOpenAI(p)
	case "anthropic":
		d, err = driver.NewAnthropic(p)
	case "gemini":
		d, err = driver.NewGemini(p)
	case "vertex":
		d, err = driver.NewVertex(p)
	// case "dify":
	default:
		err = fmt.Errorf(`unsupported driver "%s"`, p.Driver)
	}
	return
}
