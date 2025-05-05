package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Run handles creating and running a new cable
type Run struct{}

func (c *Run) Help() string {
	helpText := `
Usage: cable run [options] IMAGE [COMMAND] [ARG...]

  Create and run a new cable from an image.

Options:
  -d, --detach      Run cable in background and print cable ID
  -p, --publish=[]  Publish a cable's port(s) to the host (e.g., 8080:80)
  --name=string   Assign a name to the cable
` // Add more flags as needed
	return strings.TrimSpace(helpText)
}

func (c *Run) Synopsis() string {
	return "Create and run a new cable"
}

func (c *Run) Run(args []string) int {
	var detach bool
	var name string
	// You might need a custom flag type for port mappings, or parse strings
	var ports string // Example: use cmdFlags.String or a custom flag.Value

	cmdFlags := flag.NewFlagSet("run", flag.ContinueOnError)
	// Define flags specific to 'run'
	cmdFlags.BoolVar(&detach, "detach", false, "Run cable in background")
	cmdFlags.BoolVar(&detach, "d", false, "Run cable in background (shorthand)")
	cmdFlags.StringVar(&name, "name", "", "Assign a name to the cable")
	cmdFlags.StringVar(&ports, "publish", "", "Publish ports (e.g., 8080:80,443:443)") // Simplified for example
	cmdFlags.StringVar(&ports, "p", "", "Publish ports (shorthand)")
	// Add other flags here

	if err := cmdFlags.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing run flags: %v\n", err)
		return 1
	}

	// After flags, remaining args are IMAGE [COMMAND] [ARG...]
	remainingArgs := cmdFlags.Args()
	if len(remainingArgs) < 1 {
		fmt.Fprintln(os.Stderr, "Error: IMAGE argument is required.")
		fmt.Println(c.Help())
		return 1
	}

	image := remainingArgs[0]
	command := []string{}
	if len(remainingArgs) > 1 {
		command = remainingArgs[1:]
	}

	fmt.Printf("Running cable from image '%s'...\n", image)
	if name != "" {
		fmt.Printf("Name: %s\n", name)
	}
	if detach {
		fmt.Println("Running detached.")
	}
	if ports != "" {
		fmt.Printf("Publishing ports: %s\n", ports)
	}
	if len(command) > 0 {
		fmt.Printf("Command: %s\n", strings.Join(command, " "))
	}

	// TODO: Implement logic to create and run the cable via simp daemon
	// Use c.Globals for config like SimpHost

	fmt.Println("Cable cbl-xyz789 created and running.")

	return 0 // Success
}
