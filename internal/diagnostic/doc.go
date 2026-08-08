// Package diagnostic implements the diagnostics engine that translates
// probe failures into actionable AWS troubleshooting hints.
//
// It maps concrete failure signals (for example, "no default route" or
// "subnet not associated") to human-readable guidance such as "Verify
// the IGW is attached" or "Check the Route Table association".
package diagnostic
