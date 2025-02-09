package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/busthorne/simp/books"
	"github.com/busthorne/simp/driver"
	"github.com/sashabaranov/go-openai"
)

var setups = map[string][]string{
	"openai":    {"gpt-4o-mini"},
	"anthropic": {"claude-3-5-haiku"},
	"vertex":    {"gemini-1.5-flash-002"},
	"implicit":  {"jina-embeddings-v3"},
	"mixed":     {"text-embedding-3-small", "claude-3-5-haiku", "jina-embeddings-v3"},
}

// Only run the integration tests if LONG is set.
func integration() bool {
	if os.Getenv("INTEGRATION") != "" {
		setup()
		listen()
		books.Open(":memory:")
		return true
	}
	return false
}

func prompt(n int) []string {
	selection := []string{
		"What is the capital of France?",
		"Who is the president of Ukraine?",
		"x^2 - 4x + 3 = 0. Solve for x.",
		"What is the weather in Tokyo?",
		"What is heavier: a kilogram of feathers or a pound of lead?",
		"What is the meaning of life? Answer in 5 words or less.",
		"What is the capital of the moon?",
		"When is the next presidential election? Now: 1 Dec 2024",
		"Why is the sky blue?",
		"What is the speed of light?",
		"What is the square root of 2?",
		"How many planets are in the solar system?",
	}
	rand.Shuffle(len(selection), func(i, j int) {
		selection[i], selection[j] = selection[j], selection[i]
	})
	return selection[:n]
}

func TestBatchOpenAI(t *testing.T) {
	if !integration() {
		t.SkipNow()
	}
	for _, setup := range strings.Split(os.Getenv("SETUP"), ",") {
		t.Run(setup, batch)
	}
}

func batch(t *testing.T) {
	setup := strings.Split(t.Name(), "/")[1]
	models := setups[setup]
	if len(models) == 0 {
		t.Fatal("unknown setup:", setup)
	}
	drv, err := driver.NewDaemon(*cfg.Daemon)
	if err != nil {
		t.Fatal(err)
	}

	var inputs []openai.BatchInput
	for i, model := range models {
		m, _, ok := cfg.LookupModel(model)
		if !ok {
			t.Fatal("model not found:", model)
		}
		prompts := prompt(3)
		for j, prompt := range prompts {
			var input openai.BatchInput
			if m.Embedding {
				input.Embedding = &openai.EmbeddingRequest{
					Model: model,
					Input: []openai.EmbeddingInput{{Text: prompt}},
				}
			} else {
				input.ChatCompletion = &openai.ChatCompletionRequest{
					Model: model,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: prompt,
						},
					},
				}
			}
			input.CustomID = fmt.Sprintf("test-%d-%d", i, j)
			inputs = append(inputs, input)
		}
	}

	ctx := context.Background()
	f, err := drv.CreateFileBatch(ctx, inputs)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(f)

	b, err := drv.CreateBatch(ctx, openai.CreateBatchRequest{InputFileID: f.ID})
	if err != nil {
		t.Fatal(err)
	}

	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	timeout := time.NewTimer(15 * time.Minute)
	completed := false
	for !completed {
		select {
		case <-tick.C:
		case <-timeout.C:
			t.Fatal("timeout")
		}

		b, err = drv.RetrieveBatch(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}

		switch b.Status {
		case openai.BatchStatusCompleted:
			t.Log("batch completed")
			completed = true
		case openai.BatchStatusFailed:
			if b.Errors != nil {
				for _, err := range b.Errors.Data {
					t.Error(err)
				}
			}
			t.Fatal("batch failed")
		default:
			t.Log(b.Status)
		}
	}

	outputs, err := drv.GetBatchContent(ctx, b.OutputFileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) == len(inputs) {
		return
	}
	t.Fatalf("expected %d outputs, got %d", len(inputs), len(outputs))
}
