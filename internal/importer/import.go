package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

func destination(root string, kind library.ArtifactType, name string) string {
	switch kind {
	case library.Skill:
		return filepath.Join(root, "skills", name)
	case library.Agent:
		return filepath.Join(root, "agents", name+".md")
	default:
		return filepath.Join(root, "mcp", name+".toml")
	}
}
func target(kind library.ArtifactType) string {
	switch kind {
	case library.Skill:
		return "library-skills"
	case library.Agent:
		return "library-agents"
	default:
		return "library-mcp"
	}
}
func candidateKey(c candidate) string { return string(c.artifact.Type) + ":" + c.artifact.Name }
func sourceKey(s Source) string       { return s.Harness + "\x00" + s.Path + "\x00" + s.Key }

// Import performs a complete read-only preflight before committing independent
// groups. It never mutates harness files, project manifests or project outputs.
func Import(ctx context.Context, opts Options) (Result, error) {
	result := Result{Items: []Item{}}
	if opts.HomeDir == "" {
		return result, fmt.Errorf("home directory is required")
	}
	opts.claudeState = filepath.Join(opts.HomeDir, ".claude.json")
	if opts.ClaudeConfigDir != "" {
		opts.claudeState = filepath.Join(opts.ClaudeConfigDir, ".claude.json")
	}
	if opts.ClaudeConfigDir == "" {
		opts.ClaudeConfigDir = filepath.Join(opts.HomeDir, ".claude")
	}
	if opts.PiAgentDir == "" {
		opts.PiAgentDir = filepath.Join(opts.HomeDir, ".pi", "agent")
	}
	for _, p := range []*string{&opts.claudeState, &opts.HomeDir, &opts.ClaudeConfigDir, &opts.PiAgentDir, &opts.LibraryDir, &opts.ProjectDir} {
		if *p == "" {
			return result, fmt.Errorf("library and project directories are required")
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return result, fmt.Errorf("cannot resolve import directories")
		}
		*p = abs
	}
	lib, err := library.Load(opts.LibraryDir)
	if err != nil {
		return result, fmt.Errorf("cannot load selected library; run daypack library init or repair the library")
	}
	for _, dir := range []string{"skills", "agents", "mcp", "fragments"} {
		info, e := os.Lstat(filepath.Join(opts.LibraryDir, dir))
		if e != nil || !info.IsDir() {
			return result, fmt.Errorf("selected library is not initialized; run daypack library init")
		}
	}
	candidates, err := scan(ctx, opts)
	if err != nil {
		return result, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if candidateKey(a) != candidateKey(b) {
			return candidateKey(a) < candidateKey(b)
		}
		return sourceKey(a.sources[0]) < sourceKey(b.sources[0])
	})
	// Same-name conflicts are decided before content deduplication: no source wins
	// simply because its harness happened to be scanned first.
	names := map[string][]int{}
	for i, c := range candidates {
		names[candidateKey(c)] = append(names[candidateKey(c)], i)
	}
	for _, indices := range names {
		identity := ""
		conflict := false
		for _, i := range indices {
			c := candidates[i]
			if c.reason != "" {
				conflict = true
			}
			if identity == "" {
				identity = c.identity
			} else if identity != c.identity {
				conflict = true
			}
		}
		if conflict && len(indices) > 1 {
			for _, i := range indices {
				candidates[i].reason = "same-name source definitions conflict or cannot be validated"
			}
		}
	}
	// Allocation conflicts affect every server sharing the ambiguous name.
	allocations := map[string]string{}
	collisions := map[string]bool{}
	for _, c := range candidates {
		if c.reason != "" {
			continue
		}
		for k, v := range c.values {
			if old, ok := allocations[k]; ok && old != v {
				collisions[k] = true
			}
			allocations[k] = v
		}
	}
	for i := range candidates {
		c := &candidates[i]
		if c.reason != "" {
			continue
		}
		for k := range c.values {
			if collisions[k] {
				c.reason = "secret variable allocation collision"
			}
		}
		if c.reason == "" {
			if e := library.CheckSecrets(opts.LibraryDir, opts.ProjectDir, c.values); e != nil {
				c.reason = "secret storage preflight failed: " + e.Error()
			}
		}
	}
	// Existing identities are read-only and validated through the library. An
	// occupied invalid path still blocks publication even when Resolve rejects it.
	existing := map[string]library.Artifact{}
	existingPaths := map[string]string{}
	stored, storeErr := library.StoredSecrets(opts.LibraryDir, opts.ProjectDir)
	for _, kind := range []library.ArtifactType{library.Skill, library.Agent, library.MCP} {
		for _, a := range lib.List(kind) {
			if _, e := lib.Resolve(string(kind) + ":" + a.Name); e != nil {
				continue
			}
			identity := ""
			if kind == library.MCP {
				if storeErr != nil && len(a.MCP.RequiredVariables()) > 0 {
					continue
				}
				identity = semanticMCP(*a.MCP, stored)
			} else {
				// Existing sources must be checked too, but selected-library paths are
				// allowed here because they are comparisons, never cleanup candidates.
				compareOpts := opts
				compareOpts.LibraryDir = filepath.Join(opts.LibraryDir, ".comparison-only")
				c, e := readNative(a.Path, kind, compareOpts)
				if e != nil {
					continue
				}
				identity = c.identity
			}
			existingPaths[a.Path] = identity
			key := string(kind) + ":" + identity
			if _, ok := existing[key]; !ok {
				existing[key] = a
			}
		}
	}
	for i := range candidates {
		c := &candidates[i]
		if c.reason != "" {
			continue
		}
		dest := destination(opts.LibraryDir, c.artifact.Type, c.artifact.Name)
		if _, e := os.Lstat(dest); !os.IsNotExist(e) {
			if e != nil || existingPaths[dest] != c.identity {
				c.reason = "library destination occupied by different, invalid or unreadable content"
			}
		}
	}
	groups := []candidate{}
	content := map[string]int{}
	for _, c := range candidates {
		if c.reason == "" {
			key := string(c.artifact.Type) + ":" + c.identity
			if i, ok := content[key]; ok {
				groups[i].sources = append(groups[i].sources, c.sources...)
				groups[i].lossy = groups[i].lossy || c.lossy
				continue
			}
			content[key] = len(groups)
		}
		groups = append(groups, c)
	}
	for _, c := range groups {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		item := Item{Type: c.artifact.Type, Name: c.artifact.Name, Sources: c.sources, Action: plan.Create, Outcome: plan.Planned, Reason: c.reason}
		if library.ValidateName(item.Type, item.Name) == nil {
			item.Destination = destination(opts.LibraryDir, item.Type, item.Name)
		}
		sort.Slice(item.Sources, func(i, j int) bool { return sourceKey(item.Sources[i]) < sourceKey(item.Sources[j]) })
		if item.Reason != "" {
			item.Action = plan.Unsupported
			item.Outcome = plan.Skipped
		} else {
			if a, ok := existing[string(c.artifact.Type)+":"+c.identity]; ok {
				item.Name = a.Name
				item.Destination = a.Path
				item.Outcome = plan.Unchanged
			} else {
				item.Destination = destination(opts.LibraryDir, item.Type, item.Name)
			}
			// Never redirect a differing occupied name into another destination.
			originalDest := destination(opts.LibraryDir, c.artifact.Type, c.artifact.Name)
			if _, e := os.Lstat(originalDest); e == nil && item.Destination != originalDest {
				a, e := lib.Resolve(candidateKey(c))
				if e != nil || a.Path != item.Destination {
					item.Reason = "original library name is occupied by different or invalid content"
				}
			} else if e != nil && !os.IsNotExist(e) {
				item.Reason = "cannot inspect library destination"
			}
			if item.Outcome != plan.Unchanged && item.Reason == "" {
				if _, e := os.Lstat(item.Destination); !os.IsNotExist(e) {
					item.Reason = "library destination already occupied or unreadable"
				}
			}
			if item.Reason != "" {
				item.Action = plan.Conflict
				item.Outcome = plan.Skipped
			} else if !opts.DryRun && item.Outcome != plan.Unchanged {
				if e := commit(ctx, opts, c); e != nil {
					item.Outcome = plan.Failed
					item.Reason = e.Error()
				} else {
					item.Outcome = plan.Applied
				}
			}
			item.SafeToRemove = !opts.DryRun && !c.lossy && (item.Outcome == plan.Applied || item.Outcome == plan.Unchanged)
			if c.lossy && item.Reason == "" {
				item.Reason = "Pi auth or metadata omitted; retain original configuration"
			}
		}
		result.Items = append(result.Items, item)
		result.Plan.Operations = append(result.Plan.Operations, plan.Operation{Target: target(item.Type), Entry: string(item.Type) + ":" + item.Name, Kind: lock.File, Path: item.Destination, Action: item.Action, Outcome: item.Outcome, Reason: item.Reason})
		for _, src := range item.Sources {
			message := strconv.Quote(src.Path)
			if src.Key != "" {
				message += " entry " + strconv.Quote(src.Key)
			}
			message += " -> " + string(item.Type) + ":" + item.Name
			if item.SafeToRemove {
				message += "; original item may be removed manually"
			}
			result.Plan.Notes = append(result.Plan.Notes, contract.Note{Harness: src.Harness, Code: "import-source", Message: message})
		}
	}
	result.Plan, err = plan.Normalize(result.Plan)
	return result, err
}

