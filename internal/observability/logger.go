// Package observability provides structured logging and observability
// primitives for the vpc-proof agent.
//
// It exposes a hardened Logger built on top of go.uber.org/zap. Business
// code depends only on this package (never on zap directly), which keeps
// the logging backend swappable and enforces a single security boundary:
// every field is filtered so that sensitive keys (tokens, passwords,
// secrets, credentials, API keys) can never reach the log output.
package observability

import (
	"fmt"
	"io"
	"regexp"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field is a structured logging key/value pair.
type Field = zap.Field

// sensitiveKeyPattern matches field keys whose values must never be logged.
var sensitiveKeyPattern = regexp.MustCompile(
	`(?i)(token|secret|password|passwd|credential|authorization|authorisation|api[_-]?key|access[_-]?key|private[_-]?key|cookie)`,
)

// redacted is the placeholder written in place of sensitive values.
const redacted = "[REDACTED]"

// Logger is a thin, hardened wrapper around a zap logger.
//
// All instances returned by the methods of Logger share the same underlying
// core but carry their own context fields.
type Logger struct {
	zap *zap.Logger
}

// New initializes a Logger writing to out at the given level and format.
//
// level is one of debug, info, warn, or error; format is either "json" or
// "console". An invalid level or format returns a descriptive error.
func New(level, format string, out io.Writer) (*Logger, error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("observability: invalid log level %q", level)
	}

	var encoder zapcore.Encoder
	switch format {
	case "json":
		encoder = zapcore.NewJSONEncoder(jsonEncoderConfig())
	case "console":
		encoder = zapcore.NewConsoleEncoder(consoleEncoderConfig())
	default:
		return nil, fmt.Errorf("observability: unsupported log format %q (expected json or console)", format)
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(out), lvl)
	return &Logger{zap: zap.New(core)}, nil
}

// Named returns a child Logger carrying the given logger name.
func (l *Logger) Named(name string) *Logger {
	return &Logger{zap: l.zap.Named(name)}
}

// With returns a child Logger carrying the given context fields.
//
// Sensitive fields are redacted before being attached.
func (l *Logger) With(fields ...Field) *Logger {
	return &Logger{zap: l.zap.With(sanitize(fields)...)}
}

// Debug logs a message at debug level.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.zap.Debug(msg, sanitize(fields)...)
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, fields ...Field) {
	l.zap.Info(msg, sanitize(fields)...)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.zap.Warn(msg, sanitize(fields)...)
}

// Error logs a message at error level.
func (l *Logger) Error(msg string, fields ...Field) {
	l.zap.Error(msg, sanitize(fields)...)
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// sanitize returns fields with every sensitive value replaced by a
// redacted placeholder. Non-sensitive fields are returned untouched.
func sanitize(fields []Field) []Field {
	if len(fields) == 0 {
		return fields
	}
	out := make([]Field, len(fields))
	for i, f := range fields {
		if isSensitiveKey(f.Key) {
			out[i] = zap.String(f.Key, redacted)
			continue
		}
		out[i] = f
	}
	return out
}

// isSensitiveKey reports whether a field key carries sensitive data.
func isSensitiveKey(key string) bool {
	return sensitiveKeyPattern.MatchString(key)
}

func jsonEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}
}

func consoleEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return cfg
}
