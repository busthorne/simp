package main

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/auth"
	"github.com/busthorne/simp/config"
	"github.com/busthorne/simp/driver"
	"github.com/sashabaranov/go-openai"
)

// findWaldo will check if the daemon is configured, and will simply create a daemon driver;
// if unsuccessful, it will search the configured models, cached models from provider lists,
// and will try to refresh the outdated lists!
func findWaldo(alias string) (simp.Driver, config.Model, error) {
	if d := cfg.Daemon; !*daemon && d != nil {
		drv := driver.NewDaemon(*d)
		if err := drv.Ping(); err == nil {
			return drv, config.Model{Name: alias}, nil
		}
	}
	m, p, ok := cfg.LookupModel(alias)
	if ok {
		return drive(p), m, nil
	}
	// TODO: search cache for model list
	return nil, m, simp.ErrNotFound
}

func drive(p config.Provider) simp.Driver {
	var apikey string
	if p.APIKey == "" {
		for _, a := range cfg.Auth {
			if a.Type != "keyring" {
				continue
			}
			if p.Keyring == "" || a.Name == p.Keyring {
				ring, err := auth.NewKeyring(a, &p)
				if err != nil {
					continue
				}
				item, err := ring.Get("apikey")
				if err != nil {
					continue
				}
				apikey = string(item.Data)
				break
			}
		}
	} else {
		apikey = p.APIKey
	}
	switch p.Driver {
	case "openai":
		c := openai.DefaultConfig(apikey)
		c.BaseURL = p.BaseURL
		return driver.NewOpenAI(c)
	case "anthropic":
		return driver.NewAnthropic(option.WithAPIKey(apikey))
	// case "dify":
	default:
		fmt.Printf("unsupported driver: %q\n", p.Driver)
		exit(1)
	}
	return nil
}
