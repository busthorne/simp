package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/cable"
	"github.com/busthorne/simp/config"
	"github.com/sashabaranov/go-openai"
)

const annotation = `Create a dash-separated slug that describes this conversation.

Only return the slug, nothing else.

For example: learning-about-12-monkeys`

var annotationExpr = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func saveHistory() {
	if cfg.History == nil || anthology == "" {
		return
	}
	title := ""
	// TODO: keep state of previous turns in conversation to deduplicate
	if model := cfg.History.AnnotateWith; model != "" {
		drv, m, err := findWaldo(model)
		if err != nil {
			stderr("simp: cannot find annotation model:", err)
			exit(1)
		}
		task := cable.Message{
			Role:     "system",
			Contents: []cable.Content{{Text: annotation}},
		}
		if cab.Thread[0].Role != "system" {
			cab.Thread = append([]cable.Message{task}, cab.Thread...)
		} else {
			cab.Thread[0] = task
		}
		resp, err := drv.Chat(bg, openai.ChatCompletionRequest{
			Model:    m.Name,
			Messages: cab.Messages(),
		})
		if err != nil {
			stderr("simp: cannot annotate conversation:", err)
			exit(1)
		}
		title = annotationExpr.FindString(resp.Choices[0].Message.Content)
	}

	if title == "" {
		title = time.Now().Format(time.RFC3339)
	}
	// TODO: use cache to avoid writing the same file multiple times
	for i := 0; i < 10; i++ {
		fpath := ""
		if i > 0 {
			fpath = path.Join(anthology, fmt.Sprintf("%s-%d.c.md", title, i+1))
		} else {
			fpath = path.Join(anthology, title+".c.md")
		}
		if _, err := os.Stat(fpath); err != nil {
			os.WriteFile(fpath, []byte(cab.String()), 0644) //nolint:gosec
			break
		}
	}
}

// history will mkdir before returning target history path per working directory
func history(h *config.History, wd string) string {
	if h == nil {
		return ""
	}
	location := h.Location
	if location == "" {
		location = path.Join(simp.Path, "history")
	}
	// exclude if ignored
	for _, hp := range h.Paths {
		if !hp.Ignore {
			continue
		}
		if hp.Glob.MatchString(wd) {
			return ""
		}
	}

	var winner config.HistoryPath
	longestMatch := -1
	for _, hp := range h.Paths {
		if hp.Glob.MatchString(wd) {
			if n := len(hp.Path); n > longestMatch {
				longestMatch = n
				winner = hp
			}
		}
	}
	return path.Join(location, expandGroup(wd, winner))
}

// expandGroup replaces wildcards in group expression with actual values
func expandGroup(path string, hp config.HistoryPath) string {
	if hp.Glob == nil {
		return ""
	}
	matches := hp.Glob.FindStringSubmatch(path)
	target := hp.Group
	for _, match := range matches[1:] {
		target = strings.Replace(target, "*", match, 1)
	}
	return target
}
