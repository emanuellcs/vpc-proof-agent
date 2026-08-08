// Package main is the entry point for the vpc-proof binary.
//
// It delegates entirely to the cli package, which owns the Cobra command
// tree, configuration bootstrap, structured logging, and process exit
// codes.
package main

import (
	"os"

	"github.com/emanuellcs/vpc-proof-agent/internal/cli"
)

// main runs the CLI and propagates its exit code to the process.
func main() {
	os.Exit(cli.Execute())
}
