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

func TestBindingSummaryJSON_PrefersConcreteOperandsThenFallbacks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "uses first operand",
			raw:  `{"tool_name":"write_new_file","operands":["/tmp/demo.txt"],"cwd":"/tmp","mutating":true}`,
			want: "/tmp/demo.txt",
		},
		{
			name: "falls back to cwd",
			raw:  `{"tool_name":"apply_patch","cwd":"/tmp/repo","mutating":true}`,
			want: "/tmp/repo",
		},
		{
			name: "falls back to write roots",
			raw:  `{"tool_name":"shell_exec","write_roots":["/tmp/repo"],"mutating":true}`,
			want: "/tmp/repo",
		},
		{
			name: "falls back to read roots",
			raw:  `{"tool_name":"run_tests","read_roots":["/tmp/repo"]}`,
			want: "/tmp/repo",
		},
		{
			name: "invalid json returns empty",
			raw:  `{`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BindingSummaryJSON([]byte(tt.raw)); got != tt.want {
				t.Fatalf("BindingSummaryJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}
