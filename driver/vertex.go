package driver

import (
	"bytes"
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
	"cloud.google.com/go/auth"
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genai"
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

func (v *Vertex) region(ctx context.Context) string {
	m, ok := ctx.Value(simp.KeyModel).(config.Model)
	if !ok || m.Region == "" {
		return v.Region
	}
	return m.Region
}

func (v *Vertex) List(ctx context.Context) ([]openai.Model, error) {
	client, err := v.genaiClient(ctx)
	if err != nil {
		return nil, err
	}
	models := []openai.Model{}

	for m, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, err
		}
		models = append(models, openai.Model{ID: m.Name})
	}
	return models, nil
}

func (v *Vertex) Embed(ctx context.Context, req openai.EmbeddingRequest) (e openai.EmbeddingResponse, ret error) {
	client, err := v.genaiClient(ctx)
	if err != nil {
		return e, err
	}

	var (
		contents = []*genai.Content{}
		config   = new(genai.EmbedContentConfig)
	)
	switch {
	case req.Task != "":
		config.TaskType = req.Task
	case req.LateChunking:
		config.TaskType = "RETRIEVAL_DOCUMENT"
	}

	for _, i := range req.Input {
		switch {
		case i.Text != "":
			contents = append(contents, genai.NewContentFromText(i.Text, genai.RoleUser))
		case i.Image != "":
			gs, mime, err := v.fileUpload(ctx, i.Image)
			if err != nil {
				return e, fmt.Errorf("cannot upload image: %w", err)
			}
			contents = append(contents, genai.NewContentFromURI(gs, mime, genai.RoleUser))
		}
	}
	if d := int32(req.Dimensions); d != 0 {
		config.OutputDimensionality = &d
	}
	resp, err := client.Models.EmbedContent(ctx, req.Model, contents, config)
	if err != nil {
		return e, fmt.Errorf("cannot embed content: %w", err)
	}
	spending := 0
	for i, m := range resp.Embeddings {
		e.Data = append(e.Data, openai.Embedding{
			Object:    "embedding",
			Index:     i,
			Embedding: m.Values,
		})
		spending += int(m.Statistics.TokenCount)
	}
	e.Usage = openai.Usage{
		CompletionTokens: spending,
		TotalTokens:      spending,
	}
	e.Object = "list"
	return e, nil
}

func (v *Vertex) Complete(ctx context.Context, req openai.CompletionRequest) (c openai.CompletionResponse, err error) {
	return c, simp.ErrNotImplemented
}

type vertexRequest struct {
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
}

func (v *Vertex) encode(ctx context.Context, req openai.ChatCompletionRequest) (*vertexRequest, error) {
	var (
		contents = []*genai.Content{}
		config   = &genai.GenerateContentConfig{}
	)
	config.CandidateCount = int32(req.N)
	for _, msg := range req.Messages {
		role := ""
		switch msg.Role {
		case "system":
			config.SystemInstruction = genai.NewContentFromText(msg.Content, genai.RoleUser)
			continue
		case "assistant":
			role = "model"
		case "user":
			role = "user"
		}
		if msg.Content != "" {
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.Role(role)))
		} else if len(msg.MultiContent) > 0 {
			for _, content := range msg.MultiContent {
				if content.ImageURL != nil && content.ImageURL.URL != "" {
					gs, mime, err := v.fileUpload(ctx, content.ImageURL.URL)
					if err != nil {
						return nil, fmt.Errorf("cannot upload image: %w", err)
					}
					contents = append(contents, genai.NewContentFromURI(gs, mime, genai.RoleUser))
				}
				if content.Text != "" {
					contents = append(contents, genai.NewContentFromText(content.Text, genai.RoleUser))
				}
			}
		}
	}

	config.Temperature = &req.Temperature
	config.TopP = &req.TopP
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.Stop) > 0 {
		config.StopSequences = req.Stop
	}
	config.PresencePenalty = &req.PresencePenalty
	config.FrequencyPenalty = &req.FrequencyPenalty

	if len(req.Tools) > 0 {
		var tools []*genai.Tool
		for _, t := range req.Tools {
			switch t.Type {
			case "function":
				pre, err := json.Marshal(t.Function.Parameters)
				if err != nil {
					return nil, fmt.Errorf("cannot marshal function parameters: %w", err)
				}
				post := &genai.Schema{}
				if err := json.Unmarshal(pre, post); err != nil {
					return nil, fmt.Errorf("cannot unmarshal function parameters: %w", err)
				}
				tools = append(tools, &genai.Tool{
					FunctionDeclarations: []*genai.FunctionDeclaration{
						{
							Name:        t.Function.Name,
							Description: t.Function.Description,
							Parameters:  post,
						},
					},
				})
			case "code_interpreter":
				tools = append(tools, &genai.Tool{CodeExecution: &genai.ToolCodeExecution{}})
			default:
				return nil, fmt.Errorf("unsupported tool type: %s", t.Type)
			}
		}
		if len(tools) > 0 {
			config.Tools = tools
		}
	}

	if cc, ok := req.Metadata["cached_content"]; ok {
		config.CachedContent = cc
	}

	return &vertexRequest{
		Contents: contents,
		Config:   config,
	}, nil
}

