package locations

import (
	"path/filepath"
	"strings"
)

const (
	RootHome        = "home"
	RootProjects    = "projects"
	RootDesktop     = "desktop"
	RootDocuments   = "documents"
	RootDownloads   = "downloads"
	RootStorage     = "storage"
	RootPrimaryPath = "primary_path"
)

type Registry struct {
	roots map[string]string
}

func NewRegistry(homeDir, storageRoot, primaryPath string, extra map[string]string) Registry {
	roots := map[string]string{
		RootHome:      strings.TrimSpace(homeDir),
		RootProjects:  filepath.Join(homeDir, "Projects"),
		RootDesktop:   filepath.Join(homeDir, "Desktop"),
		RootDocuments: filepath.Join(homeDir, "Documents"),
		RootDownloads: filepath.Join(homeDir, "Downloads"),
		RootStorage:   strings.TrimSpace(storageRoot),
	}
	if strings.TrimSpace(primaryPath) != "" {
		roots[RootPrimaryPath] = strings.TrimSpace(primaryPath)
	}
	for name, path := range extra {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		roots[name] = path
	}
	return Registry{roots: roots}
}

func (r Registry) Resolve(name string) (string, bool) {
	if r.roots == nil {
		return "", false
	}
	path, ok := r.roots[name]
	if !ok || strings.TrimSpace(path) == "" {
		return "", false
	}
	return path, true
}
