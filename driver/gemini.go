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

func (g *Gemini) List(ctx context.Context) ([]simp.Model, error) {
	// Gemini currently has limited models, so we'll return a static list
	return []simp.Model{
		{ID: "gemini-pro"},
		{ID: "gemini-pro-vision"},
	}, nil
}

func (g *Gemini) Embed(ctx context.Context, req simp.Embed) (e simp.Embeddings, err error) {
	err = simp.ErrNotImplemented
	return
}

func (g *Gemini) Complete(ctx context.Context, req simp.Complete) (*simp.Completion, error) {
	client, err := genai.NewClient(ctx, g.options...)
	if err != nil {
		return nil, err
	}
	model := client.GenerativeModel(req.Model)
	chat := model.StartChat()
	// Convert the messages to Gemini format
	for i, msg := range req.Messages {
		var role string
		switch msg.Role {
		case "system":
			if i != 0 {
				return nil, fmt.Errorf("misplaced system message")
			}
			model.SystemInstruction = genai.NewUserContent(genai.Text(msg.Content))
		case "user":
			role = "user"
		case "assistant":
			role = "model"
		default:
			return nil, simp.ErrUnsupportedRole
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
						return nil, fmt.Errorf("message %d part %d: %w", i, j, err)
					}
					m := strings.TrimPrefix(mime, "image/")
					switch m {
					case "jpeg", "png":
					default:
						return nil, fmt.Errorf("message %d part %d: %w", i, j, simp.ErrUnsupportedMime)
					}
					parts = append(parts, genai.ImageData(m, b))
				default:
					return nil, fmt.Errorf("message %d part %d: type %s is not supported", i, j, part.Type)
				}
			}
		default:
			return nil, fmt.Errorf("empty message %d", i+1)
		}
		chat.History = append(chat.History, &genai.Content{Parts: parts, Role: role})
	}
	var prompt *genai.Content
	switch last := chat.History[len(chat.History)-1]; last.Role {
	case "user":
		prompt = last
	default:
		return nil, fmt.Errorf("thread must end with a user message")
	}

	c := &simp.Completion{}

	if !req.Stream {
		resp, err := chat.SendMessage(ctx, prompt.Parts...)
		if err != nil {
			return nil, err
		}
		// Convert Gemini response to OpenAI format
		choices := []openai.ChatCompletionChoice{}
		for i, s := range g.choose(resp.Candidates) {
			choices = append(choices, openai.ChatCompletionChoice{
				Index:   i,
				Message: openai.ChatCompletionMessage{Content: s},
			})
		}
		c.ChatCompletionResponse = openai.ChatCompletionResponse{Choices: choices}
		return c, nil
	}

	c.Stream = make(chan openai.ChatCompletionStreamResponse)
	go func() {
		defer close(c.Stream)
		iter := chat.SendMessageStream(ctx, prompt.Parts...)
		for {
			resp, err := iter.Next()
			if err == iterator.Done {
				return
			}
			if err != nil {
				c.Err = err
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: "error",
					}},
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