func commit(ctx context.Context, opts Options, c candidate) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = recheckSources(ctx, opts, c); err != nil {
		return err
	}
	if c.artifact.Type == library.MCP {
		if err = library.WriteSecrets(opts.LibraryDir, opts.ProjectDir, c.values); err != nil {
			return fmt.Errorf("secret storage failed; definition was not published")
		}
		if _, err = library.AddMCP(opts.LibraryDir, c.artifact.Name, *c.artifact.MCP); err != nil {
			return fmt.Errorf("MCP publication failed; original retained")
		}
		return nil
	}
	stage, e := os.MkdirTemp("", "daypack-import-")
	if e != nil {
		return fmt.Errorf("cannot stage native import")
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	stage = canonical(stage)
	source := filepath.Join(stage, "artifact")
	if c.artifact.Type == library.Skill {
		if e = os.Mkdir(source, 0755); e != nil {
			return fmt.Errorf("cannot stage skill")
		}
	}
	for _, f := range c.files {
		if f.directory {
			if e = os.MkdirAll(filepath.Join(source, filepath.FromSlash(f.path)), 0755); e != nil {
				return fmt.Errorf("cannot stage skill directory")
			}
			continue
		}
		file := source
		if c.artifact.Type == library.Skill {
			file = filepath.Join(source, filepath.FromSlash(f.path))
			if e = os.MkdirAll(filepath.Dir(file), 0755); e != nil {
				return fmt.Errorf("cannot stage skill support tree")
			}
		}
		if e = os.WriteFile(file, f.data, os.FileMode(f.mode)); e != nil {
			return fmt.Errorf("cannot stage native file")
		}
		if e = os.Chmod(file, os.FileMode(f.mode)); e != nil {
			return fmt.Errorf("cannot preserve native file permissions")
		}
	}
	if _, e = library.Add(opts.LibraryDir, library.AddOptions{Type: c.artifact.Type, Name: c.artifact.Name, Source: source}); e != nil {
		return fmt.Errorf("native publication failed; original retained")
	}
	return nil
}

// Attention reports discovered items requiring manual attention, including lossy
// imports whose original auth/metadata is intentionally retained.
func (r Result) Attention() bool {
	for _, item := range r.Items {
		if item.Outcome == plan.Skipped || item.Outcome == plan.Failed || strings.Contains(item.Reason, "omitted") {
			return true
		}
	}
	return false
}

func recheckSources(ctx context.Context, opts Options, c candidate) error {
	if c.artifact.Type != library.MCP {
		for _, source := range c.sources {
			current, err := readNative(source.Path, c.artifact.Type, opts)
			if err != nil || current.identity != c.identity {
				return fmt.Errorf("native source changed after preflight")
			}
		}
		return nil
	}
	current, err := scan(ctx, opts)
	if err != nil {
		return fmt.Errorf("cannot recheck MCP source")
	}
	identities := map[string]string{}
	for _, v := range current {
		if v.reason == "" {
			for _, src := range v.sources {
				identities[sourceKey(src)] = v.identity
			}
		}
	}
	for _, source := range c.sources {
		if identities[sourceKey(source)] != c.identity {
			return fmt.Errorf("MCP source changed after preflight")
		}
	}
	return nil
}
