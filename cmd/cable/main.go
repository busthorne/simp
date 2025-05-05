package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hashicorp/cli"
)

func main() {
	flag.BoolVar(&G.Yes, "y", false, "Assume yes to all prompts")
	flag.BoolVar(&G.V, "v", false, "Enable verbose output")
	flag.BoolVar(&G.VV, "vv", false, "Enable verbose debug output")
	flag.BoolVar(&G.VVV, "vvv", false, "Enable verbose tracing output")
	flag.Parse()

	register := func(c cli.Command) func() (cli.Command, error) {
		return func() (cli.Command, error) {
			return c, nil
		}
	}
	c := &cli.CLI{
		Name:        "cable",
		Version:     "0.0.1",
		Args:        flag.Args(),
		HelpFunc:    Help,
		HelpWriter:  os.Stdout,
		ErrorWriter: os.Stderr,

		Commands: map[string]cli.CommandFactory{
			"ps":   register(&Ps{}),
			"run":  register(&Run{}),
			"rm":   register(&Rm{}),
			"logs": register(&Logs{}),
		},
	}

	exitStatus, err := c.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing CLI: %s\n", err)
	}

	if exitStatus == 127 {
		fmt.Println("Available global flags:")
		flag.PrintDefaults()
		fmt.Println()
	}

	os.Exit(exitStatus)
}
