package locations

import "testing"

func TestRegistry_ResolvesNamedRoots(t *testing.T) {
	registry := NewRegistry("/Users/test", "/Users/test/.gistclaw", "/Users/test/Projects/gistclaw", map[string]string{
		RootProjects: "/Users/test/Projects",
	})

	tests := []struct {
		name string
		root string
		want string
	}{
		{name: "home", root: RootHome, want: "/Users/test"},
		{name: "projects", root: RootProjects, want: "/Users/test/Projects"},
		{name: "desktop", root: RootDesktop, want: "/Users/test/Desktop"},
		{name: "documents", root: RootDocuments, want: "/Users/test/Documents"},
		{name: "downloads", root: RootDownloads, want: "/Users/test/Downloads"},
		{name: "storage", root: RootStorage, want: "/Users/test/.gistclaw"},
		{name: "primary path", root: RootPrimaryPath, want: "/Users/test/Projects/gistclaw"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := registry.Resolve(tt.root)
			if !ok {
				t.Fatalf("expected root %q to resolve", tt.root)
			}
			if got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.root, got, tt.want)
			}
		})
	}
}
