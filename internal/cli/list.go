package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/spf13/cobra"
)

const listTypesHint = "types: skill, mcp, fragment, agent"

func listCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list [type]", Short: "List library artifacts and their use in this project", Args: cobra.MaximumNArgs(1)}
	SetHandler(cmd, func(_ context.Context, opts Options, args []string) (Result, error) {
		var filter library.ArtifactType
		if len(args) == 1 {
			var ok bool
			if filter, ok = parseListType(args[0]); !ok {
				return Result{}, &Failure{Code: Usage, Cause: fmt.Errorf("unknown artifact type %q", args[0]), Hint: listTypesHint}
			}
		}
		lib, err := library.Load(opts.LibraryDir)
		if err != nil {
			return Result{}, err
		}
		var m *manifest.Manifest
		var problems []manifest.Problem
		if dir, err := manifest.FindProject(opts.ProjectDir); err == nil {
			path := filepath.Join(dir, "daypack.toml")
			m, err = manifest.LoadManifest(path)
			if err != nil {
				problems = append(problems, manifest.Problem{Severity: library.Error, File: path, Message: err.Error()})
			} else {
				problems = m.Validate(lib)
			}
		}
		items := []manifest.UsageEntry{}
		for _, e := range manifest.Catalog(lib, m) {
			if filter == "" || e.Type == filter {
				items = append(items, e)
			}
		}
		result := Result{Items: items, Message: listText(items)}
		for _, p := range problems {
			if p.Severity == library.Error {
				result.Code = Attention
			}
		}
		if len(problems) > 0 {
			result.Problems = problems
		}
		return result, nil
	})
	return cmd
}

func parseListType(s string) (library.ArtifactType, bool) {
	for _, t := range manifest.ArtifactTypes {
		if s == string(t) || s == string(t)+"s" {
			return t, true
		}
	}
	return "", false
}

func listText(items []manifest.UsageEntry) string {
	var b strings.Builder
	var current library.ArtifactType
	for _, e := range items {
		if e.Type != current {
			if current != "" {
				b.WriteString("\n")
			}
			current = e.Type
			fmt.Fprintf(&b, "%s\n", current)
		}
		mark := "[ ]"
		if e.Used {
			mark = "[x]"
		}
		line := fmt.Sprintf("  %s %s", mark, oneLine(e.Name))
		if e.Description != "" {
			line += "  " + oneLine(e.Description)
		}
		if e.Source == manifest.SourceProject {
			line += "  (project-local)"
		}
		if e.Used {
			line += "  (" + strings.Join(e.Harnesses, ", ") + ")"
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
