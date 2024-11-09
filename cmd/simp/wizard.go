package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/spewerspew/spew"
)

type wizardState struct {
	reader        *bufio.Reader
	configPath    string
	useKeyring    bool
	providers     []providerConfig
	setupDaemon   bool
	setupHistory  bool
	annotateHist  bool
	historyPath   string
	daemonConfig  *config.Daemon
	historyConfig *config.History
}

type providerConfig struct {
	driver  string
	name    string
	baseURL string
	apiKey  string
	models  []config.Model
}

func wizard() {
	state := &wizardState{
		reader:     bufio.NewReader(os.Stdin),
		configPath: path.Join(simp.Path, "config"),
	}
	if notEmpty(state.configPath) {
		if !state.confirm("Config directory already exists. This will erase existing config. Continue?") {
			return
		}
		os.RemoveAll(state.configPath)
	} else {
		os.MkdirAll(simp.Path, 0755)
		os.MkdirAll(state.configPath, 0755)
	}

	// Auth setup
	fmt.Println("\nHow would you like to store API keys?")
	fmt.Println("1. System keyring (recommended)")
	fmt.Println("2. Plaintext in config")
	choice := state.prompt("Enter choice [1]:", "1")
	state.useKeyring = choice == "1"

	const openaiBaseURL = "https://api.openai.com/v1"
	// Provider setup
	for {
		fmt.Println("\nAvailable providers:")
		fmt.Println("1. OpenAI")
		fmt.Println("2. Anthropic")
		fmt.Println("x. Done")

		choice = state.prompt("Select provider to configure [x]:", "")
		if choice == "" || choice == "x" {
			break
		}

		var p providerConfig
		switch choice {
		case "1":
			p.driver = "openai"
			p.baseURL = state.prompt("Base URL:", openaiBaseURL)
			if p.baseURL != openaiBaseURL {
				p.name = state.prompt("Provider name:", "")
			}
		case "2":
			p.driver = "anthropic"
		default:
			continue
		}

		if state.useKeyring {
			p.apiKey = state.prompt("API Key (will be stored in system keyring):", "")
		} else {
			p.apiKey = state.prompt("API Key:", "")
		}

		// Model aliases
		if state.confirm("\nWould you like to configure model aliases?") {
			switch p.driver {
			case "openai":
				if p.baseURL == "https://api.openai.com/v1" {
					alias := state.prompt("Alias for gpt-4o [4o]:", "4o")
					p.models = append(p.models, config.Model{
						Name:  "gpt-4o",
						Alias: []string{alias},
					})
				}
			case "anthropic":
				alias := state.prompt("Alias for claude-3-5-sonnet-latest [sonnet]:", "sonnet")
				p.models = append(p.models, config.Model{
					Name:   "claude-3-5-sonnet-latest",
					Alias:  []string{alias},
					Latest: true,
				})
			}
		}

		state.providers = append(state.providers, p)
	}

	// Daemon setup
	if state.confirm("\nWould you like to set up the daemon?") {
		state.setupDaemon = true
		state.daemonConfig = &config.Daemon{
			ListenAddr: state.prompt("Listen address [127.0.0.1:51015]:", "127.0.0.1:51015"),
			AutoTLS:    state.confirm("Enable automatic TLS? [n]:"),
		}
	}

	// History setup
	if state.confirm("\nWould you like to set up conversation history?") {
		state.setupHistory = true
		state.historyPath = state.prompt("History location [$SIMPPATH/history]:", "$SIMPPATH/history")
		state.historyConfig = &config.History{
			Location: state.historyPath,
		}

		if state.setupDaemon {
			if state.confirm("Would you like to enable filename annotations?") {
				state.annotateHist = true
				state.historyConfig.Annotate = true
				state.historyConfig.AnnotateWith = "sonnet" // Default to first anthropic model if available
				for _, p := range state.providers {
					if p.driver == "anthropic" {
						for _, m := range p.models {
							state.historyConfig.AnnotateWith = m.ShortestAlias()
							break
						}
					}
				}
			}
		}
	}

	// Generate and write config
	if err := state.writeConfig(); err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		return
	}

	fmt.Println("\nConfiguration complete! You can now use simp.")
}

func (w *wizardState) confirm(prompt string) bool {
	resp := strings.ToLower(w.prompt(prompt+" (y/n):", "n"))
	return resp == "y" || resp == "yes"
}

func (w *wizardState) prompt(prompt, defaultVal string) string {
	fmt.Print(prompt + " ")
	input, _ := w.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func (w *wizardState) writeConfig() error {
	if err := os.MkdirAll(w.configPath, 0755); err != nil {
		return err
	}
	spew.Dump(w)
	return nil
}

func notEmpty(path string) bool {
	// check if the path contains files
	_, err := os.ReadDir(path)
	return err == nil
}
