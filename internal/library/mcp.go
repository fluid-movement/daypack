package library

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/secrets"
)

type MCPDefinition struct {
	Description string            `toml:"description"`
	Type        string            `toml:"type"`
	Command     string            `toml:"command"`
	Args        []string          `toml:"args"`
	URL         string            `toml:"url"`
	Env         map[string]string `toml:"env"`
	Headers     map[string]string `toml:"headers"`
	present     map[string]bool
}

func ParseMCP(data []byte, source string) (MCPDefinition, []Problem) {
	var d MCPDefinition
	var raw map[string]toml.Primitive
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		field := "toml"
		if e, ok := err.(toml.ParseError); ok && e.LastKey != "" {
			field = e.LastKey
		}
		return d, []Problem{problem(source, field, "invalid MCP TOML syntax or field type", "")}
	}
	d.present = map[string]bool{}
	p := []Problem{}
	fields := map[string]any{"description": &d.Description, "type": &d.Type, "command": &d.Command, "args": &d.Args, "url": &d.URL, "env": &d.Env, "headers": &d.Headers}
	for key, value := range raw {
		d.present[key] = true
		destination, ok := fields[key]
		if !ok {
			p = append(p, problem(source, key, "unknown MCP field", ""))
		} else if err := md.PrimitiveDecode(value, destination); err != nil {
			// Parser errors may contain input values; report only the field we decoded.
			p = append(p, problem(source, key, "invalid MCP field type", ""))
		}
	}
	p = append(p, ValidateMCP(d, source)...)
	sortProblems(p)
	return d, p
}

var secretKey = regexp.MustCompile(`(?i)(key|token|secret|password|passwd|auth|credential)`)

func (d MCPDefinition) fields() map[string]string {
	m := map[string]string{"command": d.Command, "url": d.URL}
	for i, v := range d.Args {
		m[fmt.Sprintf("args[%d]", i)] = v
	}
	for k, v := range d.Env {
		m["env."+k] = v
	}
	for k, v := range d.Headers {
		m["headers."+k] = v
	}
	return m
}
func (d MCPDefinition) RequiredVariables() []string {
	set := map[string]bool{}
	for _, v := range d.fields() {
		refs, err := secrets.ParseReferences(v)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			if ref.Default == nil {
				set[ref.Name] = true
			}
		}
	}
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func ValidateMCP(d MCPDefinition, source string) []Problem {
	p := []Problem{}
	add := func(f, m, h string) { p = append(p, problem(source, f, m, h)) }
	presence := func(f string) bool {
		if d.present[f] {
			return true
		}
		switch f {
		case "command":
			return d.Command != ""
		case "url":
			return d.URL != ""
		case "args":
			return d.Args != nil
		case "env":
			return d.Env != nil
		case "headers":
			return d.Headers != nil
		}
		return false
	}
	var forbidden []string
	switch d.Type {
	case "stdio":
		if strings.TrimSpace(d.Command) == "" {
			add("command", "stdio requires a non-empty command", "")
		}
		forbidden = []string{"url", "headers"}
	case "http":
		if strings.TrimSpace(d.URL) == "" {
			add("url", "http requires a non-empty url", "")
		}
		forbidden = []string{"command", "args", "env"}
	default:
		add("type", "type must be stdio or http", "")
	}
	for _, f := range forbidden {
		if presence(f) {
			add(f, "field is not valid for declared MCP type", "")
		}
	}
	for field, value := range d.fields() {
		matches, err := secrets.ParseReferences(value)
		if err != nil {
			add(field, "malformed variable reference", "use ${VAR} or ${VAR:-default}")
		}
		if strings.HasPrefix(field, "env.") || strings.HasPrefix(field, "headers.") {
			_, key, _ := strings.Cut(field, ".")
			if secretKey.MatchString(key) && len(matches) == 0 {
				add(field, "literal value under secret-looking key", "use an environment variable reference such as ${VAR}")
			}
		}
		if (field == "url" || strings.HasPrefix(field, "args[")) && suspicious(value) {
			p = append(p, Problem{Warning, source, field, "value may contain a secret", "move the value into an environment variable and reference it with ${VAR}"})
		}
	}
	sortProblems(p)
	return p
}

var tokenPattern = regexp.MustCompile(`[A-Za-z0-9_+/=.-]+`)

func suspicious(value string) bool {
	clean := value
	refs, err := secrets.ParseReferences(value)
	if err == nil {
		var b strings.Builder
		start := 0
		for _, ref := range refs {
			b.WriteString(value[start:ref.Start])
			start = ref.End
		}
		b.WriteString(value[start:])
		clean = b.String()
	}
	if strings.Contains(clean, "ghp_") || strings.Contains(clean, "sk-") || strings.Contains(clean, "xox") {
		return true
	}
	// Strip references before URL parsing so reference spans never count as secrets.
	if u, err := url.Parse(clean); err == nil {
		for k, values := range u.Query() {
			switch strings.ToLower(k) {
			case "key", "token", "secret", "password":
				for _, v := range values {
					if v != "" {
						return true
					}
				}
			}
		}
	}
	// Heuristic only: tokens >=32 characters with Shannon entropy >=4.5 bits/character.
	for _, token := range tokenPattern.FindAllString(clean, -1) {
		if len(token) < 32 {
			continue
		}
		counts := map[rune]int{}
		for _, c := range token {
			counts[c]++
		}
		entropy := 0.0
		for _, n := range counts {
			p := float64(n) / float64(len(token))
			entropy -= p * math.Log2(p)
		}
		if entropy >= 4.5 {
			return true
		}
	}
	return false
}

// IsSecretKey shares the native credential-key rule with importers.
func IsSecretKey(key string) bool { return secretKey.MatchString(key) }

// LooksLikeSecret shares native suspicious-value checks with importers.
func LooksLikeSecret(value string) bool { return suspicious(value) }
