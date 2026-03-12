package diff

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// renderTemplate executes a Go template string with the given variables.
func renderTemplate(content string, vars map[string]interface{}) (string, error) {
	tmpl, err := template.New("diff").Funcs(templateFuncs()).Option("missingkey=zero").Parse(content)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"camelCase":  func(s string) string { return s },
		"snakeCase":  func(s string) string { return s },
		"pascalCase": func(s string) string { return s },
		"kebabCase":  func(s string) string { return s },
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"title":      cases.Title(language.Und).String,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"join":       strings.Join,
		"split":      strings.Split,
		"now":        time.Now,
		"date": func(layout string, t time.Time) string {
			return t.Format(layout)
		},
	}
}
