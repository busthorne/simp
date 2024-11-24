package driver

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/busthorne/simp/config"
)

const dialTimeout = time.Second

// NewDaemon creates a daemon client for the simulating proxy.
func NewDaemon(cfg config.Daemon) (*Daemon, error) {
	baseUrl := cfg.BaseURL()
	d, err := NewOpenAI(config.Provider{BaseURL: baseUrl})
	if err != nil {
		return nil, err
	}
	return &Daemon{
		OpenAI:  *d,
		baseUrl: baseUrl,
	}, nil
}

// Daemon is the hairpin driver: basically, glorified IPC over HTTP.
//
// Big think!
type Daemon struct {
	OpenAI

	baseUrl string
}

func (d *Daemon) Ping() error {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: dialTimeout}).DialContext,
		},
	}
	resp, err := client.Get(d.baseUrl + "/ping")
	if err != nil {
		return fmt.Errorf("daemon: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon: %s", resp.Status)
	}
	return nil
}
