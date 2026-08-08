// Package main is the entry point for the vpc-proof binary.
//
// Commit 1 scaffolds the executable only; the Cobra command tree that
// will expose the CLI surface is introduced in a subsequent commit.
package main

import "fmt"

// version identifies the build of the vpc-proof binary.
const version = "0.1.0"

// main prints the current scaffold version and exits successfully.
func main() {
	fmt.Printf("vpc-proof %s (repository scaffold)\n", version)
}
