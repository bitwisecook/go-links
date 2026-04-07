package main

import (
	"html/template"
	"strings"
)

func parseTemplates() *template.Template {
	funcMap := template.FuncMap{
		"contains": strings.Contains,
		"join":     strings.Join,
		"jsSafe": func(s string) template.JS {
			return template.JS(s)
		},
	}
	return template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"),
	)
}