func (v *Vertex) decode(ret *genai.GenerateContentResponse) (openai.ChatCompletionResponse, error) {
	var resp = openai.ChatCompletionResponse{
		Object:  "chat.completion",
		Created: ret.CreateTime.Unix(),
		Model:   ret.ModelVersion,
	}

	for i, can := range ret.Candidates {
		if can.Content == nil || len(can.Content.Parts) == 0 {
			continue
		}
		var m = openai.ChatCompletionMessage{Role: "assistant"}

		var mp []openai.ChatMessagePart
		for _, part := range can.Content.Parts {
			var typ openai.ChatMessagePartType = "text"
			switch {
			case part.Text != "":
				if part.Thought {
					typ = "thought"
				}
				mp = append(mp, openai.ChatMessagePart{Type: typ, Text: part.Text})
			case part.ExecutableCode != nil:
				c := part.ExecutableCode
				t := fmt.Sprintf("```%s\n%s\n```", c.Language, c.Code)
				mp = append(mp, openai.ChatMessagePart{Type: typ, Text: t})
			case part.CodeExecutionResult != nil:
				cr := part.CodeExecutionResult
				if cr.Outcome == genai.OutcomeOK {
					typ = "code"
					mp = append(mp, openai.ChatMessagePart{Type: typ, Text: cr.Output})
				}
			case part.FunctionCall != nil:
				c := part.FunctionCall
				var b bytes.Buffer
				if err := json.NewEncoder(&b).Encode(c.Args); err != nil {
					return resp, fmt.Errorf("cannot marshal function arguments: %w", err)
				}
				m.ToolCalls = append(m.ToolCalls, openai.ToolCall{
					ID:       c.ID,
					Type:     "function",
					Function: openai.FunctionCall{Name: c.Name, Arguments: b.String()},
				})
			}
			if len(mp) == 1 {
				for _, p := range mp {
					if p.Type == "text" {
						m.Content = p.Text
						mp = nil
					}
				}
			}
			m.MultiContent = mp
		}

		finishReason := openai.FinishReasonStop
		switch can.FinishReason {
		case genai.FinishReasonMaxTokens:
			finishReason = openai.FinishReasonLength
		case genai.FinishReasonStop:
			finishReason = openai.FinishReasonStop
		case genai.FinishReasonSafety:
			finishReason = openai.FinishReasonContentFilter
		case genai.FinishReasonRecitation:
			finishReason = openai.FinishReasonContentFilter
		}

		resp.Choices = append(resp.Choices, openai.ChatCompletionChoice{
			Index:        i,
			Message:      m,
			FinishReason: finishReason,
		})
	}

	if meta := ret.UsageMetadata; meta != nil {
		resp.Usage = openai.Usage{
			PromptTokens:     int(meta.PromptTokenCount),
			CompletionTokens: int(meta.CandidatesTokenCount),
			TotalTokens:      int(meta.TotalTokenCount),
		}
		if c := meta.CachedContentTokenCount; c > 0 {
			resp.Usage.PromptTokensDetails = &openai.PromptTokensDetails{
				CachedTokens: int(c),
			}
		}
	}
	return resp, nil
}

