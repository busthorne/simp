package config

import (
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

type Default struct {
	Model            string   `hcl:"model,attr"`
	MaxTokens        *int     `hcl:"max_tokens,optional"`
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
type HistoryPath struct {
	Path string `hcl:"path,label"`
	// Group is the folder in the history location, `*` from path will expand.
	Group string `hcl:"group"`
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
	Models     []Model  `hcl:"model,block"`
	AllowedIPs []string `hcl:"allowed_ips,optional"`
}

// Model is a set of overrides passed to driver so that it can better
// integrate with a specific provider, use controls that would be
// desirable.
//
// It's up to driver whether to allow models by default.
type Model struct {
	Name             string   `hcl:"name,label"`
	Alias            []string `hcl:"alias,optional"`
	Tags             []string `hcl:"tags,optional"`
	AllowedIPs       []string `hcl:"allowed_ips,optional"`
	ContextLength    *int     `hcl:"context_length,optional"`
	MaxTokens        *int     `hcl:"max_tokens,optional"`
	Temperature      *float64 `hcl:"temperature,optional"`
	TopP             *float64 `hcl:"top_p,optional"`
	FrequencyPenalty *float64 `hcl:"frequency_penalty,optional"`
	PresencePenalty  *float64 `hcl:"presence_penalty,optional"`

	Latest    bool  `hcl:"latest,optional"`
	Ignore    bool  `hcl:"ignore,optional"`
	Embedding bool  `hcl:"embedding,optional"`
	Images    *bool `hcl:"images,optional"`
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