package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
	"github.com/busthorne/simp/driver"
)

type wizardState struct {
	config.Config
	reader     *bufio.Reader
	configPath string
}

func wizard() {
	w := &wizardState{
		reader:     bufio.NewReader(os.Stdin),
		configPath: path.Join(simp.Path, "config"),
	}
	// first, ask them where they want the config to go
	for {
		p := expandPath(w.prompt("Will configure in [$SIMPPATH/config]:", w.configPath))
		if p != "" {
			w.configPath = p
			break
		}
	}
	if alreadyExists(w.configPath) {
		if !w.confirm("%s already exists. ERASE?", w.configPath) {
			w.abort()
		}
		os.RemoveAll(w.configPath)
	} else {
		os.MkdirAll(simp.Path, 0755)
		os.MkdirAll(w.configPath, 0755)
		if !alreadyExists(w.configPath) {
			fmt.Println("Failed to create config directory:", w.configPath)
			w.abort()
		}
	}
	// daemon setup
	if w.confirm("\nWould you like to set up the daemon?") {
		const defaultAddr = "localhost:51015"
		var listenAddr string
		for {
			listenAddr = w.prompt("Listen address ["+defaultAddr+"]:", defaultAddr)
			switch spl := strings.Split(listenAddr, ":"); len(spl) {
			case 2:
			default:
				fmt.Println("Invalid listen address")
				continue
			}
			break
		}
		w.Daemon = &config.Daemon{
			ListenAddr: listenAddr,
		}
	}
	// then, ask them how they want their keys
	w.configureKeyring()
	// setup providers
	fmt.Println()
	fmt.Println("Let's configure some inference providers. Use openai for compatible apis.")
	for {
		fmt.Println("Available drivers:", strings.Join(driver.Drivers, ", "))
		fmt.Println("Enter x to quit.")
		driverName := w.prompt("Select driver, or quit [x]:", "")

		var p config.Provider
		switch driverName {
		case "openai":
			p = w.configureOpenAI()
		case "anthropic":
			p = w.configureAnthropic()
		case "dify":
			fmt.Println("TBA.")
			continue
		case "x", "done", "quit":
			goto provided
		default:
			fmt.Println("Unsupported driver:", driverName)
			continue
		}
		ring, err := w.keyring(p)
		if err != nil {
			fmt.Println("Failed to open keyring:", err)
			w.abort()
		}
		err = ring.Set(keyring.Item{Key: "apikey", Data: []byte(p.APIKey)})
		if err != nil {
			fmt.Println("Failed to save to keychain:", err)
			w.abort()
		}
		w.Providers = append(w.Providers, p)
	}
provided:
	w.printProviders()

	// histories, & annotations thereof
	if w.confirm("\nWould you like to retain conversation histories?") {
		path := simp.Path + "/history"
		hp := w.prompt("History location [$SIMPPATH/history]:", path)
		if hp == path {
			hp = ""
		}
		model := ""
		if w.confirm("Would you like to enable filename annotations?") {
			model = w.prompt("Annotation model or alias:", "")
		}
		w.History = &config.History{
			Location:     hp,
			AnnotateWith: model,
		}
	}

	// Generate and write config
	if err := w.writeConfig(); err != nil {
		fmt.Printf("Failed to write config: %v\n", err)
		w.abort()
	}

	fmt.Println("\nConfiguration complete! You can now use simp.")
}

func (w *wizardState) configureKeyring() {
	fmt.Println("\nHow would you like to store API keys?")
	fmt.Println("\t- config (not recommended)")
	backends := map[keyring.BackendType]struct{}{}
	for _, backend := range keyring.AvailableBackends() {
		backends[backend] = struct{}{}
		fmt.Printf("\t- %s\n", backend)
	}
	var backend keyring.BackendType
	for {
		backend = keyring.BackendType(w.prompt("Store API keys in:", ""))
		if backend == "config" {
			break
		}
		if _, ok := backends[backend]; !ok {
			fmt.Println("Unsupported backend")
			continue
		}
		if w.openKeyring(backend) {
			break
		}
	}
	if backend == "config" {
		fmt.Println("Okay, it's your choice.")
	} else {
		fmt.Println("Keyring configured.")
	}
}

func (w *wizardState) configureOpenAI() (p config.Provider) {
	const openaiBaseURL = "https://api.openai.com/v1"
	p.Driver = "openai"
	p.BaseURL = strings.Trim(w.prompt("Base URL:", openaiBaseURL), "/")
	if p.BaseURL == openaiBaseURL {
		p.BaseURL = ""
	}
	p.Name = w.prompt("Provider name [default]:", "")
	if p.Name == "" {
		p.Name = w.defaultProviderName("openai")
	}
	p.APIKey = w.apikey()
	return
}

func (w *wizardState) configureAnthropic() (p config.Provider) {
	p.APIKey = w.apikey()
	p.Name = w.defaultProviderName("anthropic")
	return
}

func (w *wizardState) printProviders() {
	fmt.Println("Configured providers and models:")
	for _, p := range w.Providers {
		fmt.Printf("\t- %s\n", p.Driver, p.Name)
		for _, m := range p.Models {
			alias := ""
			if len(m.Alias) > 0 {
				alias = fmt.Sprintf(" (%s)", strings.Join(m.Alias, ", "))
			}
			fmt.Printf("\t\t%s%s\n", m.Name, alias)
		}
	}
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
	if j == -1 {
		name += strconv.Itoa(j + 2)
	}
	return name
}
