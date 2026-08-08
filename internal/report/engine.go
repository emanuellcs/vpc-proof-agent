package report

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"text/template"
)

//go:embed templates/*
var templateFS embed.FS

// Engine renders Data in the supported formats.
type Engine struct {
	markdown *template.Template
	text     *template.Template
}

// New parses the embedded Markdown and plain-text templates.
func New() (*Engine, error) {
	return newEngine(templateFS)
}

// newEngine parses the Markdown and plain-text templates from fsys. It is
// separated from New so the parsing and error paths can be tested.
func newEngine(fsys fs.FS) (*Engine, error) {
	markdown, err := template.ParseFS(fsys, "templates/markdown.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse markdown template: %w", err)
	}
	text, err := template.ParseFS(fsys, "templates/text.txt.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse text template: %w", err)
	}
	return &Engine{markdown: markdown, text: text}, nil
}

// Render produces the report bytes for the given format.
func (e *Engine) Render(data *Data, format Format) ([]byte, error) {
	switch format {
	case FormatJSON:
		encoded, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode report as json: %w", err)
		}
		return append(encoded, '\n'), nil
	case FormatMarkdown:
		return e.renderTemplate(e.markdown, data)
	case FormatText:
		return e.renderTemplate(e.text, data)
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}
}

// Write renders the report and writes it to w.
func (e *Engine) Write(w io.Writer, data *Data, format Format) error {
	rendered, err := e.Render(data, format)
	if err != nil {
		return err
	}
	if _, err := w.Write(rendered); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// WriteFile renders the report and writes it to path, creating or truncating
// the file with 0644 permissions.
func (e *Engine) WriteFile(path string, data *Data, format Format) error {
	// #nosec G302 -- evidence report files are intentionally world-readable (0644).
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer file.Close()

	if err := e.Write(file, data, format); err != nil {
		return fmt.Errorf("write report to %q: %w", path, err)
	}
	return nil
}

// renderTemplate executes a text template against the data.
func (e *Engine) renderTemplate(tmpl *template.Template, data *Data) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render report: %w", err)
	}
	return buf.Bytes(), nil
}
