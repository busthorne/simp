package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/busthorne/keyring"
	"github.com/hashicorp/hcl/v2"
)

// Config is the root configuration.
type Config struct {
	Default   Default    `hcl:"default,block"`
	Daemon    *Daemon    `hcl:"daemon,block"`
	History   *History   `hcl:"history,block"`
	Auth      []Auth     `hcl:"auth,block"`
	Providers []Provider `hcl:"provider,block"`

	Diagnostics map[string]hcl.Diagnostics
}

func (c *Config) LookupModel(alias string) (m Model, p Provider, ok bool) {
	if suffix := "latest"; strings.HasSuffix(alias, suffix) {
		alias = strings.TrimSuffix(alias, suffix)
	}
	for _, p := range c.Providers {
		for _, m := range p.Models {
			if m.Name == alias || slices.Contains(m.Alias, alias) {
				// TODO: extra setup needed at model lists
				if m.Latest {
					m.Name = m.Name + "-latest"
				}
				return m, p, true
			}
		}
	}
	return
}

type Default struct {
	ModelDefault
	Model string `hcl:"model"`
}

type ModelDefault struct {
	MaxTokens        int      `hcl:"max_tokens,optional"`
	Temperature      *float64 `hcl:"temperature,optional"`
	TopP             *float64 `hcl:"top_p,optional"`
	FrequencyPenalty *float64 `hcl:"frequency_penalty,optional"`
	PresencePenalty  *float64 `hcl:"presence_penalty,optional"`
}

// Daemon is the simpd portion of the config, the key and cert files
// are picked up from the keyring, and never in plaintext
// as that would be bad taste.
type Daemon struct {
	DaemonAddr string   `hcl:"daemon_addr,optional"`
	ListenAddr string   `hcl:"listen_addr,optional"`
	AutoTLS    bool     `hcl:"auto_tls,optional"`
	Keyring    string   `hcl:"keyring,optional"`
	AllowedIPs []string `hcl:"allowed_ips,optional"`
}

func (d Daemon) BaseURL() string {
	addr := d.ListenAddr
	if addr == "" {
		addr = d.DaemonAddr
	}
	return fmt.Sprintf("%s/v1", addr)
}

// History is the past conversations remembered by either simp, or simpd.
type History struct {
	// Location is $SIMPPATH/history by default.
	Location     string        `hcl:"location,optional"`
	Paths        []HistoryPath `hcl:"path,block"`
	AnnotateWith string        `hcl:"annotate_with,optional"`
}

// HistoryPath is a path to a directory containing conversations.
//
// It supports pseudo-globbing, i.e. `path/to/*/` will only match that
// path alone, and `path/to/*/**` will match all files and directories
// inside.
//
// Group expression will expand both * and ** as relative paths.
type HistoryPath struct {
	Path   string `hcl:"path,label"`
	Group  string `hcl:"group,optional"`
	Ignore bool   `hcl:"ignore,optional"`
}

// Auth doubles as secrets manager and auth manager.
//
// For example, `keyring` will use some kind of secrets manager to store
// the API keys, and `cloudflare` will provide SSO and RBAC for
// simpd.
type Auth struct {
	Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Backend keyring.BackendType `hcl:"backend"`

	// MacOSKeychainNameKeychainName is the name of the macOS keychain that is used
	KeychainName string `hcl:"keychain_name,optional"`
	// KeychainSynchronizable is whether the item can be synchronized to iCloud
	KeychainSynchronizable bool `hcl:"keychain_icloud,optional"`
	// FileDir is the directory that keyring files are stored in, ~/ is resolved to the users' home dir
	FileDir string `hcl:"file_dir,optional"`
	// KWalletAppID is the application id for KWallet
	KWalletAppID string `hcl:"kwallet_app,optional"`
	// KWalletFolder is the folder for KWallet
	KWalletFolder string `hcl:"kwallet_dir,optional"`
	// LibSecretCollectionName is the name collection in secret-service
	LibSecretCollectionName string `hcl:"libsecret_collection,optional"`
	// PassDir is the pass password-store directory, ~/ is resolved to the users' home dir
	PassDir string `hcl:"pass_dir,optional"`
	// PassCmd is the name of the pass executable
	PassCmd string `hcl:"pass_cmd,optional"`
}

// Provider is a provider of LLM services, such as OpenAI, Anthropic, etc.
//
// OpenAI-compatible providers are supported out of the box, because they
// share the API via a common driver. This is why the driver is more a
// spec than anything.
type Provider struct {
	Driver     string   `hcl:"driver,label"`
	Name       string   `hcl:"name,label"`
	BaseURL    string   `hcl:"base_url,optional"`
	APIKey     string   `hcl:"apikey,optional"`
	Keyring    string   `hcl:"keyring,optional"`
	Models     []Model  `hcl:"model,block"`
	AllowedIPs []string `hcl:"allowed_ips,optional"`
}

// Model is a set of overrides passed to driver so that it can better
// integrate with a specific provider, use controls that would be
// desirable.
//
// It's up to driver whether to allow models by default.
type Model struct {
	ModelDefault
	Name          string   `hcl:"name,label"`
	Alias         []string `hcl:"alias,optional"`
	Tags          []string `hcl:"tags,optional"`
	AllowedIPs    []string `hcl:"allowed_ips,optional"`
	ContextLength int      `hcl:"context_length,optional"`
	Latest        bool     `hcl:"latest,optional"`
	Ignore        bool     `hcl:"ignore,optional"`
	Embedding     bool     `hcl:"embedding,optional"`
	Images        bool     `hcl:"images,optional"`
}

func (m Model) ShortestAlias() (alias string) {
	for _, a := range m.Alias {
		if len(a) < len(alias) {
			alias = a
		}
	}
	if alias == "" {
		return m.Name
	}
	return
}
