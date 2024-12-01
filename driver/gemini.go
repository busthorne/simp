package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	genai "github.com/google/generative-ai-go/genai"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// NewGemini creates a new Gemini client.
func NewGemini(p config.Provider) (*Gemini, error) {
	g := &Gemini{p: p}
	g.options = append(g.options, option.WithAPIKey(p.APIKey))
	return g, nil
}

// Gemini implements the driver interface for Google's Gemini API
type Gemini struct {
	options []option.ClientOption
	p       config.Provider
}

func (g *Gemini) List(ctx context.Context) ([]openai.Model, error) {
	// Gemini currently has limited models, so we'll return a static list
	return []openai.Model{
		{ID: "gemini-pro"},
		{ID: "gemini-pro-vision"},
	}, nil
}

func (g *Gemini) Embed(ctx context.Context, req openai.EmbeddingRequest) (e openai.EmbeddingResponse, err error) {
	client, err := genai.NewClient(ctx, g.options...)
	if err != nil {
		return e, err
	}
	model := client.EmbeddingModel(req.Model)
	batch := model.NewBatch()
	switch input := req.Input.(type) {
	case string:
		batch.AddContent(genai.Text(input))
	case []string:
		for _, s := range input {
			batch.AddContent(genai.Text(s))
		}
	default:
		return e, simp.ErrUnsupportedInput
	}
	resp, err := model.BatchEmbedContents(ctx, batch)
	if err != nil {
		return e, err
	}
	for i, embedding := range resp.Embeddings {
		e.Data = append(e.Data, openai.Embedding{
			Index:     i,
			Embedding: embedding.Values,
		})
	}
	return e, nil
}

func (g *Gemini) Complete(ctx context.Context, req openai.CompletionRequest) (c openai.CompletionResponse, err error) {
	return c, simp.ErrNotImplemented
}

func (g *Gemini) Chat(ctx context.Context, req openai.ChatCompletionRequest) (c openai.ChatCompletionResponse, err error) {
	client, err := genai.NewClient(ctx, g.options...)
	if err != nil {
		return c, err
	}
	model := client.GenerativeModel(req.Model)
	chat := model.StartChat()
	// Convert the messages to Gemini format
	for i, msg := range req.Messages {
		var role string
		switch msg.Role {
		case "system":
			if i != 0 {
				return c, fmt.Errorf("misplaced system message")
			}
			model.SystemInstruction = genai.NewUserContent(genai.Text(msg.Content))
		case "user":
			role = "user"
		case "assistant":
			role = "model"
		default:
			return c, simp.ErrUnsupportedRole
		}
		var parts []genai.Part
		switch {
		case msg.Content != "":
			parts = []genai.Part{genai.Text(msg.Content)}
		case len(msg.MultiContent) > 0:
			for j, part := range msg.MultiContent {
				switch part.Type {
				case openai.ChatMessagePartTypeText:
					parts = append(parts, genai.Text(part.Text))
				case openai.ChatMessagePartTypeImageURL:
					mime, b, err := url2image64(ctx, part.ImageURL.URL)
					if err != nil {
						return c, fmt.Errorf("message %d part %d: %w", i, j, err)
					}
					m := strings.TrimPrefix(mime, "image/")
					switch m {
					case "jpeg", "png":
					default:
						return c, fmt.Errorf("message %d part %d: %w", i, j, simp.ErrUnsupportedMime)
					}
					parts = append(parts, genai.ImageData(m, b))
				default:
					return c, fmt.Errorf("message %d part %d: type %s is not supported", i, j, part.Type)
				}
			}
		default:
			return c, fmt.Errorf("empty message %d", i+1)
		}
		chat.History = append(chat.History, &genai.Content{Parts: parts, Role: role})
	}
	var prompt *genai.Content
	switch last := chat.History[len(chat.History)-1]; last.Role {
	case "user":
		prompt = last
	default:
		return c, fmt.Errorf("thread must end with a user message")
	}
	if req.Stream {
		c.Stream = make(chan openai.ChatCompletionStreamResponse, 1)
	} else {
		resp, err := chat.SendMessage(ctx, prompt.Parts...)
		if err != nil {
			return c, err
		}
		// Convert Gemini response to OpenAI format
		for i, s := range g.choose(resp.Candidates) {
			c.Choices = append(c.Choices, openai.ChatCompletionChoice{
				Index:   i,
				Message: openai.ChatCompletionMessage{Content: s},
			})
		}
		return c, nil
	}
	go func() {
		defer close(c.Stream)
		iter := chat.SendMessageStream(ctx, prompt.Parts...)
		for {
			resp, err := iter.Next()
			switch err {
			case nil:
			case iterator.Done:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
				}
				return
			default:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "error"}},
					Error:   err,
				}
				return
			}
			// Convert Gemini response to OpenAI format
			choices := []openai.ChatCompletionStreamChoice{}
			for i, s := range g.choose(resp.Candidates) {
				choices = append(choices, openai.ChatCompletionStreamChoice{
					Index: i,
					Delta: openai.ChatCompletionStreamChoiceDelta{Content: s},
				})
			}
			c.Stream <- openai.ChatCompletionStreamResponse{Choices: choices}
		}
	}()
	return c, nil
}

func (g *Gemini) choose(cans []*genai.Candidate) (choices []string) {
	for _, c := range cans {
		var s strings.Builder
		for _, part := range c.Content.Parts {
			s.WriteString(fmt.Sprintf("%s", part))
		}
		choices = append(choices, s.String())
	}
	return choices
}
