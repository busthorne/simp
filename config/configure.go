package config

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed *.hcl.tmpl
var Templates embed.FS

func Configure(templateName string, data any) ([]byte, error) {
	b, err := Templates.ReadFile(templateName + ".hcl.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	tmpl, err := template.New(templateName).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var w bytes.Buffer
	if err := tmpl.Execute(&w, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return w.Bytes(), nil
}
