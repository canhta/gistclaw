package execution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/locations"
	"github.com/canhta/gistclaw/internal/model"
)

type Resolver struct {
	registry locations.Registry
}

func NewResolver(registry locations.Registry) Resolver {
	return Resolver{registry: registry}
}

func (r Resolver) Resolve(req Request, project model.Project) (Target, error) {
	cwd, err := r.resolveCWD(req, project)
	if err != nil {
		return Target{}, err
	}
	return Target{
		CWD:      cwd,
		ExecHost: ExecHostLocal,
	}, nil
}

func (r Resolver) resolveCWD(req Request, project model.Project) (string, error) {
	if path := strings.TrimSpace(req.ExplicitPath); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(req.StickyCWD); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(project.PrimaryPath); path != "" {
		return path, nil
	}

	var roots []string
	if strings.TrimSpace(project.RootsJSON) != "" {
		if err := json.Unmarshal([]byte(project.RootsJSON), &roots); err != nil {
			return "", fmt.Errorf("execution: decode project roots: %w", err)
		}
	}
	for _, root := range roots {
		path, ok := r.registry.Resolve(root)
		if ok {
			return path, nil
		}
	}

	path, ok := r.registry.Resolve(locations.RootHome)
	if !ok {
		return "", fmt.Errorf("execution: resolve cwd: %s root unavailable", locations.RootHome)
	}
	return path, nil
}
