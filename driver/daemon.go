package driver

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

const dialTimeout = time.Second

// NewDaemon creates a daemon client for the simulating proxy.
func NewDaemon(d config.Daemon) *Daemon {
	baseUrl := d.BaseURL()
	c := openai.DefaultConfig("")
	c.BaseURL = baseUrl
	return &Daemon{
		OpenAI:  *NewOpenAI(c),
		baseUrl: baseUrl,
	}
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
