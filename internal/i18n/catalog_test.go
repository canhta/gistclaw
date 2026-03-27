package i18n

import "testing"

func TestResolveLocale(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		hint string
		want Locale
	}{
		{name: "empty defaults to english", hint: "", want: LocaleEnglish},
		{name: "english stays english", hint: "en", want: LocaleEnglish},
		{name: "english region normalizes", hint: "en-US", want: LocaleEnglish},
		{name: "vietnamese region normalizes", hint: "vi-VN", want: LocaleVietnamese},
		{name: "unsupported falls back", hint: "fr-CA", want: LocaleEnglish},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveLocale(tc.hint); got != tc.want {
				t.Fatalf("ResolveLocale(%q) = %q, want %q", tc.hint, got, tc.want)
			}
		})
	}
}

func TestCatalogFormatUsesLocaleAndFallback(t *testing.T) {
	t.Parallel()

	if got := DefaultCatalog.Format("vi-VN", MessageApprovalPromptTitleWithTool, map[string]string{
		"tool_name": "shell_exec",
	}); got != "Cần phê duyệt cho shell_exec" {
		t.Fatalf("expected Vietnamese approval title, got %q", got)
	}

	if got := DefaultCatalog.Format("vi", MessageApprovalButtonApprove, nil); got != "Phê duyệt" {
		t.Fatalf("expected Vietnamese approve label, got %q", got)
	}

	if got := DefaultCatalog.Format("fr-CA", MessageApprovalButtonDeny, nil); got != "Deny" {
		t.Fatalf("expected English fallback deny label, got %q", got)
	}
}
