package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/spf13/cobra"
)

const applyHint = "Run 'daypack apply' to update the project."

type initItem struct {
	Path string `json:"path"`
	From string `json:"from,omitempty"`
}

type entryItem struct {
	Type      library.ArtifactType `json:"type"`
	Name      string               `json:"name"`
	Action    string               `json:"action"`
	Harnesses []string             `json:"harnesses,omitempty"`
}

func initCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "init", Short: "Create daypack.toml", Args: cobra.NoArgs}
	var harnesses []string
	var from string
	cmd.Flags().StringSliceVar(&harnesses, "harness", nil, "Harnesses to target (claude-code, pi; default both)")
	cmd.Flags().StringVar(&from, "from", "", "Copy another project's manifest (directory or daypack.toml)")
	SetHandler(cmd, func(_ context.Context, opts Options, _ []string) (Result, error) {
		if _, err := manifest.NormalizeHarnesses(harnesses); err != nil {
			return Result{}, &Failure{Code: Usage, Cause: err}
		}
		path, err := manifest.Init(opts.ProjectDir, manifest.InitOptions{Harnesses: harnesses, From: from})
		if errors.Is(err, manifest.ErrExists) {
			return Result{}, &Failure{Code: Error, Cause: err, Hint: "daypack.toml already exists; edit it by hand or add entries with daypack add"}
		}
		if err != nil {
			return Result{}, err
		}
		return Result{
			Items:   []initItem{{Path: path, From: from}},
			Message: "Created " + path + "\nNext: add entries with 'daypack add', then run 'daypack apply'.",
		}, nil
	})
	return cmd
}

func parseRef(arg string) (library.ArtifactType, string, error) {
	typ, name, ok := strings.Cut(arg, ":")
	kind := library.ArtifactType(typ)
	if !ok || name == "" {
		return "", "", &Failure{Code: Usage, Cause: fmt.Errorf("invalid reference %q: expected <type>:<name>", arg)}
	}
	if !slices.Contains([]library.ArtifactType{library.Skill, library.MCP, library.Fragment, library.Agent}, kind) {
		return "", "", &Failure{Code: Usage, Cause: fmt.Errorf("unknown artifact type %q", typ), Hint: "Use skill, mcp, fragment or agent."}
	}
	if err := library.ValidateName(kind, name); err != nil {
		return "", "", &Failure{Code: Usage, Cause: err}
	}
	return kind, name, nil
}

// loadManifestFile finds the project manifest and returns its path, bytes and parse.
func loadManifestFile(opts Options) (string, []byte, *manifest.Manifest, error) {
	dir, err := manifest.FindProject(opts.ProjectDir)
	if err != nil {
		return "", nil, nil, &Failure{Code: Error, Cause: err, Hint: "Run 'daypack init' to create a manifest."}
	}
	path := filepath.Join(dir, "daypack.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, err
	}
	m, err := manifest.LoadManifest(path)
	if err != nil {
		return "", nil, nil, err
	}
	return path, data, m, nil
}

func addCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add <type>:<name>", Short: "Add or update a library reference in daypack.toml", Args: cobra.ExactArgs(1)}
	var only []string
	cmd.Flags().StringSliceVar(&only, "only", nil, "Restrict the entry to these harnesses")
	SetHandler(cmd, func(_ context.Context, opts Options, args []string) (Result, error) {
		kind, name, err := parseRef(args[0])
		if err != nil {
			return Result{}, err
		}
		restrict := cmd.Flags().Changed("only")
		if restrict {
			if len(only) == 0 || slices.Contains(only, "") {
				return Result{}, &Failure{Code: Usage, Cause: errors.New("--only must list harnesses"), Hint: "Use claude-code, pi or both."}
			}
			if sorted := slices.Sorted(slices.Values(only)); len(slices.Compact(sorted)) != len(only) {
				return Result{}, &Failure{Code: Usage, Cause: errors.New("--only repeats a harness")}
			}
			for _, h := range only {
				if h != "claude-code" && h != "pi" {
					return Result{}, &Failure{Code: Usage, Cause: fmt.Errorf("invalid or repeated harness %q", h), Hint: "Use claude-code, pi or both."}
				}
			}
		}
		path, data, m, err := loadManifestFile(opts)
		if err != nil {
			return Result{}, err
		}
		if restrict {
			for _, h := range only {
				if !slices.Contains(m.Harnesses, h) {
					return Result{}, &Failure{Code: Error, Cause: fmt.Errorf("harness %q is not enabled in the manifest", h), Hint: "Manifest harnesses: " + strings.Join(m.Harnesses, ", ")}
				}
			}
		}
		lib, err := library.Load(opts.LibraryDir)
		if err != nil {
			return Result{}, err
		}
		if _, err := lib.Resolve(args[0]); err != nil {
			var nf *library.NotFoundError
			if errors.As(err, &nf) {
				f := &Failure{Code: Error, Cause: fmt.Errorf("%s:%s not found in the library", kind, name)}
				if nf.Closest != "" {
					f.Hint = fmt.Sprintf("did you mean %s:%s?", kind, nf.Closest)
				}
				return Result{}, f
			}
			return Result{}, &Failure{Code: Error, Cause: err, Hint: "Run 'daypack library doctor' for details."}
		}
		existing := manifest.HasEntry(m, kind, name)
		if existing && !restrict {
			return entryResult(kind, name, "Already present (unchanged)", nil), nil
		}
		entry := manifest.Entry{Name: name}
		if restrict && (len(only) != len(m.Harnesses) || !allIn(only, m.Harnesses)) {
			entry.Harnesses = slices.Clone(only)
		}
		out, changed, err := manifest.SetEntry(data, kind, entry)
		if err != nil {
			return Result{}, &Failure{Code: Error, Cause: err}
		}
		action := "Added"
		switch {
		case !changed:
			action = "Already present (unchanged)"
		case existing:
			action = "Updated"
		}
		if changed {
			if err := manifest.WriteFile(path, out, fileMode(path)); err != nil {
				return Result{}, err
			}
		}
		return entryResult(kind, name, action, entry.Harnesses), nil
	})
	return cmd
}

func allIn(a, b []string) bool {
	for _, s := range a {
		if !slices.Contains(b, s) {
			return false
		}
	}
	return true
}

func fileMode(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

func entryResult(kind library.ArtifactType, name, action string, harnesses []string) Result {
	return Result{
		Items:   []entryItem{{Type: kind, Name: name, Action: action, Harnesses: harnesses}},
		Message: fmt.Sprintf("%s %s:%s in daypack.toml\n%s", action, kind, name, applyHint),
	}
}

func removeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "remove <type>:<name>", Short: "Remove a library reference from daypack.toml", Args: cobra.ExactArgs(1)}
	SetHandler(cmd, func(_ context.Context, opts Options, args []string) (Result, error) {
		kind, name, err := parseRef(args[0])
		if err != nil {
			return Result{}, err
		}
		path, data, m, err := loadManifestFile(opts)
		if err != nil {
			return Result{}, err
		}
		const hint = "Check daypack.toml; inline MCP servers must be removed by hand."
		if kind == library.MCP && manifest.HasInline(m, name) {
			return Result{}, &Failure{Code: Error, Cause: fmt.Errorf("%s:%s is an inline server", kind, name), Hint: hint}
		}
		if !manifest.HasEntry(m, kind, name) {
			return Result{}, &Failure{Code: Error, Cause: fmt.Errorf("%s:%s is not in daypack.toml", kind, name), Hint: hint}
		}
		out, err := manifest.RemoveEntry(data, kind, name)
		if err != nil {
			return Result{}, &Failure{Code: Error, Cause: err}
		}
		if err := manifest.WriteFile(path, out, fileMode(path)); err != nil {
			return Result{}, err
		}
		return entryResult(kind, name, "Removed", nil), nil
	})
	return cmd
}
