package driver

import (
	"context"

	"github.com/busthorne/simp"
	"github.com/sashabaranov/go-openai"
)

// NewOpenAI creates a new OpenAI client.
func NewOpenAI(cfg openai.ClientConfig) *OpenAI {
	return &OpenAI{Client: *openai.NewClientWithConfig(cfg)}
}

// OpenAI is the most basic kind of driver, because it's the API that we're emulating.
//
// Big think!
type OpenAI struct {
	openai.Client
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
	if req.Stream {
		s, ret := o.CreateChatCompletionStream(ctx, req)
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
		c.ChatCompletionResponse, err = o.CreateChatCompletion(ctx, req)
	}
	return
}
