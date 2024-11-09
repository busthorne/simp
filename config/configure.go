package config

import (
	"bytes"
	"embed"
	"text/template"
)

//go:embed *.hcl.tmpl
var Templates embed.FS

func Configure(templateName string, data any) ([]byte, error) {
	b, err := Templates.ReadFile(templateName + ".hcl.tmpl")
	if err != nil {
		return nil, err
	}
	tmpl := template.Must(template.New(templateName).Parse(string(b)))
	var w bytes.Buffer
	if err := tmpl.Execute(&w, data); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
