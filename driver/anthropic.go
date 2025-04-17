package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

const anthropicBeta = "message-batches-2024-09-24"

// NewAnthropic creates a new Anthropic client.
func NewAnthropic(p config.Provider) (*Anthropic, error) {
	cli := anthropic.NewClient(
		option.WithAPIKey(p.APIKey),
		option.WithHeader("anthropic-beta", anthropicBeta),
	)
	return &Anthropic{Client: *cli, p: p}, nil
}

// Anthropic implements the driver interface for Anthropic's API
type Anthropic struct {
	anthropic.Client
	p config.Provider
}

func (a *Anthropic) translate(ctx context.Context, req openai.ChatCompletionRequest) (anthropic.BetaMessageNewParams, error) {
	m, _ := ctx.Value(simp.KeyModel).(config.Model)
	maxTokens := 4096
	if req.MaxTokens > 0 {
		maxTokens = int(req.MaxTokens)
	}
	p := anthropic.BetaMessageNewParams{
		Model:       anthropic.F(req.Model),
		Temperature: anthropic.F(float64(req.Temperature)),
		MaxTokens:   anthropic.F(int64(maxTokens)),
	}
	if req.TopP > 0 {
		p.TopP = anthropic.F(float64(req.TopP))
	} else if m.TopP != nil {
		p.TopP = anthropic.F(float64(*m.TopP))
	}
	if len(req.Stop) > 0 {
		p.StopSequences = anthropic.F(req.Stop)
	} else if len(m.Stop) > 0 {
		p.StopSequences = anthropic.F(m.Stop)
	}

	messages := []anthropic.BetaMessageParam{}
	alt := ""
	for i, msg := range req.Messages {
		switch role := msg.Role; role {
		case "system":
			if i != 0 {
				return p, fmt.Errorf("misplaced system message")
			}
			p.System = anthropic.F([]anthropic.BetaTextBlockParam{{
				Text: anthropic.F(msg.Content),
				Type: anthropic.F(anthropic.BetaTextBlockParamTypeText),
			}})
			continue
		case "user", "assistant":
			if alt == role {
				return p, fmt.Errorf("messages are not alternating")
			}
			alt = role
		default:
			return p, fmt.Errorf("message/%d: %q %w", i, role, simp.ErrUnsupportedRole)
		}

		var blocks []anthropic.BetaContentBlockParamUnion

		if msg.Content != "" {
			blocks = append(blocks, anthropic.BetaContentBlockParam{
				Type: anthropic.F(anthropic.BetaContentBlockParamTypeText),
				Text: anthropic.F(msg.Content),
			})
		}
		for j, part := range msg.MultiContent {
			switch part.Type {
			case openai.ChatMessagePartTypeText:
				blocks = append(blocks, anthropic.BetaTextBlockParam{
					Type: anthropic.F(anthropic.BetaTextBlockParamTypeText),
					Text: anthropic.F(part.Text),
				})
			case openai.ChatMessagePartTypeImageURL:
				mime, b, err := url2image64(ctx, part.ImageURL.URL)
				if err != nil {
					return p, fmt.Errorf("message %d part %d: %w", i, j, err)
				}
				blocks = append(blocks, anthropic.BetaImageBlockParam{
					Type: anthropic.F(anthropic.BetaImageBlockParamTypeImage),
					Source: anthropic.F(anthropic.BetaImageBlockParamSource{
						Type:      anthropic.F(anthropic.BetaImageBlockParamSourceTypeBase64),
						MediaType: anthropic.F(anthropic.BetaImageBlockParamSourceMediaType(mime)),
						Data:      anthropic.F(string(b)),
					}),
				})
			default:
				return p, fmt.Errorf("message %d part %d: type %s is not supported", i, j, part.Type)
			}
		}
		role := anthropic.BetaMessageParamRoleUser
		if msg.Role == "assistant" {
			role = anthropic.BetaMessageParamRoleAssistant
		}
		messages = append(messages, anthropic.BetaMessageParam{
			Content: anthropic.F(blocks),
			Role:    anthropic.F(role),
		})
	}
	p.Messages = anthropic.F(messages)
	return p, nil
}

func (a *Anthropic) List(ctx context.Context) ([]openai.Model, error) {
	return nil, simp.ErrNotImplemented
}

func (a *Anthropic) Embed(ctx context.Context, req openai.EmbeddingRequest) (e openai.EmbeddingResponse, err error) {
	return e, simp.ErrNotImplemented
}

func (a *Anthropic) Complete(ctx context.Context, req openai.CompletionRequest) (c openai.CompletionResponse, err error) {
	return c, simp.ErrNotImplemented
}

