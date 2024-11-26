package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	aipb "cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
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
	b, _ := base64.StdEncoding.DecodeString(p.APIKey)
	if len(b) > 0 {
		p.APIKey = string(b)
	}
	return &Vertex{p}, nil
}

// Vertex implements the driver interface using Google's Vertex AI API
type Vertex struct {
	config.Provider
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
		role := msg.Role
		switch role {
		case "system":
			if i == 0 {
				model.SystemInstruction = &genai.Content{
					Parts: []genai.Part{genai.Text(msg.Content)},
				}
				continue
			}
			return c, fmt.Errorf("system message is misplaced")
		case "user":
		case "assistant":
			role = "model"
		default:
			return c, fmt.Errorf("unsupported role: %q", role)
		}
		c := &genai.Content{
			Role:  role,
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
	if v.Bucket == "" {
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
	store, err := v.storageClient(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	batch.ID = uuid.New().String()
	fname := fmt.Sprintf("%s.jsonl", batch.ID)
	ww := store.Bucket(v.Bucket).Object(fname).NewWriter(ctx)
	if _, err := ww.Write(b.Bytes()); err != nil {
		return fmt.Errorf("cannot write to bucket: %w", err)
	}
	if err := ww.Close(); err != nil {
		return fmt.Errorf("unexpected upload error: %w", err)
	}
	batch.InputFileID = fname
	batch.Metadata = map[string]any{
		"model": fmt.Sprintf("publishers/google/models/%s", mag[0].Cin.Model),
	}
	return nil
}

func (v *Vertex) BatchSend(ctx context.Context, batch *simp.Batch) error {
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	params, _ := structpb.NewValue(map[string]interface{}{
		// "temperature":     0.2,
		// "maxOutputTokens": 200,
	})
	bucketUri := "gs://" + v.Bucket
	req := &aipb.CreateBatchPredictionJobRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", v.Project, v.Region),
		BatchPredictionJob: &aipb.BatchPredictionJob{
			DisplayName:     batch.ID,
			Model:           fmt.Sprintf("publishers/google/models/%s", batch.Metadata["model"]),
			ModelParameters: params,
			InputConfig: &aipb.BatchPredictionJob_InputConfig{
				Source: &aipb.BatchPredictionJob_InputConfig_GcsSource{
					GcsSource: &aipb.GcsSource{
						Uris: []string{bucketUri + "/" + batch.InputFileID},
					},
				},
				InstancesFormat: "jsonl",
			},
			OutputConfig: &aipb.BatchPredictionJob_OutputConfig{
				Destination: &aipb.BatchPredictionJob_OutputConfig_GcsDestination{
					GcsDestination: &aipb.GcsDestination{
						OutputUriPrefix: bucketUri,
					},
				},
				PredictionsFormat: "jsonl",
			},
		},
	}

	job, err := client.CreateBatchPredictionJob(ctx, req)
	if err != nil {
		return fmt.Errorf("cannot create job: %w", err)
	}
	batch.Metadata["job"] = job.GetName()
	batch.Metadata["state"] = job.GetState()
	v.updateStatus(batch, job.GetState())
	return nil
}

func (v *Vertex) BatchRefresh(ctx context.Context, batch *simp.Batch) error {
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	job, err := client.GetBatchPredictionJob(ctx, &aipb.GetBatchPredictionJobRequest{
		Name: batch.Metadata["job"].(string),
	})
	if err != nil {
		return fmt.Errorf("cannot get job: %w", err)
	}
	batch.Metadata["state"] = job.GetState()
	batch.Metadata["dest"] = job.GetOutputInfo().GetGcsOutputDirectory()
	v.updateStatus(batch, job.GetState())
	return nil
}

func (v *Vertex) BatchReceive(ctx context.Context, batch *simp.Batch) (mag simp.Magazine, err error) {
	store, err := v.storageClient(ctx)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	gs := batch.Metadata["dest"].(string)
	path := strings.TrimPrefix(gs, "gs://"+v.Bucket+"/") + "/predictions.jsonl"
	r, err := store.Bucket(v.Bucket).Object(path).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read from bucket: %w", err)
	}
	defer r.Close()

	jsonl := json.NewDecoder(r)
	for {
		var line struct {
			Pre  json.RawMessage `json:"request"`
			Post aipb.GenerateContentResponse
		}
		if err := jsonl.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
		}
	}

	return nil, nil
}

func (v *Vertex) BatchCancel(ctx context.Context, batch *simp.Batch) error {
	return simp.ErrNotImplemented
}

func (v *Vertex) serializeRequest(a *openai.ChatCompletionRequest) (b aipb.GenerateContentRequest) {
	contents := make([]*aipb.Content, 0, len(a.Messages))
	for _, msg := range a.Messages {
		if msg.Role == "system" {
			b.SystemInstruction = &aipb.Content{
				Parts: []*aipb.Part{{
					Data: &aipb.Part_Text{
						Text: msg.Content,
					},
				}},
			}
		}
		content := &aipb.Content{
			Role: msg.Role,
			Parts: []*aipb.Part{{
				Data: &aipb.Part_Text{
					Text: msg.Content,
				},
			}},
		}
		contents = append(contents, content)
	}
	b.Contents = contents
	return
}

func (v *Vertex) updateStatus(batch *simp.Batch, state aipb.JobState) {
	switch state {
	case aipb.JobState_JOB_STATE_SUCCEEDED, aipb.JobState_JOB_STATE_PARTIALLY_SUCCEEDED:
		batch.Status = openai.BatchStatusCompleted
	case aipb.JobState_JOB_STATE_CANCELLED, aipb.JobState_JOB_STATE_CANCELLING:
		batch.Status = openai.BatchStatusCancelled
	case aipb.JobState_JOB_STATE_EXPIRED:
		batch.Status = openai.BatchStatusExpired
	case
		aipb.JobState_JOB_STATE_UPDATING,
		aipb.JobState_JOB_STATE_PENDING,
		aipb.JobState_JOB_STATE_QUEUED,
		aipb.JobState_JOB_STATE_PAUSED,
		aipb.JobState_JOB_STATE_RUNNING:
		// pending
		batch.Status = openai.BatchStatusInProgress
	default:
		batch.Status = openai.BatchStatusFailed
	}
}

func (v *Vertex) client(ctx context.Context) (*genai.Client, error) {
	client, err := genai.NewClient(ctx,
		v.Project,
		v.Region,
		option.WithCredentialsJSON([]byte(v.APIKey)))
	if err != nil {
		return nil, fmt.Errorf("cannot make client: %w", err)
	}
	return client, nil
}

func (v *Vertex) jobClient(ctx context.Context) (*aiplatform.JobClient, error) {
	endpoint := fmt.Sprintf("%s-aiplatform.googleapis.com:443", v.Region)
	client, err := aiplatform.NewJobClient(ctx,
		option.WithEndpoint(endpoint),
		option.WithCredentialsJSON([]byte(v.APIKey)))
	if err != nil {
		return nil, fmt.Errorf("cannot make job client: %w", err)
	}
	return client, nil
}

func (v *Vertex) storageClient(ctx context.Context) (*storage.Client, error) {
	store, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(v.APIKey)))
	if err != nil {
		return nil, fmt.Errorf("cannot make storage client: %w", err)
	}
	return store, nil
}
