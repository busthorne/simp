package main

import (
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/busthorne/simp/driver"
	"github.com/sashabaranov/go-openai"
	gemini "google.golang.org/api/option"
)

// findWaldo will check if the daemon is configured, and will simply create a daemon driver;
// if unsuccessful, it will search the configured models, cached models from provider lists,
// and will try to refresh the outdated lists!
func findWaldo(alias string) (simp.Driver, config.Model, error) {
	if d := cfg.Daemon; !*daemon && d != nil {
		drv := driver.NewDaemon(*d)
		if err := drv.Ping(); err != nil {
			stderr(err)
		} else {
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
		ring, err := keyringFor(p, cfg)
		if err != nil {
			stderr("keyring error:", err)
			exit(1)
		}
		item, err := ring.Get("apikey")
		if err != nil {
			stderr("keyring read error:", err)
			exit(1)
		}
		apikey = string(item.Data)
	} else {
		apikey = p.APIKey
	}
	switch p.Driver {
	case "openai":
		c := openai.DefaultConfig(apikey)
		c.BaseURL = p.BaseURL
		return driver.NewOpenAI(c)
	case "anthropic":
		return driver.NewAnthropic(anthropic.WithAPIKey(apikey))
	case "gemini":
		return driver.NewGemini(gemini.WithAPIKey(apikey))
	// case "dify":
	default:
		fmt.Printf("unsupported driver: %q\n", p.Driver)
		exit(1)
	}
	return nil
}
