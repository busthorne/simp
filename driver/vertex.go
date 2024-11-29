package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	aipb "cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/vertexai/genai"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
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
	if p.Dataset == "" {
		p.Dataset = "simpbatches"
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
	client, err := v.bigqueryClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	type vertexBatch struct {
		ID      string `bigquery:"custom_id"`
		Request string `bigquery:"request"`
	}
	var (
		table  = batch.ID
		rows   = []vertexBatch{}
		models = map[string]bool{}
	)
	for i, u := range mag {
		if u.Cin == nil {
			return fmt.Errorf("magazine/%d: embeddings are %w", i, simp.ErrNotImplemented)
		}
		models[u.Cin.Model] = true
		if len(models) > 1 {
			return fmt.Errorf("all completions must use the same model")
		}
		rows = append(rows, vertexBatch{
			ID:      u.Id,
			Request: v.serializeRequest(u.Cin),
		})
	}
	err = client.
		Dataset(v.Dataset).
		Table(table).
		Create(ctx, &bigquery.TableMetadata{
			Name: table,
			Schema: bigquery.Schema{
				{Name: "custom_id", Type: bigquery.StringFieldType},
				{Name: "request", Type: bigquery.StringFieldType},
			},
		})
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	err = client.
		Dataset(v.Dataset).
		Table(table).
		Inserter().
		Put(ctx, rows)
	if err != nil {
		return fmt.Errorf("failed to insert batch: %w", err)
	}
	for model := range models {
		batch.Metadata["model"] = model
	}
	batch.InputFileID = table
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

	fid := batch.InputFileID
	input := fmt.Sprintf("bq://%s.%s.%s", v.Project, v.Dataset, fid)
	output := fmt.Sprintf("bq://%s.%s.%s", v.Project, v.Dataset, "predict-"+fid)
	req := &aipb.CreateBatchPredictionJobRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", v.Project, v.Region),
		BatchPredictionJob: &aipb.BatchPredictionJob{
			DisplayName:     fid,
			Model:           "publishers/google/models/" + batch.Metadata["model"].(string),
			ModelParameters: params,
			InputConfig: &aipb.BatchPredictionJob_InputConfig{
				Source: &aipb.BatchPredictionJob_InputConfig_BigquerySource{
					BigquerySource: &aipb.BigQuerySource{InputUri: input},
				},
				InstancesFormat: "bigquery",
			},
			OutputConfig: &aipb.BatchPredictionJob_OutputConfig{
				Destination: &aipb.BatchPredictionJob_OutputConfig_BigqueryDestination{
					BigqueryDestination: &aipb.BigQueryDestination{OutputUri: output},
				},
				PredictionsFormat: "bigquery",
			},
		},
	}
	job, err := client.CreateBatchPredictionJob(ctx, req)
	if err != nil {
		return fmt.Errorf("cannot create job: %w", err)
	}
	batch.Metadata["table"] = input
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
	client, err := v.bigqueryClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	type Row struct {
		ID       string `bigquery:"custom_id"`
		Response string `bigquery:"response"`
	}
	type Parts struct {
		Text     string `json:"text,omitempty"`
		FileUri  string `json:"fileUri,omitempty"`
		MimeType string `json:"mimeType,omitempty"`
	}
	type Content struct {
		Role  string  `json:"role"`
		Parts []Parts `json:"parts"`
	}
	type Candidates struct {
		AvgLogprobs  float64 `json:"avgLogprobs"`
		Content      Content `json:"content"`
		FinishReason string  `json:"finishReason"`
	}
	type UsageMetadata struct {
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		PromptTokenCount     int `json:"promptTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	}
	type Response struct {
		Candidates    []Candidates  `json:"candidates"`
		ModelVersion  string        `json:"modelVersion"`
		UsageMetadata UsageMetadata `json:"usageMetadata"`
	}
	it := client.
		Dataset(v.Dataset).
		Table("predict-" + batch.InputFileID).
		Read(ctx)
	for {
		var row Row
		err := it.Next(&row)
		switch err {
		case nil:
			if row.Response == "" {
				continue
			}
		case iterator.Done:
			return mag, nil
		default:
			return nil, fmt.Errorf("cannot read from table: %w", err)
		}

		var resp Response
		if err := json.Unmarshal([]byte(row.Response), &resp); err != nil {
			return nil, fmt.Errorf("cannot unmarshal response/%s: %w", row.ID, err)
		}

		bullet := openai.ChatCompletionResponse{ID: row.ID}
		for _, can := range resp.Candidates {
			c := openai.ChatCompletionChoice{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: can.Content.Parts[0].Text,
				},
			}
			bullet.Choices = append(bullet.Choices, c)
		}
		mag = append(mag, simp.BatchUnion{Cout: &bullet})
	}
}

func (v *Vertex) BatchCancel(ctx context.Context, batch *simp.Batch) error {
	return simp.ErrNotImplemented
}

func (v *Vertex) serializeRequest(a *openai.ChatCompletionRequest) string {
	type textPart struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string     `json:"role,omitempty"`
		Parts []textPart `json:"parts"`
	}

	req := map[string]any{}
	contents := make([]content, 0, len(a.Messages))
	for _, msg := range a.Messages {
		role := msg.Role
		switch role {
		case "system":
			req["system_instruction"] = content{
				Parts: []textPart{{Text: msg.Content}},
			}
			continue
		case "user":
		case "assistant":
			role = "model"
		}
		content := content{
			Role:  role,
			Parts: []textPart{{Text: msg.Content}},
		}
		contents = append(contents, content)
	}
	req["contents"] = contents
	b, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	return string(b)
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
		v.credentials())
	if err != nil {
		return nil, fmt.Errorf("cannot make job client: %w", err)
	}
	return client, nil
}

func (v *Vertex) bigqueryClient(ctx context.Context) (*bigquery.Client, error) {
	client, err := bigquery.NewClient(ctx, v.Project, v.credentials())
	if err != nil {
		return nil, fmt.Errorf("cannot make bigquery client: %w", err)
	}
	return client, nil
}

func (v *Vertex) credentials() option.ClientOption {
	return option.WithCredentialsJSON([]byte(v.APIKey))
}
