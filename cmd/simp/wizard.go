package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/auth"
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
		p := expandPath(w.prompt("Will configure in [$SIMPPATH/config]", w.configPath))
		if p != "" {
			w.configPath = p
			break
		}
	}
	if alreadyExists(w.configPath) {
		if !w.confirm("%s already exists. ERASE?", w.configPath) {
			w.abort()
		}
		// os.RemoveAll(w.configPath)
		os.Mkdir(w.configPath, 0755)
	} else {
		os.MkdirAll(w.configPath, 0755)
		if !alreadyExists(w.configPath) {
			fmt.Println("Failed to create config directory:", w.configPath)
			w.abort()
		}
	}
	// daemon setup
	if w.confirm("Would you like to set up the daemon?") {
		const defaultAddr = "localhost:51015"
		var listenAddr string
		for {
			listenAddr = w.prompt("Listen address ["+defaultAddr+"]", defaultAddr)
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
	fmt.Println("Now, let's configure inference providers. (Note: you may use openai driver for compatible apis.)")
	for {
	configure:
		fmt.Println()
		fmt.Println("Available drivers:", strings.Join(driver.Drivers, ", "))
		driverName := w.prompt("Select driver [leave blank to continue]", "")

		var p config.Provider
		switch driverName {
		case "openai":
			p = w.configureOpenAI()
		case "anthropic":
			p = w.configureAnthropic()
		case "dify":
			fmt.Println("TBA.")
			continue
		case "":
			goto provided
		default:
			fmt.Println("Unsupported driver:", driverName)
			continue
		}
		ring, err := w.keyring(p)
		if err != nil && err != errNoKeyring {
			fmt.Println("Failed to open keyring:", err)
			w.abort()
		}
		if err != errNoKeyring {
			err = ring.Set(keyring.Item{Key: "apikey", Data: []byte(p.APIKey)})
			if err != nil {
				fmt.Println("Failed to save to keychain:", err)
				w.abort()
			}
			p.APIKey = ""
		}
		// models and aliases
		for {
			yes := w.confirm("Would you like to configure models and model aliases?")
			if yes {
				w.configureModels(&p)
				break
			}
			if p.Driver == "dify" && len(p.Models) == 0 {
				fmt.Println("Dify API currently requires that bearer token is set per model.")
				if w.confirm("Abort Dify configuration?") {
					fmt.Println("Aborted.")
					break
				}
				goto configure
			}
			break
		}

		w.Providers = append(w.Providers, p)
	}
provided:
	w.printProviders()
	w.Default.Model = w.prompt("Default model or alias", "")
	if w.confirm("Would you like to retain conversation histories?") {
		w.configureHistory()
	}
	if err := w.writeConfig(); err != nil {
		fmt.Printf("Failed to write config: %v\n", err)
		w.abort()
	}
	fmt.Println()
	fmt.Println("Configuration complete! You can now use simp.")
	exit(0)
}

func (w *wizardState) configureKeyring() {
	fmt.Println("How would you like to store your secrets such as API keys?")
	fmt.Println("\tconfig (not recommended)")
	backends := keyring.AvailableBackends()
	for _, backend := range backends {
		fmt.Printf("\t%s\n", backend)
	}
	var backend keyring.BackendType = backends[0]
	for {
		backend = keyring.BackendType(w.prompt("Store keys in", string(backend)))
		if backend == "config" {
			break
		}
		if !slices.Contains(backends, backend) {
			fmt.Println("Unsupported backend")
			continue
		}
		cfg := config.Auth{Type: "keyring", Backend: backend}
		cfg.Name = w.prompt("Keyring name [default]", "default")
		switch backend {
		case "config":
			fmt.Println("Okay, it's your choice.")
			return
		case "keychain":
			cfg.KeychainName = w.prompt("Keychain name", "login")
			cfg.KeychainSynchronizable = w.confirm("Allow sync to iCloud?")
		case "file":
			cfg.FileDir = expandPath(w.prompt("File directory [$SIMPPATH/keys]", path.Join(simp.Path, "keys")))
			if err := os.MkdirAll(cfg.FileDir, 0755); err != nil {
				fmt.Println("Failed to create directory:", err)
				goto tryElse
			}
		case "kwallet":
			cfg.KWalletAppID = w.prompt("KWallet app id", "simp")
			cfg.KWalletFolder = w.prompt("KWallet folder", "")
		case "libsecret":
			cfg.LibSecretCollectionName = w.prompt("Libsecret collection name", "")
		case "pass":
			cfg.PassDir = expandPath(w.prompt("Pass directory", ""))
			cfg.PassCmd = w.prompt("Pass command [pass]", "pass")
		default:
			goto tryElse
		}
		if _, err := auth.NewKeyring(cfg, nil); err != nil {
			fmt.Println("Failed to create keyring:", err)
			goto tryElse
		}
		w.Auth = []config.Auth{cfg}
		break
	tryElse:
		fmt.Println("Please try some other backend.")
	}
	fmt.Println("Keyring configured.")
}

func (w *wizardState) configureOpenAI() (p config.Provider) {
	const openaiBaseURL = "https://api.openai.com/v1"
	p.Driver = "openai"
	p.BaseURL = strings.Trim(w.prompt("Base URL", openaiBaseURL), "/")
	if p.BaseURL == openaiBaseURL {
		p.BaseURL = ""
	}
	p.Name = w.prompt("Provider name", w.defaultProviderName("openai"))
	p.APIKey = w.apikey()
	if p.BaseURL == "" {
		return
	}
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
		fmt.Printf("\t^ provider \"%s\" \"%s\"\n", p.Driver, p.Name)
		for _, m := range p.Models {
			alias := ""
			if len(m.Alias) > 0 {
				alias = fmt.Sprintf(" (%s)", strings.Join(m.Alias, ", "))
			}
			fmt.Printf("\t\t* %s%s\n", m.Name, alias)
		}
	}
}

