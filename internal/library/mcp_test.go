package library

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMCPSchema(t *testing.T) {
	for _, tc := range []struct{ name, body, field string }{
		{"no type", "command='run'", "type"},
		{"bad type", "type='HTTP'", "type"},
		{"missing command", "type='stdio'", "command"},
		{"blank command", "type='stdio'\ncommand='  '", "command"},
		{"missing url", "type='http'", "url"},
		{"stdio url", "type='stdio'\ncommand='run'\nurl=''", "url"},
		{"stdio headers", "type='stdio'\ncommand='run'\nheaders={}", "headers"},
		{"http command", "type='http'\nurl='https://example.test'\ncommand=''", "command"},
		{"http args", "type='http'\nurl='https://example.test'\nargs=[]", "args"},
		{"http env", "type='http'\nurl='https://example.test'\nenv={}", "env"},
		{"unknown", "type='stdio'\ncommand='run'\nrequired_secrets=[]", "required_secrets"},
		{"wrong value type", "type='stdio'\ncommand=4", "command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, p := ParseMCP([]byte(tc.body), "source.toml")
			if !hasProblem(p, tc.field, Error) {
				t.Fatalf("missing error for field %s", tc.field)
			}
			for _, v := range p {
				if v.File != "source.toml" {
					t.Fatal("wrong source")
				}
			}
			if tc.name == "http command" && !hasProblem(ValidateMCP(d, "source.toml"), "command", Error) {
				t.Fatal("field presence lost")
			}
		})
	}
	for _, data := range []string{"type = [", "type='stdio'\ncommand='unterminated"} {
		_, p := ParseMCP([]byte(data), "source.toml")
		if len(p) == 0 || p[0].Severity != Error {
			t.Fatal("malformed TOML accepted")
		}
	}
	for _, body := range []string{"type='stdio'\ncommand='${RUN}'\nargs=['${ARG}']\nenv={ API_BASE='https://x.test', API_KEY='${KEY:-}' }", "type='http'\nurl='${URL}'\nheaders={ Authorization='Bearer ${TOKEN}', Token='${ONE:-default}-${TWO}' }"} {
		_, p := ParseMCP([]byte(body), "valid.toml")
		if len(p) != 0 {
			t.Fatalf("valid references rejected: %v", p)
		}
	}
}
func TestMCPSecretKeys(t *testing.T) {
	for _, key := range []string{"API_KEY", "ToKeN", "secret", "PASSWORD", "passwd", "Authorization", "credentials", "monkey"} {
		for _, value := range []string{"", "sentinel-sensitive-value"} {
			for _, kind := range []string{"stdio", "http"} {
				d := MCPDefinition{Type: kind, Command: "run"}
				field := "env." + key
				if kind == "stdio" {
					d.Env = map[string]string{key: value}
				} else {
					d.Command = ""
					d.URL = "https://x.test"
					d.Headers = map[string]string{key: value}
					field = "headers." + key
				}
				p := ValidateMCP(d, "server.toml")
				if !hasProblem(p, field, Error) {
					t.Fatal("secret-looking literal accepted")
				}
				for _, v := range p {
					if v.File != "server.toml" || !strings.Contains(v.Hint, "${VAR}") {
						t.Fatal("missing secret-reference hint")
					}
				}
				if value != "" && strings.Contains(fmt.Sprint(p), value) {
					t.Fatal("sensitive literal in diagnostic")
				}
			}
		}
	}
	for _, value := range []string{"${TOKEN}", "Bearer ${TOKEN}", "${TOKEN:-default}", "${TOKEN:-}", "${ONE}-${TWO}"} {
		d := MCPDefinition{Type: "http", URL: "https://x.test", Headers: map[string]string{"Authorization": value}}
		if len(ValidateMCP(d, "server")) != 0 {
			t.Fatal("valid reference rejected")
		}
	}
	for _, value := range []string{"${}", "${1BAD}", "${BAD-NAME}", "${UNCLOSED", "${VAR-default}"} {
		d := MCPDefinition{Type: "stdio", Command: "run", Args: []string{value}, Env: map[string]string{"BASE": value}}
		p := ValidateMCP(d, "server")
		if !hasProblem(p, "args[0]", Error) || !hasProblem(p, "env.BASE", Error) {
			t.Fatal("malformed reference accepted")
		}
	}
}
func TestRequiredVariables(t *testing.T) {
	d := MCPDefinition{Command: "${RUN} ${RUN:-fallback}", Args: []string{"${Z} ${A} ${Z}", "${OPTIONAL:-default}", "${EMPTY:-}"}, URL: "https://${HOST}/${A}", Env: map[string]string{"KEY": "${KEY} ${A:-default}"}, Headers: map[string]string{"Authorization": "Bearer ${TOKEN} ${Z:-}"}}
	want := []string{"A", "HOST", "KEY", "RUN", "TOKEN", "Z"}
	if got := d.RequiredVariables(); !reflect.DeepEqual(got, want) {
		t.Fatal("incorrect required variables")
	}
	if got := (MCPDefinition{}).RequiredVariables(); got == nil || len(got) != 0 {
		t.Fatal("empty variables must be empty slice")
	}
}
func TestSecretWarnings(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		warn        bool
	}{
		{"github", "ghp_sentinel", true}, {"api", "sk-sentinel", true}, {"slack", "xoxb-sentinel", true},
		{"query", "https://x.test/path?ToKeN=sentinel", true},
		{"entropy", "abcdefghijklmnopqrstuvwxyzABCDEF0123456789", true},
		{"ordinary", "https://example.test/documentation", false},
		{"low entropy", strings.Repeat("a", 64), false},
		{"short entropy", "aB3dE5gH7jK9mN1p", false},
		{"prefix default", "${TOKEN:-ghp_sentinel}", false},
		{"reference query", "https://x.test?token=${TOKEN}", false},
		{"reference", "${abcdefghijklmnopqrstuvwxyzABCDEF}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, field := range []string{"url", "args[0]"} {
				d := MCPDefinition{Type: "http", URL: tc.value}
				if field == "args[0]" {
					d = MCPDefinition{Type: "stdio", Command: "run", Args: []string{tc.value}}
				}
				p := ValidateMCP(d, "warning.toml")
				if hasProblem(p, field, Warning) != tc.warn {
					t.Fatal("incorrect secret heuristic")
				}
				for _, v := range p {
					if v.Severity == Error {
						t.Fatal("secret suspicion became error")
					}
					if v.File != "warning.toml" || v.Field != field || !strings.Contains(v.Hint, "${VAR}") {
						t.Fatal("missing warning context")
					}
				}
				if tc.warn && strings.Contains(fmt.Sprint(p), tc.value) {
					t.Fatal("suspected secret leaked")
				}
			}
		})
	}
	root := sandbox(t)
	write(t, root, "mcp/server.toml", "type='stdio'\ncommand='run'\nargs=['ghp_sentinel']")
	l := load(t, root)
	if _, err := l.Resolve("mcp:server"); err != nil {
		t.Fatal("warning prevented resolution")
	}
}
func TestMCPParseProblemsRetained(t *testing.T) {
	root := sandbox(t)
	write(t, root, "mcp/bad.toml", "type = [")
	write(t, root, "mcp/also-bad.toml", "type='http'\nurl='https://x.test'\ncommand=''")
	l := load(t, root)
	if len(l.Validate()) < 2 {
		t.Fatal("parse problems dropped")
	}
	for _, ref := range []string{"mcp:bad", "mcp:also-bad"} {
		if _, err := l.Resolve(ref); err == nil {
			t.Fatal("invalid MCP resolved")
		}
	}
}
