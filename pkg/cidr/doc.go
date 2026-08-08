// Package cidr provides reusable CIDR mathematics and IP ownership
// helpers.
//
// It supports validation that an IP address belongs to a CIDR block,
// VPC/subnet containment checks, and related netmask operations used by
// the probe package to prove that the instance's private IP falls within
// the intended VPC and subnet ranges.
package cidr
