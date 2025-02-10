package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

// Config is the root configuration.
type Config struct {
	Default   *Default   `hcl:"default,block"`
	Daemon    *Daemon    `hcl:"daemon,block"`
	History   *History   `hcl:"history,block"`
	Auth      []Auth     `hcl:"auth,block"`
	Providers []Provider `hcl:"provider,block"`

	Diagnostics map[string]hcl.Diagnostics
}

type lookupOk struct {
	Model    Model
	Provider Provider
}

var lookupCache = make(map[string]lookupOk)

func (c *Config) ClearCache() {
	lookupCache = make(map[string]lookupOk)
}

func (c *Config) LookupModel(alias string) (m Model, p Provider, ok bool) {
	if v, ok := lookupCache[alias]; ok {
		return v.Model, v.Provider, true
	}

	if suffix := "-latest"; strings.HasSuffix(alias, suffix) {
		alias = strings.TrimSuffix(alias, suffix)
	}
	for _, p := range c.Providers {
		for _, m := range p.Models {
			if m.Name == alias || slices.Contains(m.Alias, alias) {
				// TODO: extra setup needed at model lists
				if m.Latest {
					m.Name += "-latest"
				}
				lookupCache[alias] = lookupOk{Model: m, Provider: p}
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
	Seed             *int32   `hcl:"seed,optional"`
	Stop             []string `hcl:"stop,optional"`
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
	addr = strings.ReplaceAll(addr, "0.0.0.0", "127.0.0.1")
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

	Glob *regexp.Regexp
}

// Auth doubles as secrets manager and auth manager.
//
// For example, `keyring` will use some kind of secrets manager to store
// the API keys, and `cloudflare` will provide SSO and RBAC for
// simpd.
type Auth struct {
	Type    string `hcl:"type,label"`
	Name    string `hcl:"name,label"`
	Backend string `hcl:"backend"`
	Default bool   `hcl:"default,optional"`

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
	Batch      bool     `hcl:"batch,optional"`

	// Vertex AI
	Project string `hcl:"project,optional"`
	Region  string `hcl:"region,optional"`
	Dataset string `hcl:"dataset,optional"`
	Bucket  string `hcl:"bucket,optional"`
}

// Model is a set of overrides passed to driver so that it can better
// integrate with a specific provider, use controls that would be
// desirable.
//
// It's up to driver whether to allow models by default.
type Model struct {
	Name          string   `hcl:"name,label"`
	Alias         []string `hcl:"alias,optional"`
	Tags          []string `hcl:"tags,optional"`
	AllowedIPs    []string `hcl:"allowed_ips,optional"`
	ContextLength int      `hcl:"context_length,optional"`
	Dimensions    int      `hcl:"dimensions,optional"`
	Latest        bool     `hcl:"latest,optional"`
	Ignore        bool     `hcl:"ignore,optional"`
	Embedding     bool     `hcl:"embedding,optional"`
	Images        bool     `hcl:"images,optional"`
	Videos        bool     `hcl:"videos,optional"`
	Reasoning     bool     `hcl:"reasoning,optional"`

	ModelDefault
}

func (m Model) ShortestAlias() (alias string) {
	alias = m.Name
	for _, a := range m.Alias {
		if len(a) < len(alias) {
			alias = a
		}
	}
	return
}
