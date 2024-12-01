package driver

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

func vertex(t *testing.T) *Vertex {
	c, err := config.ParsePath(path.Join(simp.Path, "config"))
	if err != nil {
		t.Fatal("parse config:", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal("validate config:", err)
	}
	for _, p := range c.Providers {
		if p.Driver != "vertex" {
			continue
		}
		v, err := NewVertex(p)
		if err != nil {
			t.Error(err)
		}
		return v
	}
	t.Fatal("no vertex provider")
	return nil
}

const vertexModel = "gemini-1.5-flash-002"

var prompts = func(user ...string) (inputs []openai.BatchInput) {
	for _, u := range user {
		inputs = append(inputs, openai.BatchInput{
			CustomID: strings.ReplaceAll(uuid.New().String(), "-", "")[:10],
			ChatCompletion: &openai.ChatCompletionRequest{
				Model: vertexModel,
				Messages: []openai.ChatCompletionMessage{
					{Role: openai.ChatMessageRoleUser, Content: u},
				},
			},
		})
	}
	return
}

func TestVertexBatchUpload(t *testing.T) {
	if os.Getenv("UPLOAD") == "" {
		t.SkipNow()
	}
	v := vertex(t)
	b := openai.Batch{ID: uuid.New().String()[:18]}
	mag := prompts(
		"Who is the president of Ukraine?",
		"Хто президент України?",
	)
	if err := v.BatchUpload(context.Background(), &b, mag); err != nil {
		t.Fatal(err)
	}
	t.Log("batch id:", b.ID)
}

func TestVertexBatchSend(t *testing.T) {
	id := os.Getenv("SEND")
	if id == "" {
		t.SkipNow()
	}
	v := vertex(t)
	b := openai.Batch{
		ID: id,
		Metadata: map[string]any{
			"model": vertexModel,
		},
	}
	if err := v.BatchSend(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	t.Log(b.Metadata)
}

func TestVertexBatchReceive(t *testing.T) {
	id := os.Getenv("RECV")
	if id == "" {
		t.SkipNow()
	}
	v := vertex(t)
	b := openai.Batch{
		ID: id,
		Metadata: map[string]any{
			"model": vertexModel,
			"job":   id,
		},
	}
	mag, err := v.BatchReceive(context.Background(), &b)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range mag {
		t.Log(u.ChatCompletion.Choices[0].Message.Content)
	}
}
