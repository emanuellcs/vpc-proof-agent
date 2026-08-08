// Package metadata provides a reusable, hardened client for fetching
// EC2 instance metadata over IMDSv2.
//
// The client requires a session token (TOKEN request followed by
// authenticated metadata requests) and exposes instance ID, private IP,
// public IP, and availability zone without ever persisting credentials.
package metadata
