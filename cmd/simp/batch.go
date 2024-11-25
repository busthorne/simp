package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/gofiber/fiber/v2"
)

var (
	errNoid        = fmt.Errorf("missing custom_id")
	errBadMethod   = fmt.Errorf("POST method is required")
	errMeatNorFish = fmt.Errorf("neither a chat completion nor an embedding")
)

type BatchRequest struct {
	ID        string `json:"custom_id"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	MaxTokens int    `json:"max_tokens,omitempty"`

	Body     json.RawMessage `json:"body,omitempty"`
	Embed    simp.Embed      `json:"-"`
	Complete simp.Complete   `json:"-"`
}

type BatchRequests []BatchRequest

func batchUpload(c *fiber.Ctx) error {
	switch purpose := c.FormValue("purpose"); purpose {
	case "batch":
	default:
		return fmt.Errorf("%s purpose is %w", purpose, notImplemented(c))
	}
	ff, err := c.FormFile("file")
	if err != nil {
		return err
	}
	f, err := ff.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	by := map[string]config.Provider{}
	batches := map[string]BatchRequests{}
	lines := json.NewDecoder(f)
	for i := 1; ; i++ {
		var req BatchRequest
		malformed := func(err error) error {
			return fmt.Errorf("request %s/%d is malformed: %w", req.ID, i, err)
		}
		switch err := lines.Decode(&req); err {
		case nil:
			if req.ID == "" {
				return malformed(errNoid)
			}
		case io.EOF:
			goto agg
		default:
			return malformed(err)
		}
		switch req.Method {
		case "":
		case "POST":
		case "post":
		default:
			return malformed(errBadMethod)
		}
		const chatCompletions = "/v1/chat/completions"
		const embeddings = "/v1/embeddings"
		model := ""
		// conditional unmarshaling
		switch req.URL {
		case chatCompletions:
			if err := json.Unmarshal(req.Body, &req.Complete); err != nil {
				return malformed(err)
			}
			model = req.Complete.Model
		case embeddings:
			if err := json.Unmarshal(req.Body, &req.Embed); err != nil {
				return malformed(err)
			}
			model = string(req.Embed.Model)
		default:
			return malformed(errMeatNorFish)
		}
		m, p, ok := cfg.LookupModel(model)
		if !ok {
			return malformed(fmt.Errorf("model %s not found", model))
		}
		// validation
		switch req.URL {
		case chatCompletions:
			tailrole := ""
			for i, m := range req.Complete.Messages {
				switch m.Role {
				case "system":
					if i != 0 {
						return malformed(fmt.Errorf("system message is misplaced"))
					}
				case "user", "assistant":
					if tailrole == m.Role {
						return malformed(fmt.Errorf("message %d is not alternating", i+1))
					}
				default:
					return malformed(fmt.Errorf("message %d has bad role %s", i+1, m.Role))
				}
				tailrole = m.Role
			}
			req.Complete.Model = m.Name
			req.Body, _ = json.Marshal(req.Complete)
		case embeddings:
			if !m.Embedding {
				return malformed(fmt.Errorf("model %s doesn't do embeddings", model))
			}
			req.Embed.Model = m.Name
			req.Body, _ = json.Marshal(req.Embed)
		}
		pid := p.Driver + "." + p.Name
		by[pid] = p
		batches[pid] = append(batches[pid], req)
	}
agg:
	var jobs []BatchJob
	for pid, batch := range batches {
		p := by[pid]

		var chunks []BatchRequests
		switch p.Driver {
		case "anthropic":
			chunks = batchAgg(batch, 30*1e6, 10*1e3)
		case "openai":
			if p.BaseURL == "" {
				chunks = batchAgg(batch, 190*1e6, 50*1e3)
			}
		}
		job := BatchJob{Chunks: chunks}
		if chunks == nil {
			job.Magazine = batch
		}
		jobs = append(jobs, job)
	}
	return saveJobs(c, jobs)
}

type BatchJob struct {
	// Individual requests done in traditional fashion
	Magazine BatchRequests
	// Batches of requests a la Batch API
	Chunks []BatchRequests
}

func saveJobs(c *fiber.Ctx, jobs []BatchJob) error {
	return notImplemented(c)
}

func batchAgg(batch BatchRequests, maxbytes, maxn int) (chunks []BatchRequests) {
	var (
		chunk  BatchRequests
		rn, rb int
	)
	for _, req := range batch {
		if rn == maxn || rb+len(req.Body) > maxbytes {
			chunks = append(chunks, chunk)
			chunk = nil
			rn, rb = 0, 0
		}
		rn++
		rb += len(req.Body)
		chunk = append(chunk, req)
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}
	return
}