func (v *Vertex) accumulate(a, b openai.Usage) openai.Usage {
	u := openai.Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
	}
	if d := a.PromptTokensDetails; d != nil {
		u.PromptTokensDetails = &openai.PromptTokensDetails{
			AudioTokens:  d.AudioTokens,
			CachedTokens: d.CachedTokens,
		}
	}
	if d := a.CompletionTokensDetails; d != nil {
		u.CompletionTokensDetails = &openai.CompletionTokensDetails{
			AudioTokens:     d.AudioTokens,
			ReasoningTokens: d.ReasoningTokens,
		}
	}
	if d := b.PromptTokensDetails; d != nil {
		d0 := u.PromptTokensDetails
		d0.AudioTokens += d.AudioTokens
		d0.CachedTokens += d.CachedTokens
	}
	if d := b.CompletionTokensDetails; d != nil {
		d0 := u.CompletionTokensDetails
		d0.AudioTokens += d.AudioTokens
		d0.ReasoningTokens += d.ReasoningTokens
	}
	return u
}

func (v *Vertex) Chat(ctx context.Context, req openai.ChatCompletionRequest) (c openai.ChatCompletionResponse, err error) {
	client, err := v.genaiClient(ctx)
	if err != nil {
		return c, err
	}
	p, err := v.encode(ctx, req)
	if err != nil {
		return c, err
	}
	if ttl, ok := req.Metadata["cache_ttl"]; ok {
		ttl, err := strconv.ParseInt(ttl, 10, 64)
		if err != nil {
			return c, fmt.Errorf("cannot parse cache ttl: %w", err)
		}
		cc, err := client.Caches.Create(ctx, v.googleModel(req.Model), &genai.CreateCachedContentConfig{
			TTL:               time.Duration(ttl) * time.Second,
			Contents:          p.Contents,
			SystemInstruction: p.Config.SystemInstruction,
			Tools:             p.Config.Tools,
			ToolConfig:        p.Config.ToolConfig,
		})
		if err != nil {
			return c, fmt.Errorf("cannot create cache: %w", err)
		}
		c.ID = cc.Name
		c.Usage = openai.Usage{
			TotalTokens: int(cc.UsageMetadata.TotalTokenCount),
		}
		return c, nil
	}
	if !req.Stream {
		resp, err := client.Models.GenerateContent(ctx, req.Model, p.Contents, p.Config)
		if err != nil {
			return c, fmt.Errorf("vertex GenerateContent failed: %w", err)
		}
		return v.decode(resp)
	}
	c.Stream = make(chan openai.ChatCompletionStreamResponse, 1)
	go func() {
		defer close(c.Stream)

		var total openai.Usage
		for chunk, err := range client.Models.GenerateContentStream(ctx, req.Model, p.Contents, p.Config) {
			if err != nil {
				c.Stream <- openai.ChatCompletionStreamResponse{Error: err}
				return
			}
			resp, err := v.decode(chunk)
			if err != nil {
				c.Stream <- openai.ChatCompletionStreamResponse{Error: err}
				return
			}
			total = v.accumulate(total, resp.Usage)

			var choices []openai.ChatCompletionStreamChoice
			for i, c := range resp.Choices {
				choices = append(choices, openai.ChatCompletionStreamChoice{
					Index: i,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Role:         c.Message.Role,
						Content:      c.Message.Content,
						FunctionCall: c.Message.FunctionCall,
						ToolCalls:    c.Message.ToolCalls,
						Refusal:      c.Message.Refusal,
					},
				})
			}
			c.Stream <- openai.ChatCompletionStreamResponse{
				ID:      resp.ID,
				Object:  resp.Object,
				Created: resp.Created,
				Model:   resp.Model,
				Choices: choices,
			}
		}
		c.Stream <- openai.ChatCompletionStreamResponse{Usage: &total}
		c.Stream <- openai.ChatCompletionStreamResponse{
			Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "stop"}},
		}
	}()
	return c, nil
}

