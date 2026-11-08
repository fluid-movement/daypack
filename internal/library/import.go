package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ValidateNative validates an existing regular native source without staging it.
func ValidateNative(source string, kind ArtifactType, name string) (Artifact, error) {
	if err := ValidateName(kind, name); err != nil {
		return Artifact{}, err
	}
	return validateArtifact(source, kind, name)
}

// InspectNative identifies native frontmatter and applies the library validator.
// It also supports a materialized SKILL.md that was originally a symlink.
func InspectNative(data []byte, kind ArtifactType, fallback string) (Artifact, error) {
	meta, _ := parseMetadata(data, "source", true)
	name := meta.Name
	if name == "" {
		name = fallback
	}
	if err := ValidateName(kind, name); err != nil {
		return Artifact{}, err
	}
	description, problems := metadata(data, "source", kind, name)
	for _, p := range problems {
		if p.Severity == Error {
			return Artifact{}, fmt.Errorf("invalid native metadata")
		}
	}
	return Artifact{Type: kind, Name: name, Description: description}, nil
}

// AddMCP validates and exclusively publishes an already sanitized definition.
func AddMCP(root, name string, definition MCPDefinition) (_ Artifact, err error) {
	var data bytes.Buffer
	fields := map[string]any{"type": definition.Type}
	if definition.Description != "" {
		fields["description"] = definition.Description
	}
	if definition.Type == "stdio" {
		fields["command"] = definition.Command
		if len(definition.Args) > 0 {
			fields["args"] = definition.Args
		}
		if len(definition.Env) > 0 {
			fields["env"] = definition.Env
		}
	} else {
		fields["url"] = definition.URL
		if len(definition.Headers) > 0 {
			fields["headers"] = definition.Headers
		}
	}
	if problems := ValidateMCP(definition, "source"); len(problems) > 0 {
		return Artifact{}, fmt.Errorf("invalid MCP definition")
	}
	if err = toml.NewEncoder(&data).Encode(fields); err != nil {
		return Artifact{}, fmt.Errorf("encode MCP definition")
	}
	_, problems := ParseMCP(data.Bytes(), "source")
	for _, p := range problems {
		if p.Severity == Error {
			return Artifact{}, fmt.Errorf("invalid MCP definition")
		}
	}
	dest, err := nativePath(root, MCP, name)
	if err != nil {
		return Artifact{}, err
	}
	if err = absent(dest); err != nil {
		return Artifact{}, err
	}
	stage, clean, err := stagePath(dest)
	if err != nil {
		return Artifact{}, err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if err = os.WriteFile(stage, data.Bytes(), 0644); err != nil {
		return Artifact{}, err
	}
	a, err := validateArtifact(stage, MCP, name)
	if err != nil {
		return Artifact{}, err
	}
	if err = publish(stage, dest); err != nil {
		return Artifact{}, err
	}
	a.Path = dest
	return a, nil
}
