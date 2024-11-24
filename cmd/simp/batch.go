package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/gofiber/fiber/v2"
	"github.com/sashabaranov/go-openai"
)

var (
	errNoid        = fmt.Errorf("missing custom_id")
	errBadMethod   = fmt.Errorf("POST method is required")
	errMeatNorFish = fmt.Errorf("neither a chat completion nor an embedding")
)

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
	batches := map[string]simp.Batch{}
	lines := json.NewDecoder(f)
	for i := 1; ; i++ {
		var req simp.BatchRequest
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
			req.Complete = nil
		case embeddings:
			if !m.Embedding {
				return malformed(fmt.Errorf("model %s doesn't do embeddings", model))
			}
			req.Embed.Model = openai.EmbeddingModel(m.Name)
			req.Body, _ = json.Marshal(req.Embed)
			req.Embed = nil
		}
		pid := p.Driver + "." + p.Name
		by[pid] = p
		batches[pid] = append(batches[pid], req)
	}
agg:
	var jobs []BatchJob
	for pid, batch := range batches {
		p := by[pid]

		var chunks []simp.Batch
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
			job.Shots = batch
		}
		jobs = append(jobs, job)
	}
	return saveJobs(c, jobs)
}

type BatchJob struct {
	Shots  simp.Batch
	Chunks []simp.Batch
}

func saveJobs(c *fiber.Ctx, jobs []BatchJob) error {
	return notImplemented(c)
}

func batchAgg(batch simp.Batch, maxbytes, maxn int) (chunks []simp.Batch) {
	var (
		chunk  simp.Batch
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
