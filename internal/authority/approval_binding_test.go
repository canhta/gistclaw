package authority

import "testing"

func TestBindingFingerprint_BindsExecutionPlan(t *testing.T) {
	binding := Binding{
		ToolName:   "shell_exec",
		Argv:       []string{"git", "status"},
		CWD:        "/Users/test/Projects/gistclaw",
		ReadRoots:  []string{"/Users/test/Projects"},
		WriteRoots: []string{"/Users/test/Projects/gistclaw"},
		Mutating:   false,
		Network:    false,
		Operands:   []string{"README.md"},
	}

	first := binding.Fingerprint()
	second := binding.Fingerprint()
	if first != second {
		t.Fatalf("Fingerprint() should be deterministic, got %q then %q", first, second)
	}

	changed := binding
	changed.CWD = "/Users/test/Desktop"
	if changed.Fingerprint() == first {
		t.Fatal("expected cwd to change fingerprint")
	}

	changed = binding
	changed.Operands = []string{"go.mod"}
	if changed.Fingerprint() == first {
		t.Fatal("expected operands to change fingerprint")
	}
}
