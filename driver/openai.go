package driver

import (
	"context"
	"fmt"

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
	return &OpenAI{*client, p}, nil
}

// OpenAI is the most basic kind of driver, because it's the API that we're emulating.
//
// Big think!
type OpenAI struct {
	openai.Client
	config.Provider
}

func (o *OpenAI) List(ctx context.Context) ([]openai.Model, error) {
	models, err := o.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	return models.Models, nil
}

func (o *OpenAI) Embed(ctx context.Context, req openai.EmbeddingRequest) (e openai.EmbeddingResponse, err error) {
	return o.CreateEmbeddings(ctx, req)
}

func (o *OpenAI) Complete(ctx context.Context, req openai.CompletionRequest) (c openai.CompletionResponse, err error) {
	return o.CreateCompletion(ctx, req)
}

func (o *OpenAI) Chat(ctx context.Context, req openai.ChatCompletionRequest) (c openai.ChatCompletionResponse, err error) {
	return o.CreateChatCompletion(ctx, req)
}

func (o *OpenAI) BatchUpload(ctx context.Context, batch *openai.Batch, inputs []openai.BatchInput) error {
	if o.BaseURL != "" && !o.BatchAPI {
		return simp.ErrNotImplemented
	}
	f, err := o.CreateFileBatch(ctx, inputs)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	batch.InputFileID = f.ID
	return nil
}

func (o *OpenAI) BatchSend(ctx context.Context, batch *openai.Batch) error {
	if batch.InputFileID == "" {
		panic("no input file id")
	}
	b, err := o.CreateBatch(ctx, openai.CreateBatchRequest{
		InputFileID:      batch.InputFileID,
		Endpoint:         batch.Endpoint,
		CompletionWindow: "24h",
	})
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	batch.Metadata["real_id"] = b.ID
	return nil
}

func (o *OpenAI) BatchRefresh(ctx context.Context, batch *openai.Batch) error {
	b, err := o.RetrieveBatch(ctx, batch.Metadata["real_id"].(string))
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	batch.OutputFileID = b.OutputFileID
	batch.Errors = b.Errors
	batch.Status = b.Status
	return nil
}

func (o *OpenAI) BatchReceive(ctx context.Context, batch *openai.Batch) (outputs []openai.BatchOutput, err error) {
	if batch.OutputFileID == "" {
		return nil, simp.ErrBatchIncomplete
	}
	return o.GetBatchContent(ctx, batch.OutputFileID)
}

func (o *OpenAI) BatchCancel(ctx context.Context, batch *openai.Batch) error {
	_, err := o.CancelBatch(ctx, batch.Metadata["real_id"].(string))
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	return nil
}
