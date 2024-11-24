package driver

import (
	"context"
	"io"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

// NewOpenAI creates a new OpenAI client.
func NewOpenAI(p config.Provider) (*OpenAI, error) {
	c := openai.DefaultConfig(p.APIKey)
	if p.BaseURL != "" {
		c.BaseURL = p.BaseURL
	}
	client := openai.NewClientWithConfig(c)
	return &OpenAI{Client: *client, p: p}, nil
}

// OpenAI is the most basic kind of driver, because it's the API that we're emulating.
//
// Big think!
type OpenAI struct {
	openai.Client
	p config.Provider
}

func (o *OpenAI) List(ctx context.Context) ([]simp.Model, error) {
	models, err := o.Client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	return models.Models, nil
}

func (o *OpenAI) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	e, err = o.Client.CreateEmbeddings(ctx, req)
	return
}

func (o *OpenAI) Complete(ctx context.Context, req simp.Complete) (c *simp.Completion, err error) {
	c = &simp.Completion{}
	if !req.Stream {
		c.ChatCompletionResponse, err = o.CreateChatCompletion(ctx, req)
		return
	}

	s, err := o.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}
	c.Stream = make(chan openai.ChatCompletionStreamResponse)
	go func() {
		defer close(c.Stream)
		for {
			r, err := s.Recv()
			switch err {
			case nil:
				c.Stream <- r
			case io.EOF:
				return
			default:
				c.Err = err
				// Send error as final message
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: "error",
					}},
				}
				return
			}
		}
	}()
	return
}
