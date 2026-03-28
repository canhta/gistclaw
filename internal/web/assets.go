package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
)

var templateFiles = []string{
	"templates/layout.html",
	"templates/runs.html",
	"templates/run_detail.html",
	"templates/approvals.html",
	"templates/settings.html",
	"templates/team.html",
	"templates/memory.html",
	"templates/sessions.html",
	"templates/routes_deliveries.html",
	"templates/session_detail.html",
}

//go:embed templates static
var webAssets embed.FS

func loadTemplates() (*template.Template, error) {
	tpls, err := template.ParseFS(webAssets, templateFiles...)
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	return tpls, nil
}

func staticAssetFS() fs.FS {
	sub, err := fs.Sub(webAssets, "static")
	if err != nil {
		panic(fmt.Sprintf("web: locate static assets: %v", err))
	}
	return sub
}
