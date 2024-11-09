package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp/auth"
	"github.com/busthorne/simp/config"
	"golang.org/x/term"
)

func (w *wizardState) abort() {
	fmt.Println("Aborted.")
	os.Exit(1)
}

func (w *wizardState) confirm(prompt string, formatValues ...any) bool {
	if formatValues != nil {
		prompt = fmt.Sprintf(prompt, formatValues...)
	}
	resp := strings.ToLower(w.prompt(prompt+" (y/n):", "n"))
	return resp == "y" || strings.HasPrefix(resp, "ye")
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

func (w *wizardState) apikey() string {
	for {
		fmt.Print("API key: ")
		p, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // Add a newline after password input
		if err != nil {
			continue
		}
		return string(p)
	}
}

func (w *wizardState) writeConfig() error {
	if err := os.MkdirAll(w.configPath, 0755); err != nil {
		return err
	}
	b, err := config.Configure("simp", w.Config)
	if err != nil {
		return err
	}
	// write file
	fpath := filepath.Join(w.configPath, "simp.hcl")
	return os.WriteFile(fpath, b, 0644)
}

var errNoKeyring = errors.New("no keyring")

func (w *wizardState) keyring(provider ...config.Provider) (keyring.Keyring, error) {
	if len(w.Auth) == 0 {
		return nil, errNoKeyring
	}
	if w.Auth[0].Backend == "config" {
		return nil, errNoKeyring
	}
	var p *config.Provider = nil
	if len(provider) > 0 {
		p = &provider[0]
	}
	return auth.NewKeyring(w.Auth[0], p)
}

func (w *wizardState) openKeyring(backend keyring.BackendType) bool {
	if w.Auth == nil {
		w.Auth = []config.Auth{{Name: "default", Backend: backend}}
	} else {
		w.Auth[0].Backend = backend
	}
	_, err := w.keyring()
	if err != nil {
		fmt.Println("Failed to create keyring:", err)
		return false
	}
	return true
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = strings.Replace(path, "~", home, 1)
		}
	}
	for _, env := range os.Environ() {
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
