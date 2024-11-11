package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/busthorne/simp"
)

func cabling(prompt string) error {
	if !cable.Empty() {
		cable.AppendUser(prompt)
		return nil
	}
	c, err := simp.ParseCable(prompt)
	if err != nil {
		return err
	}
	cable = c
	return nil
}

// promptComplete reads the prompt from stdin once in non-interactive mode,
// or keeps asking for newline-terminated input otherwise. it would insert
// a prompt marker in either vim, or interactive mode.
func promptComplete() error {
	var prompt string
	switch {
	case *interactive:
		// read until newline
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			prompt = scanner.Text()
		} else {
			return fmt.Errorf("failed to read input: %v", scanner.Err())
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
	ws = cable.Whitespace
	drv, model, err := findWaldo(model)
	if err != nil {
		return err
	}
	if *vim || *interactive {
		fmt.Println()
		fmt.Printf("%s%s %s\n", ws, simp.MarkAsst, model.ShortestAlias())
	}
	resp, err := drv.Complete(bg, simp.Complete{
		Stream:           true,
		Model:            model.Name,
		Messages:         cable.Messages(),
		Temperature:      coalesce32(temperature, cfg.Default.Temperature, model.Temperature),
		TopP:             coalesce32(topP, cfg.Default.TopP, model.TopP),
		FrequencyPenalty: coalesce32(frequencyPenalty, cfg.Default.FrequencyPenalty, model.FrequencyPenalty),
		PresencePenalty:  coalesce32(presencePenalty, cfg.Default.PresencePenalty, model.PresencePenalty),
	})
	if err != nil {
		return fmt.Errorf("complete: %v", err)
	}
	if err := resp.Err; err != nil {
		return fmt.Errorf("stream complete: %v", err)
	}
	for chunk := range resp.Stream {
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
			return fmt.Errorf("stream complete chunk: %w", resp.Err)
		}
	}
	fmt.Println()
	if *vim {
		fmt.Printf("\n%s%s\n\n", ws, simp.MarkUser)
	}
	if !*interactive {
		return io.EOF
	}
	return nil
}
