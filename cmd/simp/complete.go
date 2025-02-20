package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/cable"
	"github.com/sashabaranov/go-openai"
)

func cabling(prompt string) error {
	if !cab.Empty() {
		cab.AppendUser(prompt)
		return nil
	}
	c, err := cable.ParseCable(prompt)
	if err != nil {
		return err
	}
	cab = c
	return nil
}

// promptComplete reads the prompt from stdin once in non-interactive mode,
// or keeps asking for newline-terminated input otherwise. it would insert
// a prompt marker in either vim, or interactive mode.
func promptComplete() error {
	var prompt string
	switch {
	case *interactive:
		s, err := multiline()
		switch err {
		case nil:
			prompt = s
		case io.EOF:
			return io.EOF
		default:
			return err
		}
	default:
		// read prompt
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("no input: %v", err)
		}
		prompt = string(b)
	}
	// parse cable
	if err := cabling(prompt); err != nil {
		return fmt.Errorf("bad cable: %v", err)
	}
	drv, m, err := findWaldo(model)
	if err != nil {
		return err
	}
	if *vim || *interactive {
		fmt.Println()
		fmt.Printf("%s %s\n", cab.Tab(simp.GuidelineOutput), m.ShortestAlias())
	}
	var so *openai.StreamOptions
	if !*nos {
		so = &openai.StreamOptions{
			IncludeUsage: true,
		}
	}
	start := time.Now()
	ctx := context.WithValue(bg, simp.KeyModel, m)
	resp, err := drv.Chat(ctx, openai.ChatCompletionRequest{
		Stream:           !*nos,
		Model:            m.Name,
		Messages:         cab.Messages(),
		Temperature:      coalesce(temperature, m.Temperature, cfg.Default.Temperature),
		TopP:             coalesce(topP, m.TopP, cfg.Default.TopP),
		FrequencyPenalty: coalesce(frequencyPenalty, m.FrequencyPenalty, cfg.Default.FrequencyPenalty),
		PresencePenalty:  coalesce(presencePenalty, m.PresencePenalty, cfg.Default.PresencePenalty),
		StreamOptions:    so,
	})
	if err != nil {
		stderrf("%T %v\n", drv, err)
		exit(1)
	}
	if *nos {
		fmt.Println(resp.Choices[0].Message.Content)
		goto suffix
	}
	for chunk := range resp.Stream {
		if chunk.Usage != nil {
			resp.Usage = *chunk.Usage
			continue
		}
		c := chunk.Choices[0]
		switch c.FinishReason {
		case "":
			fmt.Print(chunk.Choices[0].Delta.Content)
		case "stop":
		case "length":
		case "function_call":
		case "tool_calls":
		case "content_filter":
		case "null":
		case "error":
			stderr("stream complete:", chunk.Error)
			exit(1)
		}
	}
	fmt.Println()
suffix:
	if *verbose {
		stderrf("\n\t\t\t%d", resp.Usage.PromptTokens)
		if resp.Usage.PromptTokensDetails != nil {
			stderrf(" (%d)", resp.Usage.PromptTokensDetails.CachedTokens)
		}
		stderrf(" + %d = %d\t%v\n",
			resp.Usage.CompletionTokens,
			resp.Usage.TotalTokens,
			time.Since(start).Round(time.Second/100))
	}
	if *vim {
		fmt.Printf("\n%s\n", cab.Tab(simp.GuidelineInput))
	}
	if !*interactive {
		return io.EOF
	}
	return nil
}
