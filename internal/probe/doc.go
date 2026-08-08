// Package probe implements the core network and metadata validation
// logic of the vpc-proof agent.
//
// Responsibilities include IMDSv2 metadata extraction, IP ownership and
// CIDR consistency checks, default-route and gateway detection, DNS
// resolution, outbound HTTPS connectivity tests, and public IP
// consistency checks against an external echo service.
package probe
