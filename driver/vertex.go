package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"cloud.google.com/go/storage"
	"cloud.google.com/go/vertexai/genai"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
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
					Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
				}
			default:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "error"}},
					Error:   err,
				}
			}
		}
	}()
	return c, nil
}

func (v *Vertex) BatchUpload(ctx context.Context, batch *simp.Batch, mag simp.Magazine) error {
	if len(mag) == 0 {
		return simp.ErrNotFound
	}
	if v.p.Bucket == "" {
		return fmt.Errorf("bucket configuration required for batch operations")
	}

	b := bytes.Buffer{}
	w := json.NewEncoder(&b)
	for i, u := range mag {
		if u.Cin == nil {
			return fmt.Errorf("magazine/%d: embeddings are %w", i, simp.ErrNotImplemented)
		}
		req := v.serializeRequest(u.Cin)
		if err := w.Encode(map[string]any{"request": &req}); err != nil {
			return fmt.Errorf("magazine/%d: %w", i, err)
		}
	}
	store, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(v.p.APIKey)))
	if err != nil {
		return fmt.Errorf("storage client: %w", err)
	}
	defer store.Close()
	batch.ID = uuid.New().String()
	fname := fmt.Sprintf("%s.jsonl", batch.ID)
	ww := store.Bucket(v.p.Bucket).Object(fname).NewWriter(ctx)
	if _, err := ww.Write(b.Bytes()); err != nil {
		return fmt.Errorf("cloud upload error: %w", err)
	}
	if err := ww.Close(); err != nil {
		return fmt.Errorf("unexpected cloud upload error: %w", err)
	}
	batch.InputFileID = fname
	batch.Metadata = map[string]any{
		"model": fmt.Sprintf("publishers/google/models/%s", mag[0].Cin.Model),
	}
	return nil
}

func (v *Vertex) BatchSend(ctx context.Context, batch *simp.Batch) error {
	endpoint := fmt.Sprintf("%s-aiplatform.googleapis.com:443", v.p.Region)
	client, err := aiplatform.NewJobClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		return fmt.Errorf("create job client: %w", err)
	}
	defer client.Close()

	params, _ := structpb.NewValue(map[string]interface{}{
		// "temperature":     0.2,
		// "maxOutputTokens": 200,
	})
	bucketUri := "gs://" + v.p.Bucket
	req := &aiplatformpb.CreateBatchPredictionJobRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", v.p.Project, v.p.Region),
		BatchPredictionJob: &aiplatformpb.BatchPredictionJob{
			DisplayName:     batch.ID,
			Model:           fmt.Sprintf("publishers/google/models/%s", batch.Metadata["model"]),
			ModelParameters: params,
			InputConfig: &aiplatformpb.BatchPredictionJob_InputConfig{
				Source: &aiplatformpb.BatchPredictionJob_InputConfig_GcsSource{
					GcsSource: &aiplatformpb.GcsSource{
						Uris: []string{bucketUri + "/" + batch.InputFileID},
					},
				},
				InstancesFormat: "jsonl",
			},
			OutputConfig: &aiplatformpb.BatchPredictionJob_OutputConfig{
				Destination: &aiplatformpb.BatchPredictionJob_OutputConfig_GcsDestination{
					GcsDestination: &aiplatformpb.GcsDestination{
						OutputUriPrefix: bucketUri + "/result-",
					},
				},
				PredictionsFormat: "jsonl",
			},
		},
	}

	job, err := client.CreateBatchPredictionJob(ctx, req)
	if err != nil {
		return fmt.Errorf("create batch job: %w", err)
	}
	batch.Metadata["job"] = job.GetName()
	v.updateStatus(batch, job.GetState())
	return nil
}

func (v *Vertex) updateStatus(batch *simp.Batch, state aiplatformpb.JobState) {
	switch state {
	case aiplatformpb.JobState_JOB_STATE_SUCCEEDED, aiplatformpb.JobState_JOB_STATE_PARTIALLY_SUCCEEDED:
		batch.Status = openai.BatchStatusCompleted
	case aiplatformpb.JobState_JOB_STATE_CANCELLED, aiplatformpb.JobState_JOB_STATE_CANCELLING:
		batch.Status = openai.BatchStatusCancelled
	case aiplatformpb.JobState_JOB_STATE_EXPIRED:
		batch.Status = openai.BatchStatusExpired
	case
		aiplatformpb.JobState_JOB_STATE_UPDATING,
		aiplatformpb.JobState_JOB_STATE_PENDING,
		aiplatformpb.JobState_JOB_STATE_QUEUED,
		aiplatformpb.JobState_JOB_STATE_PAUSED,
		aiplatformpb.JobState_JOB_STATE_RUNNING:
		// pending
		batch.Status = openai.BatchStatusInProgress
	default:
		batch.Status = openai.BatchStatusFailed
	}
}

func (v *Vertex) BatchRefresh(ctx context.Context, batch *simp.Batch) error {
	return simp.ErrNotImplemented
}

func (v *Vertex) BatchReceive(ctx context.Context, batch *simp.Batch) (mag simp.Magazine, err error) {
	return nil, simp.ErrNotImplemented
}

func (v *Vertex) BatchCancel(ctx context.Context, batch *simp.Batch) error {
	return simp.ErrNotImplemented
}

func (v *Vertex) serializeRequest(a *openai.ChatCompletionRequest) (b aiplatformpb.GenerateContentRequest) {
	contents := make([]*aiplatformpb.Content, 0, len(a.Messages))
	for _, msg := range a.Messages {
		if msg.Role == "system" {
			b.SystemInstruction = &aiplatformpb.Content{
				Parts: []*aiplatformpb.Part{{
					Data: &aiplatformpb.Part_Text{
						Text: msg.Content,
					},
				}},
			}
		}
		content := &aiplatformpb.Content{
			Role: msg.Role,
			Parts: []*aiplatformpb.Part{{
				Data: &aiplatformpb.Part_Text{
					Text: msg.Content,
				},
			}},
		}
		contents = append(contents, content)
	}
	b.Contents = contents
	return
}
