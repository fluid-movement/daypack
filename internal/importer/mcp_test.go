package importer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPiReferences(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		bad      bool
	}{
		{`$TOKEN`, `${TOKEN}`, false}, {`${TOKEN}`, `${TOKEN}`, false}, {`prefix-$TOKEN`, `prefix-${TOKEN}`, false},
		{`\$TOKEN`, `$TOKEN`, false}, {`\${TOKEN}`, ``, true}, {`$(helper)`, ``, true}, {`$1`, ``, true}, {`${TOKEN?bad}`, ``, true}, {`!helper`, ``, true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizePi(tc.in)
			if (err != nil) != tc.bad || !tc.bad && got != tc.want {
				t.Fatalf("normalization: %q, error %v", got, err)
			}
		})
	}
}
func TestSecretVariableNames(t *testing.T) {
	for _, tc := range []struct {
		server, key, want string
		bad               bool
	}{
		{"server", "X-Api.Token", "SERVER_X_API_TOKEN", false}, {"1server", "API_TOKEN", "_1SERVER_API_TOKEN", false},
		{"anthropic", "API_KEY", "", true}, {"anthropic", "AUTH_TOKEN", "", true}, {"aws", "BEARER_TOKEN_BEDROCK", "", true}, {"https", "PROXY", "", true}, {"npm", "TOKEN", "", true},
	} {
		t.Run(tc.server+tc.key, func(t *testing.T) {
			got, err := variableName(tc.server, tc.key)
			if (err != nil) != tc.bad || !tc.bad && got != tc.want {
				t.Fatalf("name %q, error %v", got, err)
			}
		})
	}
}
func TestMCPStrictTypes(t *testing.T) {
	for _, raw := range []string{`{"command":null}`, `{"command":"echo","env":{"TOKEN":null}}`, `{"command":"echo","args":[null]}`, `{"command":"echo","env":[]}`, `{"url":"https://example.invalid","command":"echo"}`, `{"command":"echo","url":""}`} {
		_, _, _, err := prepareMCP("server", json.RawMessage(raw), "claude-code")
		if err == nil {
			t.Fatal("invalid source accepted", raw)
		}
	}
}
func TestMCPNormalizationAndReferencePreservation(t *testing.T) {
	d, batch, _, err := prepareMCP("1server", json.RawMessage(`{"command":"echo","env":{"X-Api.Token":"Bearer original","KEEP_TOKEN":"${PRESERVE}"}}`), "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d.Env["X-Api.Token"] != "${_1SERVER_X_API_TOKEN}" || batch["_1SERVER_X_API_TOKEN"] != "Bearer original" || d.Env["KEEP_TOKEN"] != "${PRESERVE}" {
		t.Fatal("credential normalization changed semantics")
	}
	// Decoder errors must never carry source values.
	_, _, _, err = prepareMCP("server", json.RawMessage(`{"env":"SENTINEL"}`), "claude-code")
	if err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal("unsafe error")
	}
}
