package main

import (
	"testing"

	"github.com/busthorne/simp/config"
)

func TestHistory(t *testing.T) {
	tests := []struct {
		name     string
		history  *config.History
		wd       string
		expected string
	}{
		{
			name: "simple path match",
			history: &config.History{
				Location: "/hist",
				Paths: []config.HistoryPath{
					{Path: "/home/user/projects/*", Group: "projects/*"},
				},
			},
			wd:       "/home/user/projects/myproject",
			expected: "/hist/projects/myproject",
		},
		{
			name: "deep wildcard match",
			history: &config.History{
				Location: "/hist",
				Paths: []config.HistoryPath{
					{Path: "/src/**/service/*", Group: "services/*"},
				},
			},
			wd:       "/src/github.com/org/service/auth",
			expected: "/hist/services/auth",
		},
		{
			name: "ignore path",
			history: &config.History{
				Location: "/hist",
				Paths: []config.HistoryPath{
					{Path: "/tmp/*", Group: "temp/*", Ignore: true},
				},
			},
			wd:       "/tmp/work",
			expected: "",
		},
		{
			name: "longest match wins",
			history: &config.History{
				Location: "/hist",
				Paths: []config.HistoryPath{
					{Path: "/code/*", Group: "general/*"},
					{Path: "/code/go/*", Group: "golang/*"},
				},
			},
			wd:       "/code/go/myproject",
			expected: "/hist/golang/myproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate to compile the regexps
			if err := tt.history.Validate(); err != nil {
				t.Fatalf("invalid history config: %v", err)
			}

			got := history(tt.history, tt.wd)
			if got != tt.expected {
				t.Errorf("history() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExpandGroup(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		hp       config.HistoryPath
		expected string
	}{
		{
			name: "single wildcard",
			path: "/home/user/projects/myapp",
			hp: config.HistoryPath{
				Path:  "/home/user/projects/*",
				Group: "projects/*",
			},
			expected: "projects/myapp",
		},
		{
			name: "multiple wildcards",
			path: "/org/repo/service",
			hp: config.HistoryPath{
				Path:  "/org/*/service",
				Group: "services/*",
			},
			expected: "services/repo",
		},
		{
			name: "no wildcards",
			path: "/static/path",
			hp: config.HistoryPath{
				Path:  "/static/path",
				Group: "static",
			},
			expected: "static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the regexp
			if glob, err := config.Glob(tt.hp.Path); err != nil {
				t.Fatalf("invalid glob in path %s: %v", tt.hp.Path, err)
			} else {
				tt.hp.Glob = glob
			}

			got := expandGroup(tt.path, tt.hp)
			if got != tt.expected {
				t.Errorf("expandGroup() = %v, want %v", got, tt.expected)
			}
		})
	}
}
