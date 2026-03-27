package execution

import (
	"testing"

	"github.com/canhta/gistclaw/internal/locations"
	"github.com/canhta/gistclaw/internal/model"
)

func TestResolver_ResolveCWDPriority(t *testing.T) {
	registry := locations.NewRegistry("/Users/test", "/Users/test/.gistclaw", "/Users/test/Projects/gistclaw", map[string]string{
		locations.RootProjects: "/Users/test/Projects",
	})
	resolver := NewResolver(registry)

	tests := []struct {
		name    string
		request Request
		project model.Project
		want    string
	}{
		{
			name:    "explicit path wins",
			request: Request{ExplicitPath: "/tmp/manual"},
			project: model.Project{PrimaryPath: "/Users/test/Projects/gistclaw", RootsJSON: `["projects"]`},
			want:    "/tmp/manual",
		},
		{
			name:    "sticky cwd wins over project",
			request: Request{StickyCWD: "/Users/test/Desktop/scratch"},
			project: model.Project{PrimaryPath: "/Users/test/Projects/gistclaw", RootsJSON: `["projects"]`},
			want:    "/Users/test/Desktop/scratch",
		},
		{
			name:    "project primary path wins over named roots",
			request: Request{},
			project: model.Project{PrimaryPath: "/Users/test/Projects/gistclaw", RootsJSON: `["desktop","projects"]`},
			want:    "/Users/test/Projects/gistclaw",
		},
		{
			name:    "named root wins when project path missing",
			request: Request{},
			project: model.Project{RootsJSON: `["projects"]`},
			want:    "/Users/test/Projects",
		},
		{
			name:    "home is final fallback",
			request: Request{},
			project: model.Project{},
			want:    "/Users/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := resolver.Resolve(tt.request, tt.project)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if target.CWD != tt.want {
				t.Fatalf("Resolve().CWD = %q, want %q", target.CWD, tt.want)
			}
		})
	}
}
