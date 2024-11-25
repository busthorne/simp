package driver

import (
	"context"
	"fmt"

	"cloud.google.com/go/vertexai/genai"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// NewVertex creates a new Vertex AI client.
func NewVertex(p config.Provider) (*Vertex, error) {
	return &Vertex{p: p}, nil
}

// Vertex implements the driver interface using Google's Vertex AI API
type Vertex struct {
	p config.Provider
}

func (v *Vertex) client(ctx context.Context) (*genai.Client, error) {
	return genai.NewClient(ctx,
		v.p.Project,
		v.p.Region,
		option.WithCredentialsJSON([]byte(v.p.APIKey)))
}

func (v *Vertex) List(ctx context.Context) ([]simp.Model, error) {
	// Vertex AI doesn't have a direct model listing API
	// Return predefined list of available models
	return []simp.Model{
		{ID: "gemini-1.5-flash-001"},
		{ID: "gemini-1.5-flash-002"},
		{ID: "gemini-1.5-pro-001"},
		{ID: "gemini-1.5-pro-002"},
		{ID: "gemini-flash-experimental"},
		{ID: "gemini-pro-experimental"},
	}, nil
}

func (v *Vertex) Embed(ctx context.Context, req simp.Embed) (simp.Embeddings, error) {
	return simp.Embeddings{}, simp.ErrNotImplemented
}

func (v *Vertex) Complete(ctx context.Context, req simp.Complete) (c simp.Completions, err error) {
	client, err := v.client(ctx)
	if err != nil {
		return c, err
	}
	model := client.GenerativeModel(req.Model)
	cs := model.StartChat()
	h := []*genai.Content{}
	for i, msg := range req.Messages {
		if msg.Role == "system" {
			if i == 0 {
				model.SystemInstruction = &genai.Content{
					Parts: []genai.Part{genai.Text(msg.Content)},
				}
				continue
			}
			return c, fmt.Errorf("system message is misplaced")
		}
		c := &genai.Content{
			Role:  msg.Role,
			Parts: []genai.Part{genai.Text(msg.Content)},
		}
		h = append(h, c)
	}
	if len(h) == 0 {
		return c, fmt.Errorf("no messages to complete")
	}
	cs.History = h[:len(h)-1]
	tail := h[len(h)-1].Parts
	if !req.Stream {
		resp, err := cs.SendMessage(ctx, tail...)
		if err != nil {
			return c, err
		}
		for _, can := range resp.Candidates {
			c.Choices = append(c.Choices, openai.ChatCompletionChoice{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: fmt.Sprintf("%s", can.Content.Parts[0]),
				},
			})
		}
		c.Usage = openai.Usage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
		return c, nil
	}
	it := cs.SendMessageStream(ctx, tail...)
	c.Stream = make(chan openai.ChatCompletionStreamResponse, 1)
	go func() {
		defer close(c.Stream)
		for {
			chunk, err := it.Next()
			switch err {
			case nil:
				choices := []openai.ChatCompletionStreamChoice{}
				for _, can := range chunk.Candidates {
					choices = append(choices, openai.ChatCompletionStreamChoice{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: fmt.Sprintf("%s", can.Content.Parts[0]),
						},
					})
				}
				c.Stream <- openai.ChatCompletionStreamResponse{Choices: choices}
			case iterator.Done:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: "stop",
					}},
				}
			default:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: "error",
					}},
					Error: err,
				}
			}
		}
	}()
	return c, nil
}