func (w *wizardState) configureModels(p *config.Provider) {
	type taken struct{}
	models := map[string]taken{}

	fmt.Println("You'll be able to follow the config to add more models and aliases later.")
	for {
		var m config.Model
		m.Name = w.prompt("Model name [leave blank to continue]", "")
		if m.Name == "" {
			fmt.Printf("Configured %d models.\n", len(p.Models))
			break
		}
		// converted model_name:tag to canonical model_name
		if spl := strings.Split(m.Name, ":"); len(spl) > 1 {
			name, tag := spl[0], spl[1]
			m.Name = name
			m.Tags = append(m.Tags, tag)
		}
		if suffix := "-latest"; strings.HasSuffix(m.Name, suffix) {
			m.Name = strings.TrimSuffix(m.Name, suffix)
			m.Latest = true
		}
		maybe := func(s string) bool {
			return strings.HasPrefix(m.Name, s)
		}
		switch {
		case maybe("gpt"), maybe("claude"), maybe("llama"), maybe("gemma"):
			m.Embedding = false
		case maybe("text-embedding"), strings.Contains(m.Name, "embedding"):
			m.Embedding = true
		default:
			m.Embedding = w.confirm("Is this an embedding model?")
		}
		placeholder := ""
		switch spl := strings.Split(m.Name, "-"); {
		case p.Driver == "openai":
			if spl[0] == "gpt" && len(spl) > 1 {
				placeholder = spl[1]
			} else {
				placeholder = spl[0]
			}
		default:
			placeholder = strings.Join(simp.Map(spl, func(s string) string {
				return s[0:1]
			}), "")
			if n, ni := m.Name, len(m.Name); len(placeholder) <= 2 {
				placeholder = n[0:1] + strconv.Itoa(ni-2) + n[ni-1:]
			}
		}
		for {
			const prompt = "Model alias, comma separated [leave blank for none]"
			aliasByComma := w.prompt(prompt, placeholder)
			if aliasByComma == "" {
				break
			}
			m.Alias =
				slices.Compact(
					slices.Sorted(
						slices.Values(
							simp.Map(
								strings.Split(aliasByComma, ","),
								strings.TrimSpace))))
			for _, alias := range m.Alias {
				if _, ok := models[alias]; ok {
					fmt.Println("Alias already used:", alias)
					continue
				}
			}
			if w.confirm("Aliases: %v. Accept?", m.Alias) {
				break
			}
		}
		for {
			if len(m.Tags) == 0 || p.BaseURL == "" {
				break
			}
			fmt.Println()
			fmt.Println("Some providers offer model_name:tag for quantized versions.\n" +
				"Note: unless a tag is provided for a tagged model, " +
				"simp will always use the first, primary tag.")
			if len(m.Tags) > 0 {
				placeholder = strings.Join(m.Tags, ", ")
			}
			const prompt = "Model version tags, comma separated [leave blank for none]"
			tagsByComma := w.prompt(prompt, placeholder)
			if tagsByComma == "" {
				m.Tags = nil
				break
			}
			m.Tags = simp.Map(strings.Split(tagsByComma, ","), strings.TrimSpace)
			if w.confirm("Tags: %v. Accept?", m.Alias) {
				break
			}
		}
		p.Models = append(p.Models, m)
		models[m.Name] = taken{}
		for _, alias := range m.Alias {
			models[alias] = taken{}
		}
	}
}

func (w *wizardState) configureHistory() {
	defaultPath := path.Join(simp.Path, "history")
	for {
		hp := expandPath(w.prompt("History location [$SIMPPATH/history]", defaultPath))
		if err := os.MkdirAll(hp, 0755); err != nil {
			fmt.Println("Failed to create history directory:", err)
			continue
		}
		if hp == defaultPath {
			hp = ""
		}
		var paths []config.HistoryPath
		if w.confirm("Would you like to group certain histories by path?") {
			fmt.Println(historyHelp)
			for {
				var p config.HistoryPath
				p.Path = w.prompt("Path [leave blank to continue]", "")
				if p.Path == "" {
					break
				}
				prefill := ""
				if strings.Contains(p.Path, "*") {
					prefill = "*"
				}
				p.Group = w.prompt("Group by", prefill)
				paths = append(paths, p)
			}
		}
		if w.confirm("Would you like to ignore certain paths?") {
			for {
				var p config.HistoryPath
				p.Path = w.prompt("Path [leave blank to continue]", "")
				if p.Path == "" {
					break
				}
				p.Ignore = true
				paths = append(paths, p)
			}
		}

		model := ""
		if w.Daemon != nil && w.confirm("Would you like to annotate cables histories?") {
			model = w.prompt("Annotation model or alias", "")
		}
		w.History = &config.History{
			Location:     hp,
			Paths:        paths,
			AnnotateWith: model,
		}
		return
	}
}

const historyHelp = `By default, all simp cables are stored in the same history directory.
You can override this by specifying paths and grouping expressions, which will be used
to catalogue the cables appropriately.

Note that you may use wildcards. For example, you could group the cables produced in
/opt/projects by top-level project directory name. In that case, you would use
/opt/projects/*/**, and * in the group expression. For example: if you had a project
called "simp", all cables created in /opt/projects/simp will be saved under "simp/"
in the history directory.

The longest-prefix match wins; you may later configure to ignore certain paths.

Without ** in the path expression, the paths themselves would be considered, while
their children would not. Similarly "/opt/projects/simp" would not apply to the
child directories inside simp. If you wish to include children,
you should always use the ** suffix.

Who knows, we might add regular expressions someday :-)`