func (a *Anthropic) Chat(ctx context.Context, req openai.ChatCompletionRequest) (c openai.ChatCompletionResponse, ret error) {
	params, err := a.translate(ctx, req)
	if err != nil {
		return c, err
	}
	if !req.Stream {
		resp, err := a.Beta.Messages.New(ctx, params)
		if err != nil {
			return c, err
		}
		c.Usage = openai.Usage{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
		}
		c.Usage.TotalTokens = c.Usage.PromptTokens + c.Usage.CompletionTokens
		if n := resp.Usage.CacheReadInputTokens; n > 0 {
			c.Usage.PromptTokensDetails = &openai.PromptTokensDetails{
				CachedTokens: int(n),
			}
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
		var usage openai.Usage
		stream := a.Beta.Messages.NewStreaming(ctx, params)
		for i := 0; stream.Next(); i++ {
			event := stream.Current()
			if i == 0 {
				usage.PromptTokens = int(event.Message.Usage.InputTokens)
				if n := event.Message.Usage.CacheReadInputTokens; n > 0 {
					usage.PromptTokensDetails = &openai.PromptTokensDetails{
						CachedTokens: int(n),
					}
				}
			}
			switch delta := event.Delta.(type) {
			case anthropic.BetaRawMessageDeltaEventDelta:
				usage.CompletionTokens += int(event.Usage.OutputTokens)
			case anthropic.BetaRawContentBlockDeltaEventDelta:
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
			return
		} else {
			c.Stream <- openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
			}
		}
		if so := req.StreamOptions; so != nil && so.IncludeUsage {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			c.Stream <- openai.ChatCompletionStreamResponse{Usage: &usage}
		}
	}()
	return
}

func (a *Anthropic) BatchUpload(ctx context.Context, b *openai.Batch, inputs []openai.BatchInput) error {
	return simp.ErrBatchDeferred
}

func (a *Anthropic) BatchSend(ctx context.Context, b *openai.Batch) error {
	inputs, ok := ctx.Value(simp.KeyBatchInputs).([]openai.BatchInput)
	if !ok {
		return fmt.Errorf("inputs are unknown at send time")
	}
	reqs := make([]anthropic.BetaMessageBatchNewParamsRequest, len(inputs))
	for i, input := range inputs {
		if input.ChatCompletion == nil {
			return fmt.Errorf("input/%d: chat completion is nil", i)
		}

		params, err := a.translate(ctx, *input.ChatCompletion)
		if err != nil {
			return fmt.Errorf("input/%d: %w", i, err)
		}
		reqs[i].Params = anthropic.F(anthropic.BetaMessageBatchNewParamsRequestsParams{
			Model:       params.Model,
			Messages:    params.Messages,
			MaxTokens:   params.MaxTokens,
			Temperature: params.Temperature,
			TopP:        params.TopP,
		})
		reqs[i].CustomID = anthropic.F(input.CustomID)
	}
	batch, err := a.Beta.Messages.Batches.New(ctx, anthropic.BetaMessageBatchNewParams{
		Requests: anthropic.F(reqs),
	})
	if err != nil {
		return err
	}
	b.Metadata["job"] = batch.ID
	b.Metadata["state"] = batch.ProcessingStatus
	b.Status = a.batchStatus(batch.ProcessingStatus)
	return nil
}

func (a *Anthropic) BatchRefresh(ctx context.Context, b *openai.Batch) error {
	job, ok := b.Metadata["job"].(string)
	if !ok {
		return fmt.Errorf("job is unknown")
	}
	batch, err := a.Beta.Messages.Batches.Get(
		ctx,
		job,
		anthropic.BetaMessageBatchGetParams{})
	if err != nil {
		return err
	}
	b.Metadata["state"] = batch.ProcessingStatus
	b.Metadata["results"] = batch.ResultsURL
	b.Status = a.batchStatus(batch.ProcessingStatus)
	return nil
}

func (a *Anthropic) BatchReceive(ctx context.Context, b *openai.Batch) (outputs []openai.BatchOutput, ret error) {
	job, ok := b.Metadata["job"].(string)
	if !ok {
		return nil, fmt.Errorf("job is unknown")
	}
	resp, err := a.Beta.Messages.Batches.Results(
		ctx,
		job,
		anthropic.BetaMessageBatchResultsParams{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r := json.NewDecoder(resp.Body)

	var indy anthropic.BetaMessageBatchIndividualResponse
	for {
		switch err := r.Decode(&indy); err {
		case nil:
			output := openai.BatchOutput{
				CustomID: indy.CustomID,
			}
			result := indy.Result
			switch msg, err := result.Message, result.Error.Error; {
			case err.Message != "":
				output.Error = &openai.APIError{
					Type:    string(err.Type),
					Message: err.Message,
				}
			default:
				cc := &openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Index: 0,
						Message: openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: msg.Content[0].Text,
						},
					}},
				}
				cc.Usage = openai.Usage{
					PromptTokens:     int(msg.Usage.InputTokens),
					CompletionTokens: int(msg.Usage.OutputTokens),
					TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
				}
				output.ChatCompletion = cc
			}
			outputs = append(outputs, output)
		case io.EOF:
			return outputs, nil
		default:
			return nil, fmt.Errorf("failed to decode result: %w", err)
		}
	}
}

func (a *Anthropic) BatchCancel(ctx context.Context, b *openai.Batch) error {
	job, ok := b.Metadata["job"].(string)
	if !ok {
		return fmt.Errorf("job is unknown")
	}
	batch, err := a.Beta.Messages.Batches.Cancel(
		ctx,
		job,
		anthropic.BetaMessageBatchCancelParams{})
	if err != nil {
		return fmt.Errorf("failed to cancel batch: %w", err)
	}
	b.Metadata["state"] = batch.ProcessingStatus
	b.Status = a.batchStatus(batch.ProcessingStatus)
	return nil
}

func (a *Anthropic) batchStatus(s anthropic.BetaMessageBatchProcessingStatus) openai.BatchStatus {
	switch s {
	case anthropic.BetaMessageBatchProcessingStatusInProgress:
		return openai.BatchStatusInProgress
	case anthropic.BetaMessageBatchProcessingStatusCanceling:
		return openai.BatchStatusCancelled
	case anthropic.BetaMessageBatchProcessingStatusEnded:
		return openai.BatchStatusCompleted
	}
	return openai.BatchStatusFailed
}
