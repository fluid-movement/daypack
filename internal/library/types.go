package library

import (
	"fmt"
	"regexp"
	"sort"
)

type ArtifactType string

const (
	Skill    ArtifactType = "skill"
	MCP      ArtifactType = "mcp"
	Fragment ArtifactType = "fragment"
	Agent    ArtifactType = "agent"
)

type Artifact struct {
	Type                    ArtifactType
	Name, Path, Description string
	MCP                     *MCPDefinition
}
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Problem struct {
	Severity                   Severity
	File, Field, Message, Hint string
}
type Library struct {
	root      string
	artifacts []Artifact
	problems  []Problem
	index     map[string]Artifact
	invalid   map[string]bool
}
type NotFoundError struct {
	Type          ArtifactType
	Name, Closest string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s:%s not found (closest: %s)", e.Type, e.Name, e.Closest)
}

// ValidationError prevents a discovered but invalid artifact from being used.
type ValidationError struct{ Problems []Problem }

func (e *ValidationError) Error() string {
	return "artifact has validation errors; inspect library validation problems"
}

var namePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func validType(t ArtifactType) bool { return t == Skill || t == MCP || t == Fragment || t == Agent }
func (l *Library) List(kind ArtifactType) []Artifact {
	out := []Artifact{}
	for _, a := range l.artifacts {
		if a.Type == kind {
			out = append(out, cloneArtifact(a))
		}
	}
	return out
}
func cloneArtifact(a Artifact) Artifact {
	if a.MCP != nil {
		d := *a.MCP
		d.Args = append([]string(nil), d.Args...)
		d.Env = cloneMap(d.Env)
		d.Headers = cloneMap(d.Headers)
		d.present = cloneMap(d.present)
		a.MCP = &d
	}
	return a
}
func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	n := make(map[K]V, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}
func (l *Library) Validate() []Problem { return append([]Problem{}, l.problems...) }
func sortProblems(p []Problem) {
	sort.SliceStable(p, func(i, j int) bool {
		a, b := p[i], p[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
}
func problem(file, field, message, hint string) Problem {
	return Problem{Error, file, field, message, hint}
}
