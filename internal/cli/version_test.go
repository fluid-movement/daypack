package cli

import (
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	environment(t)
	old := version
	t.Cleanup(func() { version = old })

	version = "v0.0.0-test"
	code, out, _ := invoke(t, []string{"version"}, false, nil)
	if code != Success || strings.TrimSpace(out) != "daypack v0.0.0-test" {
		t.Fatalf("code %d output %q", code, out)
	}

	code, out, _ = invoke(t, []string{"version", "--json"}, false, nil)
	value := object(t, out, code, "daypack version")
	item, ok := value["items"].(map[string]any)
	if !ok || item["version"] != "v0.0.0-test" {
		t.Fatalf("unexpected JSON: %#v", value)
	}

	version = "dev"
	code, out, _ = invoke(t, []string{"version"}, false, nil)
	if code != Success || !strings.HasPrefix(out, "daypack ") || strings.TrimSpace(out) == "daypack" {
		t.Fatalf("code %d output %q", code, out)
	}
}
