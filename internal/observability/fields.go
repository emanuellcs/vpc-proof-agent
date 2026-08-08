package observability

import (
	"time"

	"go.uber.org/zap"
)

// Str builds a string field.
func Str(key, value string) Field {
	return zap.String(key, value)
}

// Int builds an int field.
func Int(key string, value int) Field {
	return zap.Int(key, value)
}

// Bool builds a boolean field.
func Bool(key string, value bool) Field {
	return zap.Bool(key, value)
}

// Duration builds a duration field.
func Duration(key string, value time.Duration) Field {
	return zap.Duration(key, value)
}

// Any builds a field for an arbitrary value. Prefer the typed constructors
// in hot paths; Any uses reflection.
func Any(key string, value any) Field {
	return zap.Any(key, value)
}

// Error builds an error field.
func Error(err error) Field {
	return zap.Error(err)
}

// Component builds a field tagging the logging component (for example
// "cli", "api", or "probe").
func Component(name string) Field {
	return zap.String("component", name)
}
