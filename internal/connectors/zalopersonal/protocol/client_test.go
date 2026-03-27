package protocol

import (
	"io"
	"strings"
	"testing"
)

func TestMakeURL(t *testing.T) {
	t.Parallel()

	sess := &Session{Language: "vi", IMEI: "test-imei", UserAgent: DefaultUserAgent}

	t.Run("includes defaults when requested", func(t *testing.T) {
		t.Parallel()

		got := makeURL(sess, "https://api.zalo.me/path", map[string]any{"foo": "bar"}, true)
		for _, want := range []string{"foo=bar", "zpw_ver=", "zpw_type="} {
			if !strings.Contains(got, want) {
				t.Fatalf("expected %q in URL %q", want, got)
			}
		}
	})

	t.Run("preserves existing params", func(t *testing.T) {
		t.Parallel()

		got := makeURL(sess, "https://api.zalo.me/path?foo=existing", map[string]any{"foo": "new"}, false)
		if strings.Contains(got, "foo=new") {
			t.Fatalf("expected existing param to win, got %q", got)
		}
		if !strings.Contains(got, "foo=existing") {
			t.Fatalf("expected existing param in URL, got %q", got)
		}
	})
}

func TestBuildFormBody(t *testing.T) {
	t.Parallel()

	body := buildFormBody(map[string]string{"key": "value", "foo": "bar"})
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	got := string(data)
	for _, want := range []string{"key=value", "foo=bar"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in form body %q", want, got)
		}
	}
}
