package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/busthorne/simp/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (w *wizardState) abort() {
	fmt.Println("Aborted.")
	exit(1)
}

func (w *wizardState) confirm(prompt string, formatValues ...any) bool {
	if formatValues != nil {
		prompt = fmt.Sprintf(prompt, formatValues...)
	}
	prompt += " (y/n)"

	p := wizardPrompt{
		textInput: textinput.New(),
		prompt:    prompt,
		confirm:   true,
	}
	resp := strings.ToLower(w.input(p))
	switch resp {
	case "y", "yes", "ye", "yeah", "yep":
		return true
	default:
		return false
	}
}

func (w *wizardState) prompt(prompt, prefill string) string {
	p := wizardPrompt{
		textInput: textinput.New(),
		prompt:    prompt,
	}
	p.textInput.SetValue(prefill)
	return w.input(p)
}

func (w *wizardState) apikey() string {
	p := wizardPrompt{
		textInput: textinput.New(),
		prompt:    "API key",
	}
	p.textInput.EchoMode = textinput.EchoPassword
	return w.input(p)
}

func (w *wizardState) writeConfig() error {
	if err := os.MkdirAll(w.configPath, 0755); err != nil {
		return err
	}
	b, err := config.Configure("simp", w.Config)
	if err != nil {
		return err
	}
	fpath := filepath.Join(w.configPath, "simp.hcl")
	fmt.Printf("-> %s\n", fpath)
	return os.WriteFile(fpath, b, 0755)
}

func (w *wizardState) defaultProviderName(driver string) string {
	var name = "api"
	var j = -1
	for i, p := range w.Providers {
		if p.Driver == driver {
			j = i
			break
		}
	}
	if j >= 0 {
		name += strconv.Itoa(j + 2)
	}
	return name
}

func expandPath(path string) string {
	environ := os.Environ()
	home, err := os.UserHomeDir()
	if err == nil {
		environ = append(environ, "HOME="+home)
		if strings.HasPrefix(path, "~") {
			path = strings.Replace(path, "~", home, 1)
		}
	}
	for _, env := range environ {
		env := strings.SplitN(env, "=", 2)
		if len(env) != 2 {
			continue
		}
		k, v := env[0], env[1]
		path = strings.ReplaceAll(path, "$"+k, v)
	}
	return path
}

func alreadyExists(path string) bool {
	// check if the path already exists
	_, err := os.ReadDir(path)
	return err == nil
}

type wizardPrompt struct {
	textInput textinput.Model
	prompt    string
	confirm   bool
	aborted   bool
}

func (w *wizardState) input(prompt wizardPrompt) string {
	prompt.textInput.Focus()
	prompt.textInput.Prompt = ""
	p := tea.NewProgram(prompt)
	result, err := p.Run()
	if err != nil {
		w.abort()
	}
	if f := result.(wizardPrompt); f.aborted {
		w.abort()
	} else {
		fmt.Println(result.View())
		return f.textInput.Value()
	}
	return ""
}

func (m wizardPrompt) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlZ, tea.KeyCtrlD:
			m.aborted = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.confirm {
				val := strings.ToLower(m.textInput.Value())
				if val == "y" || val == "n" || val == "yes" || val == "no" {
					return m, tea.Quit
				}
			} else {
				return m, tea.Quit
			}
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

var bold = lipgloss.NewStyle().Bold(true)

func (m wizardPrompt) View() string {
	return fmt.Sprintf("%s: %s", bold.Render(m.prompt), m.textInput.View())
}
