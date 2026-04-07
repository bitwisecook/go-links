package main

import (
	"html/template"
	"strings"
)

func parseTemplates() *template.Template {
	funcMap := template.FuncMap{
		"contains": strings.Contains,
		"join":     strings.Join,
		"or": func(a, b bool) bool {
			return a || b
		},
		// jsSafe marks a string as trusted JS for template rendering.
		// This is safe because admin endpoints are gated behind admin CIDR auth,
		// and JS snippets are an intentional admin feature.
		"jsSafe": func(s string) template.JS {
			return template.JS(s)
		},
	}
	return template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"),
	)
}
