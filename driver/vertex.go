package driver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	aipl "cloud.google.com/go/aiplatform/apiv1"
	aipb "cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
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
	return &Vertex{Provider: p, uploads: map[string]string{}}, nil
}

// Vertex implements the driver interface using Google's Vertex AI API
type Vertex struct {
	config.Provider

	storage *storage.Client
	uploads map[string]string
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
	for _, s := range req.Input {
		if s.Text == "" {
			return e, simp.ErrUnsupportedInput
		}
		texts = append(texts, s.Text)
	}

	client, err := v.predictionClient(ctx)
	if err != nil {
		return e, err
	}
	defer client.Close()

	instances := make([]*structpb.Value, len(texts))
	for i, text := range texts {
		p := map[string]any{"content": text}
		switch {
		case req.Task != "":
			p["task_type"] = req.Task
		case req.LateChunking:
			p["task_type"] = "RETRIEVAL_DOCUMENT"
		}
		v, err := structpb.NewValue(p)
		if err != nil {
			return e, fmt.Errorf("cannot marshal instance/%d: %w", i, err)
		}
		instances[i] = v
	}
	p := map[string]any{}
	if v := req.Dimensions; v != 0 {
		p["outputDimensionality"] = v
	}
	params, err := structpb.NewValue(p)
	if err != nil {
		return e, fmt.Errorf("cannot marshal model parameters: %w", err)
	}
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

	model := client.GenerativeModel(req.Model)

	if v, ok := req.Metadata["cache_ttl"]; ok {
		t, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return c, fmt.Errorf("invalid cache ttl: %w", err)
		}
		cc, err := client.CreateCachedContent(ctx, &genai.CachedContent{
			Expiration:        genai.ExpireTimeOrTTL{TTL: time.Duration(t) * time.Second},
			SystemInstruction: &genai.Content{},
			Contents:          []*genai.Contents,
		})
	}

	model.SetTemperature(req.Temperature)
	if scm := req.ResponseFormat.JSONSchema; scm != nil {
		return c, fmt.Errorf("json schema for vertex is hell")
	}
	if v := req.MaxTokens; v != 0 {
		model.SetMaxOutputTokens(int32(v)) //nolint:gosec
	}
	if v := req.TopP; v != 0 {
		model.TopP = &v
	}
	if v := req.FrequencyPenalty; v != 0 {
		model.FrequencyPenalty = &v
	}
	if v := req.PresencePenalty; v != 0 {
		model.PresencePenalty = &v
	}
	cs := model.StartChat()
	h := []*genai.Content{}
	v.marshal(ctx, &req)
	for i, msg := range req.Messages {
		role := msg.Role
		switch role {
		case "system":
			if i != 0 {
				return c, simp.ErrMisplacedSystem
			}
			model.SystemInstruction = &genai.Content{
				Parts: []genai.Part{genai.Text(msg.Content)},
			}
		case "user":
		case "assistant":
			role = "model"
		default:
			return c, simp.ErrUnsupportedRole
		}
		if role == "system" {
			continue
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
		defer client.Close()
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
		defer client.Close()
		defer close(c.Stream)

		var usage *openai.Usage
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

				if u := chunk.UsageMetadata; u != nil && u.TotalTokenCount > 0 {
					usage = &openai.Usage{
						PromptTokens:     int(u.PromptTokenCount),
						CompletionTokens: int(u.CandidatesTokenCount),
						TotalTokens:      int(u.TotalTokenCount),
					}
				}
			case iterator.Done:
				c.Stream <- openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
				}
				if so := req.StreamOptions; so != nil && so.IncludeUsage && usage != nil {
					c.Stream <- openai.ChatCompletionStreamResponse{Usage: usage}
				}
				return
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
	for i, input := range inputs {
		if input.ChatCompletion == nil {
			return fmt.Errorf("embeddings are not supported")
		}
		models[input.ChatCompletion.Model] = true
		if len(models) > 1 {
			return fmt.Errorf("all completions must use the same model")
		}
		req, err := v.marshal(ctx, input.ChatCompletion)
		if err != nil {
			return fmt.Errorf("message/%d could not be marshaled: %w", i, err)
		}
		rows = append(rows, vertexBatch{
			ID:      input.CustomID,
			Request: req,
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
	inserter := client.
		Dataset(v.Dataset).
		Table(table).
		Inserter()

	const chunkSize = 1000
	for input := 0; input < len(rows); input += chunkSize {
		end := input + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[input:end]

		if err := inserter.Put(ctx, chunk); err != nil {
			return fmt.Errorf("failed to insert batch chunk: %w", err)
		}
	}
	batch.InputFileID = table
	return nil
}

func (v *Vertex) BatchSend(ctx context.Context, batch *openai.Batch) error {
	m, ok := ctx.Value(simp.KeyModel).(config.Model)
	if !ok {
		return fmt.Errorf("model not found")
	}
	client, err := v.jobClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	p := map[string]any{}
	if v := m.MaxTokens; v != 0 {
		p["maxOutputTokens"] = v
	}
	if v := m.Temperature; v != nil {
		p["temperature"] = *v
	}
	if v := m.TopP; v != nil {
		p["topP"] = *v
	}
	if v := m.FrequencyPenalty; v != nil {
		p["frequencyPenalty"] = *v
	}
	if v := m.PresencePenalty; v != nil {
		p["presencePenalty"] = *v
	}
	if v := m.Seed; v != nil {
		p["seed"] = *v
	}
	if v := m.Stop; len(v) > 0 {
		p["stopSequences"] = v
	}
	params, err := structpb.NewValue(p)
	if err != nil {
		return fmt.Errorf("cannot marshal model parameters: %w", err)
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
			Model:           "publishers/google/models/" + m.Name,
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

	jobName, ok := batch.Metadata["job"].(string)
	if !ok {
		return fmt.Errorf("job name not available in metadata: %v", batch.Metadata)
	}
	req := aipb.GetBatchPredictionJobRequest{Name: jobName}
	job, err := client.GetBatchPredictionJob(ctx, &req)
	if err != nil {
		return fmt.Errorf("cannot get job: %w", err)
	}
	batch.Metadata["state"] = job.GetState()
	batch.OutputFileID = job.GetOutputInfo().GetGcsOutputDirectory()
	v.updateStatus(batch, job.GetState())
	return nil
}

// VertexP is a message content part.
type VertexP struct {
	Text string   `json:"text,omitempty"`
	File *VertexF `json:"fileData,omitempty"`
}

type VertexF struct {
	FileUri  string `json:"fileUri,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// VertexC is a message content.
type VertexC struct {
	Role  string    `json:"role,omitempty"`
	Parts []VertexP `json:"parts"`
}

func (v *Vertex) BatchReceive(ctx context.Context, batch *openai.Batch) (outputs []openai.BatchOutput, err error) {
	client, err := v.bigqueryClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if batch.Status == openai.BatchStatusCompleted {
		// NOTE: predict-UUID will contain the requests for posterity
		defer client.
			Dataset(v.Dataset).
			Table(batch.InputFileID).
			Delete(ctx)
	}

	type Candidates struct {
		AvgLogprobs  float64 `json:"avgLogprobs"`
		Content      VertexC `json:"content"`
		FinishReason string  `json:"finishReason"`
	}
	type Usage struct {
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		PromptTokenCount     int `json:"promptTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	}
	type Response struct {
		Candidates    []Candidates `json:"candidates"`
		ModelVersion  string       `json:"modelVersion"`
		UsageMetadata Usage        `json:"usageMetadata"`
	}
	table := "predict-" + batch.InputFileID
	it := client.
		Dataset(v.Dataset).
		Table(table).
		Read(ctx)
	for {
		var row struct {
			ID       string `bigquery:"custom_id"`
			Response string `bigquery:"response"`
		}
		err := it.Next(&row)
		switch err {
		case nil:
			if row.Response == "" {
				continue
			}
		case iterator.Done:
			return outputs, nil
		default:
			return nil, fmt.Errorf("cannot read from table %q: %w", table, err)
		}
		var resp Response
		if err := json.Unmarshal([]byte(row.Response), &resp); err != nil {
			return nil, fmt.Errorf("cannot unmarshal response/%s: %w", row.ID, err)
		}
		output := &openai.ChatCompletionResponse{ID: row.ID}
		for _, can := range resp.Candidates {
			s := ""
			if parts := can.Content.Parts; len(parts) != 0 {
				s = parts[0].Text
			}
			c := openai.ChatCompletionChoice{
				Message: openai.ChatCompletionMessage{
					Role:    "assistant",
					Content: s,
				},
				LogProbs: &openai.LogProbs{
					Content: []openai.LogProb{{Token: "average", LogProb: can.AvgLogprobs}},
				},
				FinishReason: openai.FinishReason(can.FinishReason),
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
	jobName, ok := batch.Metadata["job"].(string)
	if !ok {
		return fmt.Errorf("job name not available in metadata: %v", batch.Metadata)
	}
	req := &aipb.CancelBatchPredictionJobRequest{Name: jobName}
	return client.CancelBatchPredictionJob(ctx, req)
}

func (v *Vertex) marshalChat(ctx context.Context, a *openai.ChatCompletionRequest) (string, error) {
	req := map[string]any{}
	contents := make([]VertexC, 0, len(a.Messages))
	for i, msg := range a.Messages {
		var (
			c    VertexC
			role = msg.Role
			mp   = msg.MultiContent
		)
		if len(mp) == 0 {
			if msg.Content == "" {
				return "", fmt.Errorf("empty message/%d", i)
			}
			mp = append(mp, openai.ChatMessagePart{Type: "text", Text: msg.Content})
		}
		for j, p := range mp {
			switch p.Type {
			case openai.ChatMessagePartTypeText:
				c.Parts = append(c.Parts, VertexP{Text: p.Text})
			case openai.ChatMessagePartTypeImageURL:
				if p.ImageURL == nil || p.ImageURL.URL == "" {
					return "", fmt.Errorf("no fileUri present in message/%d part/%d", i, j)
				}
				gs, mime, err := v.fileUpload(ctx, p.ImageURL.URL)
				if err != nil {
					return "", fmt.Errorf("upload message/%d part/%d: %w", i, j, err)
				}
				c.Parts = append(c.Parts, VertexP{File: &VertexF{FileUri: gs, MimeType: mime}})
			default:
				return "", fmt.Errorf("message/%d part/%d: %w", i, j, simp.ErrUnsupportedInput)
			}
		}
		switch role {
		case "system":
			if i != 0 {
				return "", fmt.Errorf("message/%d: %w", i, simp.ErrMisplacedSystem)
			}
			req["system_instruction"] = c
			continue
		case "user":
		case "assistant":
			role = "model"
		default:
			return "", fmt.Errorf("message/%d: %w", i, simp.ErrUnsupportedRole)
		}
		c.Role = role
		contents = append(contents, c)
	}
	req["contents"] = contents

	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (v *Vertex) fileUpload(ctx context.Context, fileUri string) (gs, mime string, ret error) {
	var ext string
	switch ext = strings.ToLower(filepath.Ext(fileUri)); ext {
	case ".pdf":
		mime = "application/pdf"
	case ".mp3", ".wav", ".mpeg":
		mime = "audio/" + ext
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".webp", ".png":
		mime = "image/" + ext
	case ".mov", ".mp4", ".mpg", ".avi", ".wmv", ".mpegps", ".flv":
		mime = "video/" + ext
	default:
		mime = "text/plain"
	}
	if strings.HasPrefix(fileUri, "gs://") {
		return fileUri, mime, nil
	}
	if s, ok := v.uploads[fileUri]; ok {
		return s, mime, ret
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileUri, nil)
	if err != nil {
		return gs, mime, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return gs, mime, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return gs, mime, err
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mime = ct
	}

	h := sha256.New()
	h.Write(b)
	digest := hex.EncodeToString(h.Sum(nil))

	client, err := v.storageClient(ctx)
	if err != nil {
		return gs, mime, err
	}

	if ext != "" {
		ext = "." + ext
	}
	obj := client.Bucket(v.Bucket).Object(digest + ext)
	switch _, err := obj.Attrs(ctx); err {
	case storage.ErrObjectNotExist:
		writer := obj.NewWriter(ctx)
		if _, err := writer.Write(b); err != nil {
			return gs, mime, err
		}
		if err := writer.Close(); err != nil {
			return gs, mime, err
		}
	default:
		return gs, mime, err
	}
	gs = "gs://" + v.Bucket + "/" + obj.ObjectName()
	v.uploads[fileUri] = gs
	return gs, mime, nil
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

func (v *Vertex) storageClient(ctx context.Context) (*storage.Client, error) {
	if v.storage != nil {
		return v.storage, nil
	}
	client, err := storage.NewClient(ctx, v.credentials())
	if err != nil {
		return nil, fmt.Errorf("cannot make storage client: %w", err)
	}
	v.storage = client
	return client, nil
}

func (v *Vertex) credentials() option.ClientOption {
	return option.WithCredentialsJSON([]byte(v.APIKey))
}
