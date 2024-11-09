package driver

import (
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/config"
)

var Drivers = []string{"openai", "anthropic", "dify"}

func ProvideModel(alias string) (simp.Driver, config.Model, error) {
	return simp.Drivers["anthropic"], config.Model{Name: "claude-3-5-sonnet-latest"}, nil
}
