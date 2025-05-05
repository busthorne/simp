package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Rm handles removing one or more cables
type Rm struct{}

func (c *Rm) Help() string {
	helpText := `
Usage: cable rm [options] CABLE [CABLE...]

  Remove one or more cables.

Options:
  -f, --force   Force the removal of a running cable
` // Add more flags as needed
	return strings.TrimSpace(helpText)
}

func (c *Rm) Synopsis() string {
	return "Remove one or more cables"
}

func (c *Rm) Run(args []string) int {
	var force bool

	cmdFlags := flag.NewFlagSet("rm", flag.ContinueOnError)
	// Define flags specific to 'rm'
	cmdFlags.BoolVar(&force, "force", false, "Force removal")
	cmdFlags.BoolVar(&force, "f", false, "Force removal (shorthand)")
	// Add other flags here

	if err := cmdFlags.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing rm flags: %v\n", err)
		return 1
	}

	// After flags, remaining args are the CABLE IDs/names
	cableIDs := cmdFlags.Args()
	if len(cableIDs) < 1 {
		fmt.Fprintln(os.Stderr, "Error: requires at least one CABLE argument.")
		fmt.Println(c.Help())
		return 1
	}

	fmt.Printf("Removing cables: %s\n", strings.Join(cableIDs, ", "))
	if force {
		fmt.Println("(Forcing removal)")
	}

	// TODO: Implement logic to remove cables via simp daemon
	// Loop through cableIDs and make requests
	// Use c.Globals for config

	for _, id := range cableIDs {
		fmt.Println(id) // Print removed ID (as Docker does)
	}

	return 0 // Success
}
