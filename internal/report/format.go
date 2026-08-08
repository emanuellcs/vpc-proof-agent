package report

import "fmt"

// Format identifies a report output format.
type Format string

// Supported report formats.
const (
	// FormatJSON produces a strict, machine-readable JSON document.
	FormatJSON Format = "json"
	// FormatMarkdown produces a polished human-readable Markdown document.
	FormatMarkdown Format = "markdown"
	// FormatText produces a console-friendly plain-text document.
	FormatText Format = "text"
)

// ParseFormat maps a format name to a Format, rejecting unknown names.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatJSON, FormatMarkdown, FormatText:
		return Format(s), nil
	default:
		return "", fmt.Errorf("unsupported report format %q (expected json, markdown, or text)", s)
	}
}

// String returns the format name.
func (f Format) String() string {
	return string(f)
}
