// Package main is the entry point for the vpc-proof binary.
//
// It delegates entirely to the cli package, which owns the Cobra command
// tree, configuration bootstrap, and structured logging.
package main

import (
	"os"

	"github.com/emanuellcs/vpc-proof-agent/internal/cli"
)

// main runs the CLI and maps any execution error to a non-zero exit code.
func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
