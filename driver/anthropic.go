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
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

// NewAnthropic creates a new Anthropic client.
func NewAnthropic(p config.Provider) (*Anthropic, error) {
	cli := anthropic.NewClient(option.WithAPIKey(p.APIKey))
	return &Anthropic{Client: *cli, p: p}, nil
}

// Anthropic implements the driver interface for Anthropic's API
type Anthropic struct {
	anthropic.Client

	p config.Provider
}

func (a *Anthropic) List(ctx context.Context) ([]simp.Model, error) {
	return nil, simp.ErrNotImplemented
}

func (a *Anthropic) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	err = simp.ErrNotImplemented
	return
}

func (a *Anthropic) Complete(ctx context.Context, req simp.Complete) (c simp.Completion, ret error) {
	// Convert messages to Anthropic format
	messages := []anthropic.MessageParam{}
	system := ""
	for i, msg := range req.Messages {
		if msg.Role == "system" {
			if i != 0 {
				return c, fmt.Errorf("misplaced system message")
			}
			system = msg.Content
			continue
		}
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
				mime, b, err := url2image64(ctx, part.ImageURL.URL)
				if err != nil {
					return c, fmt.Errorf("message %d part %d: %w", i, j, err)
				}
				blocks = append(blocks, anthropic.NewImageBlockBase64(mime, base64.StdEncoding.EncodeToString(b)))
			default:
				return c, fmt.Errorf("message %d part %d: type %s is not supported", i, j, part.Type)
			}
		}
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(blocks...))
		default:
			return c, fmt.Errorf("message %d: %w", i+1, simp.ErrUnsupportedRole)
		}
	}
	params := anthropic.MessageNewParams{
		Model:    anthropic.F(req.Model),
		Messages: anthropic.F(messages),
	}
	if system != "" {
		params.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(system),
		})
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
		c.Choices = []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: resp.Content[0].Text,
			},
		}}
		return
	}
	c.Stream = make(chan openai.ChatCompletionStreamResponse, 1)
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
			c.Stream <- openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{{
					FinishReason: "error",
				}},
				Error: err,
			}
		}
	}()
	return
}

func url2image64(ctx context.Context, url string) (mime string, b []byte, err error) {
	resp, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", nil, err
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
		return "", nil, err
	}
	b = data
	return
}
