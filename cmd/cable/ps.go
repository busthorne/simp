package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Ps command handles listing running processes.
type Ps struct{}

func (c *Ps) Help() string {
	helpText := `
Usage: cable ps [options]

  List running cables (similar to docker ps).

Options:
  -a, --all   Show all cables (default shows just running)
` // Add more flags as needed
	return strings.TrimSpace(helpText)
}

func (c *Ps) Synopsis() string {
	return "List running cables"
}

func (c *Ps) Run(args []string) int {
	var showAll bool

	cmdFlags := flag.NewFlagSet("ps", flag.ContinueOnError)
	// Define flags specific to 'ps'
	cmdFlags.BoolVar(&showAll, "all", false, "Show all cables (shorthand)")
	cmdFlags.BoolVar(&showAll, "a", false, "Show all cables")
	// Add other flags here

	if err := cmdFlags.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ps flags: %v\n", err)
		return 1
	}

	fmt.Println("Listing cables...")
	if showAll {
		fmt.Println("(Showing all cables)")
	}

	// TODO: Implement logic to fetch cable status from simp daemon
	// Use c.Globals to access global options if needed

	fmt.Println("ID            NAME          IMAGE         STATUS        PORTS")
	fmt.Println("cbl-abc123    my-service    nginx:latest  Running       80/tcp")
	fmt.Println("cbl-def456    db-backup     postgres:15   Exited (0)")

	return 0 // Success
}