func (v *Vertex) BatchUpload(ctx context.Context, batch *openai.Batch, inputs []openai.BatchInput) error {
	if !v.Batch {
		return simp.ErrNotImplemented
	}
	model, ok := ctx.Value(simp.KeyModel).(config.Model)
	if !ok {
		return fmt.Errorf("model not found")
	}
	if !model.Batch {
		return fmt.Errorf("model %q does not support batching", model.Name)
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
		table = batch.ID
		rows  = []vertexBatch{}
	)
	for _, input := range inputs {
		if input.ChatCompletion == nil {
			return fmt.Errorf("embeddings are not supported")
		}
		sect, err := v.encode(ctx, *input.ChatCompletion)
		if err != nil {
			return fmt.Errorf("cannot encode request: %w", err)
		}
		contents, config := sect.Contents, sect.Config
		// Build the request map with only non-nil values
		req := map[string]any{
			"model":    v.googleModel(model.Name),
			"contents": contents,
		}
		if config.SystemInstruction != nil {
			req["system_instruction"] = config.SystemInstruction
		}
		if config.CachedContent != "" {
			req["cached_content"] = config.CachedContent
		}
		if len(config.Tools) > 0 {
			req["tools"] = config.Tools
		}
		if config.ToolConfig != nil {
			req["tool_config"] = config.ToolConfig
		}
		genConfig := map[string]any{}
		if config.Temperature != nil {
			genConfig["temperature"] = *config.Temperature
		}
		if config.TopP != nil {
			genConfig["top_p"] = *config.TopP
		}
		if config.MaxOutputTokens > 0 {
			genConfig["max_output_tokens"] = config.MaxOutputTokens
		}
		if len(config.StopSequences) > 0 {
			genConfig["stop_sequences"] = config.StopSequences
		}
		if config.PresencePenalty != nil {
			genConfig["presence_penalty"] = *config.PresencePenalty
		}
		if config.FrequencyPenalty != nil {
			genConfig["frequency_penalty"] = *config.FrequencyPenalty
		}
		if len(genConfig) > 0 {
			req["generation_config"] = genConfig
		}

		b, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("cannot marshal request: %w", err)
		}
		rows = append(rows, vertexBatch{
			ID:      input.CustomID,
			Request: string(b),
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

	const chunkSize = 200
	for input := 0; input < len(rows); input += chunkSize {
		end := input + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[input:end]

		fmt.Println("inserting chunk", len(chunk), "into", v.Dataset, table)
		if err := inserter.Put(ctx, chunk); err != nil {
			return fmt.Errorf("failed to insert batch chunk: %w", err)
		}
	}
	batch.InputFileID = table
	return nil
}

func (v *Vertex) googleModel(m string) string {
	return fmt.Sprintf("publishers/google/models/%s", m)
	// return fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
	// 	v.Project, v.Region, m)
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
			DisplayName: ifd,
			Model:       v.googleModel(m.Name),
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
			if len(row.Response) == 0 {
				continue
			}
		case iterator.Done:
			return outputs, nil
		default:
			return nil, fmt.Errorf("cannot read from table %q: %w", table, err)
		}
		var resp genai.GenerateContentResponse
		if err := json.Unmarshal([]byte(row.Response), &resp); err != nil {
			return nil, fmt.Errorf("cannot unmarshal response/%s: %w", row.ID, err)
		}
		output, err := v.decode(&resp)
		if err != nil {
			return nil, fmt.Errorf("cannot decode response/%s: %w", row.ID, err)
		}
		output.ID = row.ID
		outputs = append(outputs, openai.BatchOutput{
			CustomID:       row.ID,
			ChatCompletion: &output,
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
	creds, err := google.CredentialsFromJSON(ctx, []byte(v.APIKey), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials file: %w", err)
	}
	httpClient := oauth2.NewClient(ctx, creds.TokenSource)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  v.Project,
		Location: v.region(ctx),
		Credentials: auth.NewCredentials(&auth.CredentialsOptions{
			JSON: []byte(v.APIKey),
		}),
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			APIVersion: "v1beta1",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot make genai client: %w", err)
	}
	return client, nil
}

func (v *Vertex) jobClient(ctx context.Context) (*aipl.JobClient, error) {
	client, err := aipl.NewJobClient(ctx,
		option.WithEndpoint(v.region(ctx)+"-aiplatform.googleapis.com:443"),
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
