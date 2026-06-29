package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/danieljustus/symaira-memory/internal/db"
)

// OutputFormatter handles formatting command output as JSON or human-readable text.
type OutputFormatter struct {
	Format          string // "json" or "text"
	Writer          io.Writer
	IncludeEmbedding bool  // when true, include raw embedding vectors in JSON output
}

// NewOutputFormatter creates an OutputFormatter with the given format and os.Stdout as writer.
func NewOutputFormatter(format string) *OutputFormatter {
	return &OutputFormatter{Format: format, Writer: os.Stdout}
}

// Output dispatches to FormatJSON or FormatText based on the configured format.
// For text format, a default template is selected based on the templateName hint
// ("list", "search", "get").
func (f *OutputFormatter) Output(data interface{}, templateName string) error {
	if f.Format == "json" {
		return f.FormatJSON(data)
	}
	tmpl, ok := defaultTextTemplates[templateName]
	if !ok {
		return fmt.Errorf("unknown text template: %s", templateName)
	}
	return f.FormatText(data, tmpl)
}

// FormatJSON writes data as indented JSON to the writer.
// When IncludeEmbedding is false, raw embedding vectors are stripped from the output.
func (f *OutputFormatter) FormatJSON(data interface{}) error {
	output := data
	if !f.IncludeEmbedding {
		output = stripEmbedding(data)
	}
	bytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	_, err = fmt.Fprintln(f.Writer, string(bytes))
	return err
}

// stripEmbedding removes embedding vectors from Memory and SearchResult types
// to produce compact JSON output suitable for agent workflows.
func stripEmbedding(data interface{}) interface{} {
	switch v := data.(type) {
	case *db.Memory:
		clone := *v
		clone.Embedding = nil
		return &clone
	case db.Memory:
		clone := v
		clone.Embedding = nil
		return clone
	case *db.SearchResult:
		clone := *v
		if clone.Memory != nil {
			memClone := *clone.Memory
			memClone.Embedding = nil
			clone.Memory = &memClone
		}
		return &clone
	case db.SearchResult:
		clone := v
		if clone.Memory != nil {
			memClone := *clone.Memory
			memClone.Embedding = nil
			clone.Memory = &memClone
		}
		return clone
	case []*db.Memory:
		out := make([]*db.Memory, len(v))
		for i, m := range v {
			clone := *m
			clone.Embedding = nil
			out[i] = &clone
		}
		return out
	case []db.Memory:
		out := make([]db.Memory, len(v))
		for i, m := range v {
			clone := m
			clone.Embedding = nil
			out[i] = clone
		}
		return out
	case []db.SearchResult:
		out := make([]db.SearchResult, len(v))
		for i, r := range v {
			clone := r
			if clone.Memory != nil {
				memClone := *clone.Memory
				memClone.Embedding = nil
				clone.Memory = &memClone
			}
			out[i] = clone
		}
		return out
	case []*db.SearchResult:
		out := make([]*db.SearchResult, len(v))
		for i, r := range v {
			clone := *r
			if clone.Memory != nil {
				memClone := *clone.Memory
				memClone.Embedding = nil
				clone.Memory = &memClone
			}
			out[i] = &clone
		}
		return out
	default:
		return data
	}
}

// FormatText renders data using the provided Go text/template string.
func (f *OutputFormatter) FormatText(data interface{}, tmplStr string) error {
	funcMap := template.FuncMap{
		"truncate": truncateString,
		"join":     strings.Join,
	}
	tmpl, err := template.New("output").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	return tmpl.Execute(f.Writer, data)
}

// truncateString returns the first n characters of s followed by "..." if truncated.
func truncateString(n int, s string) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var defaultTextTemplates = map[string]string{
	"list": `{{- if not .}}No memories found.
{{- else}}{{- range .}}[{{.ID | truncate 8}}] ({{.Scope}}) {{.Content | truncate 80}}
{{end}}{{- end}}`,

	"search": `{{- if not .}}No relevant memories found.
{{- else}}{{- range .}}[{{.Memory.ID | truncate 8}}] (score: {{printf "%.4f" .Score}}) {{.Memory.Content | truncate 72}}
{{end}}{{- end}}`,

	"get": `ID:        {{.ID}}
Scope:     {{.Scope}}
Content:   {{.Content}}
Created:   {{.CreatedAt.Format "2006-01-02 15:04:05"}}
Updated:   {{.UpdatedAt.Format "2006-01-02 15:04:05"}}
{{- if .Entities}}
Entities:  {{join .Entities ", "}}
{{- end}}
{{- if .CreatedBy}}
Created By: {{.CreatedBy}}
{{- end}}
{{- if .Metadata}}
Metadata:
{{- range $k, $v := .Metadata}}
  {{$k}}: {{$v}}
{{- end}}
{{- end}}
`,

	"entity-list": `{{- if not .}}No entities found.
{{- else}}{{- range .}}[{{.ID | truncate 8}}] {{.Name}} ({{.Type}}) {{.Description | truncate 60}}
{{end}}{{- end}}`,

	"rule-list": `{{- if not .}}No rules found.
{{- else}}{{- range .}}[{{.ID | truncate 8}}] ({{.Scope}}) {{.Content | truncate 80}}
{{end}}{{- end}}`,
}
