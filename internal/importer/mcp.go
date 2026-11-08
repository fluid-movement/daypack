package importer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/secrets"
)

var bareVariable = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
var variablePunctuation = regexp.MustCompile(`[^A-Z0-9_]`)

func normalizePi(value string) (string, error) {
	if strings.HasPrefix(value, "!") {
		return "", fmt.Errorf("helper-dependent MCP value is unsupported")
	}
	// Pi's escaped dollars are literal and must not become references.
	const escaped = "\x00"
	if strings.Contains(value, escaped) {
		return "", fmt.Errorf("invalid MCP value")
	}
	value = strings.ReplaceAll(value, `\$`, escaped)
	value = bareVariable.ReplaceAllString(value, "$${$1}")
	refs, err := secrets.ParseReferences(value)
	if err != nil {
		return "", fmt.Errorf("unsupported Pi variable expression")
	}
	remainder := value
	for i := len(refs) - 1; i >= 0; i-- {
		r := refs[i]
		remainder = remainder[:r.Start] + remainder[r.End:]
	}
	if strings.Contains(remainder, "$") {
		return "", fmt.Errorf("unsupported Pi variable expression")
	}
	// A literal dollar before '{' cannot be represented by the library grammar.
	value = strings.ReplaceAll(value, escaped, "$")
	if strings.Contains(value, "${") {
		check, e := secrets.ParseReferences(value)
		if e != nil || len(check) != len(refs) {
			return "", fmt.Errorf("escaped reference is unsupported")
		}
	}
	return value, nil
}
func variableName(server, key string) (string, error) {
	name := variablePunctuation.ReplaceAllString(strings.ToUpper(server+"_"+key), "_")
	if name != "" && name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	if !secrets.ValidName(name) {
		return "", fmt.Errorf("invalid secret variable name")
	}
	switch name {
	case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "AWS_BEARER_TOKEN_BEDROCK", "HTTPS_PROXY", "NPM_TOKEN":
		return "", fmt.Errorf("reserved secret variable name")
	}
	return name, nil
}
func prepareMCP(name string, raw json.RawMessage, harness string) (library.MCPDefinition, map[string]string, bool, error) {
	var d library.MCPDefinition
	values := map[string]string{}
	fail := func(reason string) (library.MCPDefinition, map[string]string, bool, error) {
		return library.MCPDefinition{}, nil, false, fmt.Errorf("%s", reason)
	}
	if library.ValidateName(library.MCP, name) != nil {
		return fail("incompatible MCP library name")
	}
	fields, err := object(raw)
	if err != nil {
		return fail("invalid MCP server object")
	}
	lossy := false
	for _, key := range sortedKeys(fields) {
		var target any
		switch key {
		case "description":
			target = &d.Description
		case "type":
			target = &d.Type
		case "command":
			target = &d.Command
		case "args":
			target = &d.Args
		case "url":
			target = &d.URL
		case "env":
			target = &d.Env
		case "headers":
			target = &d.Headers
		case "oauth", "auth", "enabled", "exposure", "timeout":
			if harness != "pi" {
				return fail("unsupported MCP field")
			}
			lossy = true
			continue
		default:
			return fail("unsupported MCP field affecting server behavior")
		}

		// encoding/json otherwise accepts null as a zero string, including inside
		// maps and arrays; executable configuration must have exact field types.
		if key == "env" || key == "headers" {
			entries, e := object(fields[key])
			if e != nil {
				return fail("invalid MCP mapping type")
			}
			for _, entry := range entries {
				var value string
				if string(entry) == "null" || json.Unmarshal(entry, &value) != nil {
					return fail("invalid MCP mapping value type")
				}
			}
		}
		if key == "args" {
			var entries []json.RawMessage
			if json.Unmarshal(fields[key], &entries) != nil {
				return fail("invalid MCP argument array")
			}
			for _, entry := range entries {
				var value string
				if string(entry) == "null" || json.Unmarshal(entry, &value) != nil {
					return fail("invalid MCP argument type")
				}
			}
		}
		if string(fields[key]) == "null" || json.Unmarshal(fields[key], target) != nil {
			return fail("invalid MCP field type")
		}
	}
	if d.Type == "streamable-http" {
		d.Type = "http"
	}
	if d.Type == "" {
		if d.Command != "" {
			d.Type = "stdio"
		} else if harness == "pi" && d.URL != "" {
			d.Type = "http"
		}
	}
	if d.Type != "stdio" && d.Type != "http" {
		return fail("unsupported MCP transport")
	}
	for _, v := range append(append([]string{d.Command, d.URL}, d.Args...), d.Description) {
		if strings.HasPrefix(v, "!") {
			return fail("helper-dependent MCP value is unsupported")
		}
	}
	// Command and description are also screened: neither belongs in a report or
	// git-synced definition when it embeds a credential.
	for _, v := range append([]string{d.Command, d.URL, d.Description}, d.Args...) {
		if library.LooksLikeSecret(v) {
			return fail("secret-looking command, argument, URL or description; value redacted")
		}
	}
	for _, m := range []map[string]string{d.Env, d.Headers} {
		for _, key := range sortedKeys(m) {
			value := m[key]
			if strings.HasPrefix(value, "!") {
				return fail("helper-dependent MCP value is unsupported")
			}
			if harness == "pi" {
				value, err = normalizePi(value)
				if err != nil {
					return fail(err.Error())
				}
				m[key] = value
			}
			refs, e := secrets.ParseReferences(value)
			if e != nil {
				return fail("malformed MCP variable reference")
			}
			if !library.IsSecretKey(key) {
				if library.LooksLikeSecret(value) {
					return fail("secret-looking value under noncredential key; value redacted")
				}
				continue
			}
			if len(refs) > 0 {
				if len(refs) != 1 || refs[0].Start != 0 || refs[0].End != len(value) || refs[0].Default != nil {
					return fail("mixed or defaulted credential reference cannot be safely imported")
				}
				continue
			}
			variable, e := variableName(name, key)
			if e != nil {
				return fail(e.Error())
			}
			if old, ok := values[variable]; ok && old != value {
				return fail("secret variable allocation collision")
			}
			values[variable] = value
			reference, e := secrets.EnvReference(variable)
			if e != nil {
				return fail("invalid credential variable")
			}
			m[key] = reference
		}
	}
	if len(d.Args) == 0 {
		d.Args = nil
	}
	if len(d.Env) == 0 {
		d.Env = nil
	}
	if len(d.Headers) == 0 {
		d.Headers = nil
	}
	// Explicit contradictory fields must not disappear during empty normalization.
	if d.Type == "stdio" {
		if _, ok := fields["url"]; ok {
			return fail("URL is invalid for stdio")
		}
		if _, ok := fields["headers"]; ok {
			return fail("headers are invalid for stdio")
		}
	}
	if d.Type == "http" {
		for _, k := range []string{"command", "args", "env"} {
			if _, ok := fields[k]; ok {
				return fail("stdio field is invalid for HTTP")
			}
		}
	}
	if len(library.ValidateMCP(d, "source")) > 0 {
		return fail("invalid normalized MCP definition")
	}
	return d, values, lossy, nil
}
