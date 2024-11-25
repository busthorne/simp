package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

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

func (o *OpenAI) List(ctx context.Context) ([]simp.Model, error) {
	models, err := o.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	return models.Models, nil
}

func (o *OpenAI) Embed(ctx context.Context, req simp.Embed) (simp.Embeddings, error) {
	return o.CreateEmbeddings(ctx, req)
}

func (o *OpenAI) Complete(ctx context.Context, req simp.Complete) (simp.Completions, error) {
	return o.CreateChatCompletion(ctx, req)
}

func (o *OpenAI) BatchUpload(ctx context.Context, batch *simp.Batch, mag simp.Magazine) error {
	switch {
	case o.BaseURL != "" && !o.BatchAPI:
		return simp.ErrNotImplemented
	case len(mag) == 0:
		return simp.ErrNotFound
	}
	completing := mag[0].Cin != nil
	if completing {
		batch.Endpoint = chatCompletions
	} else {
		batch.Endpoint = embeddings
	}
	b := bytes.Buffer{}
	w := json.NewEncoder(&b)
	for i, u := range mag {
		var err error
		if completing {
			req := openai.BatchChatCompletionRequest{
				CustomID: u.Id,
				Body:     *u.Cin,
				Method:   "POST",
				URL:      chatCompletions,
			}
			err = w.Encode(req)
		} else {
			req := openai.BatchEmbeddingRequest{
				CustomID: u.Id,
				Body:     *u.Ein,
				Method:   "POST",
				URL:      embeddings,
			}
			err = w.Encode(req)
		}
		if err != nil {
			return fmt.Errorf("magazine/%d: %w", i, err)
		}
	}
	f, err := o.CreateFileBytes(ctx, openai.FileBytesRequest{
		Name:    "batch.jsonl",
		Bytes:   b.Bytes(),
		Purpose: openai.PurposeBatch,
	})
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	batch.InputFileID = f.ID
	return nil
}

func (o *OpenAI) BatchSend(ctx context.Context, batch *simp.Batch) error {
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
	*batch = b.Batch
	return nil
}

func (o *OpenAI) BatchRefresh(ctx context.Context, batch *simp.Batch) error {
	b, err := o.RetrieveBatch(ctx, batch.ID)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	*batch = b.Batch
	return nil
}

func (o *OpenAI) BatchReceive(ctx context.Context, batch *simp.Batch) (mag simp.Magazine, err error) {
	if batch.OutputFileID == nil {
		return nil, simp.ErrBatchIncomplete
	}
	f, err := o.GetFileContent(ctx, *batch.OutputFileID)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	r := json.NewDecoder(f)
	for i := 0; ; i++ {
		var u simp.BatchUnion
		var err error
		if batch.Endpoint == chatCompletions {
			err = r.Decode(&u.Cout)
		} else {
			err = r.Decode(&u.Eout)
		}
		switch err {
		case nil:
			mag = append(mag, u)
		case io.EOF:
			return mag, nil
		default:
			return nil, fmt.Errorf("magazine/%d: %w", i, err)
		}
	}
}

func (o *OpenAI) BatchCancel(ctx context.Context, batch *simp.Batch) error {
	b, err := o.CancelBatch(ctx, batch.ID)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	*batch = b.Batch
	return nil
}
