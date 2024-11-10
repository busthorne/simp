package driver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

const dialTimeout = time.Second

// NewDaemon creates a daemon client for the simulating proxy.
func NewDaemon(d config.Daemon) *Daemon {
	baseUrl := strings.ReplaceAll(d.BaseURL(), "0.0.0.0", "127.0.0.1")
	return &Daemon{
		Client: *openai.NewClientWithConfig(openai.ClientConfig{
			BaseURL: baseUrl,
		}),
		baseUrl: baseUrl,
	}
}

// Daemon is the hairpin driver: basically, glorified IPC over HTTP.
//
// Big think!
type Daemon struct {
	openai.Client

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
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon not responding: %s", resp.Status)
	}
	return nil
}

func (d *Daemon) List(ctx context.Context) ([]simp.Model, error) {
	models, err := d.Client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	return models.Models, nil
}

func (d *Daemon) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	e, err = d.Client.CreateEmbeddings(ctx, req)
	return
}

func (d *Daemon) Complete(ctx context.Context, req simp.Complete) (c *simp.Completion, err error) {
	c = &simp.Completion{}
	if req.Stream {
		s, ret := d.CreateChatCompletionStream(ctx, req)
		if ret != nil {
			return c, ret
		}
		c.Stream = make(chan openai.ChatCompletionStreamResponse)
		go func() {
			defer close(c.Stream)
			for {
				r, err := s.Recv()
				if err != nil {
					c.Err = err
					// Send error as final message
					c.Stream <- openai.ChatCompletionStreamResponse{
						Choices: []openai.ChatCompletionStreamChoice{{
							FinishReason: "error",
						}},
					}
					return
				}
				c.Stream <- r
			}
		}()
	} else {
		c.ChatCompletionResponse, err = d.CreateChatCompletion(ctx, req)
	}
	return
}
