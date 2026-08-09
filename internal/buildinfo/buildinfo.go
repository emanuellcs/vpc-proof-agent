// Package buildinfo exposes build-time metadata for the vpc-proof binary.
//
// The variables in this package are overridden at build time through
// -ldflags "-X" directives (see the Makefile), allowing the version
// command to report accurate information without runtime discovery.
package buildinfo

// Version is the semantic version of the binary. Overridden at build time.
var Version = "dev"

// Commit is the git commit hash the binary was built from. Overridden at
// build time; "none" when built outside a git checkout.
var Commit = "none"

// BuildDate is the UTC timestamp of the build. Overridden at build time.
var BuildDate = "unknown"

// Developer is the developer attribution shown by the CLI and reports.
// Overridden at build time; defaults to the project's primary developer.
var Developer = "Emanuel Lázaro (emanuellcs)"
