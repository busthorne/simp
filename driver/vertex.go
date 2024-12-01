package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	aipl "cloud.google.com/go/aiplatform/apiv1"
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
	return &Vertex{p}, nil
}

// Vertex implements the driver interface using Google's Vertex AI API
type Vertex struct {
	config.Provider
}

func (v *Vertex) List(ctx context.Context) ([]openai.Model, error) {
	// Vertex AI doesn't have a direct model listing API
	// Return predefined list of available models
	return []openai.Model{
		{ID: "gemini-1.5-flash-001"},
		{ID: "gemini-1.5-flash-002"},
		{ID: "gemini-1.5-pro-001"},
		{ID: "gemini-1.5-pro-002"},
		{ID: "gemini-flash-experimental"},
		{ID: "gemini-pro-experimental"},
	}, nil
}

func (v *Vertex) Embed(ctx context.Context, req openai.EmbeddingRequest) (e openai.EmbeddingResponse, ret error) {
	var texts []string
	switch input := req.Input.(type) {
	case string:
		texts = []string{input}
	case []string:
		texts = input
	default:
		return e, simp.ErrUnsupportedInput
	}

	client, err := v.predictionClient(ctx)
	if err != nil {
		return e, err
	}
	defer client.Close()

	instances := make([]*structpb.Value, len(texts))
	for i, text := range texts {
		fields := map[string]*structpb.Value{
			"content": structpb.NewStringValue(text),
		}
		if req.Task != "" {
			fields["task_type"] = structpb.NewStringValue(req.Task)
		}
		instances[i] = structpb.NewStructValue(&structpb.Struct{Fields: fields})
	}
	params := structpb.NewStructValue(&structpb.Struct{
		Fields: map[string]*structpb.Value{
			"outputDimensionality": structpb.NewNumberValue(float64(req.Dimensions)),
		},
	})
	resp, err := client.Predict(ctx, &aipb.PredictRequest{
		Endpoint: fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
			v.Project,
			v.Region,
			req.Model,
		),
		Instances:  instances,
		Parameters: params,
	})
	if err != nil {
		return e, err
	}
	for i, prediction := range resp.Predictions {
		values := prediction.
			GetStructValue().
			Fields["embeddings"].
			GetStructValue().
			Fields["values"].
			GetListValue().
			Values
		vector := make([]float32, len(values))
		for j, value := range values {
			vector[j] = float32(value.GetNumberValue())
		}
		e.Data = append(e.Data, openai.Embedding{
			Index:     i,
			Embedding: vector,
		})
	}
	return
}

func (v *Vertex) Complete(ctx context.Context, req openai.CompletionRequest) (c openai.CompletionResponse, err error) {
	return c, simp.ErrNotImplemented
}

func (v *Vertex) Chat(ctx context.Context, req openai.ChatCompletionRequest) (c openai.ChatCompletionResponse, err error) {
	client, err := v.genaiClient(ctx)
	if err != nil {
		return c, err
	}
	defer client.Close()

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

func (v *Vertex) BatchUpload(ctx context.Context, batch *openai.Batch, inputs []openai.BatchInput) error {
	if !v.Batch {
		return simp.ErrNotImplemented
	}

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
	for _, i := range inputs {
		if i.ChatCompletion == nil {
			return fmt.Errorf("embeddings are not supported")
		}
		models[i.ChatCompletion.Model] = true
		if len(models) > 1 {
			return fmt.Errorf("all completions must use the same model")
		}
		rows = append(rows, vertexBatch{
			ID:      i.CustomID,
			Request: v.serializeRequest(i.ChatCompletion),
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

func (v *Vertex) BatchSend(ctx context.Context, batch *openai.Batch) error {
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	var params *structpb.Value
	if p, ok := batch.Metadata["model_parameters"]; ok {
		params, err = structpb.NewValue(p)
		if err != nil {
			return fmt.Errorf("cannot marshal model parameters: %w", err)
		}
	}

	ifd := batch.InputFileID
	input := fmt.Sprintf("bq://%s.%s.%s",
		v.Project,
		v.Dataset,
		ifd,
	)
	output := fmt.Sprintf("bq://%s.%s.%s",
		v.Project,
		v.Dataset,
		"predict-"+ifd,
	)
	req := aipb.CreateBatchPredictionJobRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", v.Project, v.Region),
		BatchPredictionJob: &aipb.BatchPredictionJob{
			DisplayName:     ifd,
			ModelParameters: params,
			Model:           "publishers/google/models/" + batch.Metadata["model"].(string),
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
	job, err := client.CreateBatchPredictionJob(ctx, &req)
	if err != nil {
		return fmt.Errorf("cannot create job: %w", err)
	}
	batch.Metadata["table"] = input
	batch.Metadata["job"] = job.GetName()
	batch.Metadata["state"] = job.GetState()
	v.updateStatus(batch, job.GetState())
	return nil
}

func (v *Vertex) BatchRefresh(ctx context.Context, batch *openai.Batch) error {
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	req := aipb.GetBatchPredictionJobRequest{
		Name: batch.Metadata["job"].(string),
	}
	job, err := client.GetBatchPredictionJob(ctx, &req)
	if err != nil {
		return fmt.Errorf("cannot get job: %w", err)
	}
	batch.Metadata["state"] = job.GetState()
	batch.Metadata["dest"] = job.GetOutputInfo().GetGcsOutputDirectory()
	v.updateStatus(batch, job.GetState())
	return nil
}

func (v *Vertex) BatchReceive(ctx context.Context, batch *openai.Batch) (outputs []openai.BatchOutput, err error) {
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
			return outputs, nil
		default:
			return nil, fmt.Errorf("cannot read from table: %w", err)
		}
		var resp Response
		if err := json.Unmarshal([]byte(row.Response), &resp); err != nil {
			return nil, fmt.Errorf("cannot unmarshal response/%s: %w", row.ID, err)
		}
		output := &openai.ChatCompletionResponse{ID: row.ID}
		for _, can := range resp.Candidates {
			c := openai.ChatCompletionChoice{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: can.Content.Parts[0].Text,
				},
			}
			output.Choices = append(output.Choices, c)
		}
		output.Usage = openai.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
		outputs = append(outputs, openai.BatchOutput{
			CustomID:       row.ID,
			ChatCompletion: output,
		})
	}
}

func (v *Vertex) BatchCancel(ctx context.Context, batch *openai.Batch) error {
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	req := &aipb.CancelBatchPredictionJobRequest{
		Name: batch.Metadata["job"].(string),
	}
	return client.CancelBatchPredictionJob(ctx, req)
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

func (v *Vertex) updateStatus(batch *openai.Batch, state aipb.JobState) {
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

func (v *Vertex) genaiClient(ctx context.Context) (*genai.Client, error) {
	client, err := genai.NewClient(ctx,
		v.Project,
		v.Region,
		option.WithCredentialsJSON([]byte(v.APIKey)))
	if err != nil {
		return nil, fmt.Errorf("cannot make client: %w", err)
	}
	return client, nil
}

func (v *Vertex) predictionClient(ctx context.Context) (*aipl.PredictionClient, error) {
	client, err := aipl.NewPredictionClient(ctx,
		option.WithEndpoint(v.Region+"-aiplatform.googleapis.com:443"),
		v.credentials())
	if err != nil {
		return nil, fmt.Errorf("cannot make client: %w", err)
	}
	return client, nil
}

func (v *Vertex) jobClient(ctx context.Context) (*aipl.JobClient, error) {
	client, err := aipl.NewJobClient(ctx,
		option.WithEndpoint(v.Region+"-aiplatform.googleapis.com:443"),
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
