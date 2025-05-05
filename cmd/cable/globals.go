package main

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/hashicorp/cli"
)

var G Globals

// Globals holds values for global flags and provides common functionality.
type Globals struct {
	Yes bool // -y
	V   bool // -v
	VV  bool // -vv
	VVV bool // -vvv
}

// Verbose returns the level of verbosity requested (0-3).
func (g Globals) Verbose() int {
	switch {
	case g.VVV:
		return 3
	case g.VV:
		return 2
	case g.V:
		return 1
	default:
		return 0
	}
}

func Help(commands map[string]cli.CommandFactory) string {
	var buf bytes.Buffer
	buf.WriteString("Usage: cable [-globals] command [-flags] [args]\n\n")
	buf.WriteString("Available commands are:\n")

	// Get the list of keys so we can sort them, and also get the maximum
	// key length so they can be aligned properly.
	keys := make([]string, 0, len(commands))
	maxKeyLen := 0
	for key := range commands {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}

		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		commandFunc, ok := commands[key]
		if !ok {
			// This should never happen since we JUST built the list of
			// keys.
			panic("command not found: " + key)
		}

		command, err := commandFunc()
		if err != nil {
			log.Printf("[ERR] cli: Command '%s' failed to load: %s",
				key, err)
			continue
		}

		key = fmt.Sprintf("%s%s", key, strings.Repeat(" ", maxKeyLen-len(key)))
		buf.WriteString(fmt.Sprintf("    %s    %s\n", key, command.Synopsis()))
	}

	return buf.String()
}
