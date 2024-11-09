package driver

import (
	"context"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/busthorne/simp/dify"
	"github.com/sashabaranov/go-openai"
)

// NewDify creates a new Dify provider, implementing the official API.
func NewDify(p config.Provider) *Dify {
	return &Dify{Provider: p}
}

// Dify is a workflow GUI thing that can be used to build LLM agents.
type Dify struct {
	Provider config.Provider
}

func (o *Dify) List(ctx context.Context) ([]simp.Model, error) {
	return nil, simp.ErrUnsupported
}

func (o *Dify) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	err = simp.ErrUnsupported
	return
}

func (o *Dify) Complete(ctx context.Context, req simp.Complete) (*simp.Completion, error) {
	c := &simp.Completion{}
	// TODO: get user, conversation_id from some state
	difyReq := &dify.ChatMessageRequest{
		Inputs:         map[string]any{},
		Query:          req.Messages[len(req.Messages)-1].Content,
		User:           "",
		ConversationID: "",
	}

	// TODO: bearer
	client := dify.NewClient(o.Provider.BaseURL, "bearer")
	if req.Stream {
		resp, err := client.Api().ChatMessagesStream(context.Background(), difyReq)
		if err != nil {
			return c, err
		}
		c.Stream = make(chan openai.ChatCompletionStreamResponse)
		go func() {
			defer close(c.Stream)
			for chunk := range resp {
				if err := chunk.Err; err != nil {
					c.Err = err
					c.Stream <- openai.ChatCompletionStreamResponse{
						Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "error"}},
					}
					return
				}
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{Delta: openai.ChatCompletionStreamChoiceDelta{Content: chunk.Answer}},
					},
				}
			}
			c.Stream <- openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
			}
		}()
	}
	resp, err := client.Api().ChatMessages(context.Background(), difyReq)
	if err != nil {
		return c, err
	}
	c.Choices = []openai.ChatCompletionChoice{
		{Message: openai.ChatCompletionMessage{Content: resp.Answer}},
	}
	return c, nil
}
