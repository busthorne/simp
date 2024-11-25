package driver

import (
	"context"

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

func (o *OpenAI) Embed(ctx context.Context, req simp.Embed) (simp.Embeddings, error) {
	return o.Client.CreateEmbeddings(ctx, req)
}

func (o *OpenAI) Complete(ctx context.Context, req simp.Complete) (simp.Completion, error) {
	return o.Client.CreateChatCompletion(ctx, req)
}
