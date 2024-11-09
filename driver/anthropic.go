package driver

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/busthorne/simp"
	"github.com/sashabaranov/go-openai"
)

// NewAnthropic creates a new Anthropic client.
func NewAnthropic(opts ...option.RequestOption) *Anthropic {
	return &Anthropic{Client: *anthropic.NewClient(opts...)}
}

// Anthropic implements the driver interface for Anthropic's API
type Anthropic struct {
	anthropic.Client
}

func (a *Anthropic) List(ctx context.Context) ([]simp.Model, error) {
	// Anthropic doesn't have a list models endpoint, return supported models statically
	return []simp.Model{
		{ID: "claude-3-sonnet"},
		{ID: "claude-3-opus"},
		{ID: "claude-3-haiku"},
		{ID: "claude-3-5-sonnet"},
		{ID: "claude-3-5-haiku"},
	}, nil
}

func (a *Anthropic) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	err = simp.ErrUnsupported
	return
}

func (a *Anthropic) Complete(ctx context.Context, req simp.Complete) (c *simp.Completion, ret error) {
	c = &simp.Completion{}

	// Convert messages to Anthropic format
	messages := make([]anthropic.MessageParam, len(req.Messages))
	for i, msg := range req.Messages {
		var blocks []anthropic.MessageParamContentUnion
		// Handle text content
		if msg.Content != "" {
			blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
		}
		// Handle images
		for j, part := range msg.MultiContent {
			switch part.Type {
			case openai.ChatMessagePartTypeText:
				blocks = append(blocks, anthropic.NewTextBlock(part.Text))
			case openai.ChatMessagePartTypeImageURL:
				b64, mime, err := a.url2image64(part.ImageURL.URL)
				if err != nil {
					return c, fmt.Errorf("message %d part %d: %w", i, j, err)
				}
				blocks = append(blocks, anthropic.NewImageBlockBase64(mime, b64))
			default:
				return c, fmt.Errorf("message %d part %d: unsupported '%v' multipart type", i, j, part.Type)
			}
		}
		switch msg.Role {
		case "user":
			messages[i] = anthropic.NewUserMessage(blocks...)
		case "assistant":
			messages[i] = anthropic.NewAssistantMessage(blocks...)
		default:
			return c, simp.ErrUnsupportedRole
		}
	}
	params := anthropic.MessageNewParams{
		Model:    anthropic.F(req.Model),
		Messages: anthropic.F(messages),
	}
	if req.MaxTokens > 0 {
		params.MaxTokens = anthropic.F(int64(req.MaxTokens))
	} else {
		params.MaxTokens = anthropic.F(int64(4096))
	}
	if req.Temperature > 0 {
		params.Temperature = anthropic.F(float64(req.Temperature))
	}
	if req.TopP > 0 {
		params.TopP = anthropic.F(float64(req.TopP))
	}

	if !req.Stream {
		resp, err := a.Messages.New(ctx, params)
		if err != nil {
			return c, err
		}
		c.ChatCompletionResponse = openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: resp.Content[0].Text,
				},
			}},
		}
		return
	}
	c.Stream = make(chan openai.ChatCompletionStreamResponse)
	go func() {
		defer close(c.Stream)
		stream := a.Messages.NewStreaming(ctx, params)
		for stream.Next() {
			event := stream.Current()
			switch delta := event.Delta.(type) {
			case anthropic.ContentBlockDeltaEventDelta:
				if delta.Text == "" {
					continue
				}
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: delta.Text,
						},
					}},
				}
			}
		}
		if err := stream.Err(); err != nil {
			c.Err = err
			// Send error as final message
			c.Stream <- openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{{
					FinishReason: "error",
				}},
			}
		}
	}()
	return
}

func (a *Anthropic) url2image64(url string) (mime string, b64 string, err error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	mime = resp.Header.Get("Content-Type")
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		err = simp.ErrUnsupportedMime
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	b64 = base64.StdEncoding.EncodeToString(data)
	return
}