package driver

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

func vertex() *Vertex {
	p := config.Provider{
		APIKey:  os.Getenv("VERTEX_JSON"),
		Project: "busthorne",
		Region:  "europe-north1",
	}
	v, err := NewVertex(p)
	if err != nil {
		panic(err)
	}
	return v
}

const vertexModel = "gemini-1.5-pro-002"

var prompts = func(user ...string) (mag simp.Magazine) {
	for _, u := range user {
		mag = append(mag, simp.BatchUnion{
			Id: strings.ReplaceAll(uuid.New().String(), "-", "")[:10],
			Cin: &openai.ChatCompletionRequest{
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
		t.Skip()
	}
	v := vertex()
	b := &simp.Batch{}
	mag := prompts(
		"Who is the president of Ukraine?",
		"Хто президент України?",
	)
	if err := v.BatchUpload(context.Background(), b, mag); err != nil {
		t.Fatal(err)
	}
	t.Log("batch id:", b.ID)
}

func TestVertexBatchSend(t *testing.T) {
	id := os.Getenv("SEND")
	if id == "" {
		t.Skip()
	}
	v := vertex()
	b := &simp.Batch{
		ID: id,
		Metadata: map[string]any{
			"model": vertexModel,
		},
	}
	if err := v.BatchSend(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	t.Log(b.Metadata)
}

func TestVertexBatchReceive(t *testing.T) {
	id := os.Getenv("RECV")
	if id == "" {
		t.Skip()
	}
	p := config.Provider{
		APIKey:  os.Getenv("VERTEX_JSON"),
		Project: "busthorne",
		Region:  "europe-north1",
	}
	v, err := NewVertex(p)
	if err != nil {
		t.Fatal(err)
	}
	b := &simp.Batch{
		ID: id,
		Metadata: map[string]any{
			"job": id,
		},
	}
	mag, err := v.BatchReceive(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range mag {
		t.Log(u.Cout.Choices[0].Message.Content)
	}
}
