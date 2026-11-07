package adapters_test

import (
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
)

func TestAdapterConformance(t *testing.T) {
	for _, a := range []*testadapter.Adapter{testadapter.New("base", false), testadapter.New("pi", false), testadapter.New("third", true)} {
		t.Run(a.Name(), func(t *testing.T) {
			unsupported := entry("agent")
			unsupported.Artifact.Type = library.Agent
			testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: entry("svelte"), Project: contract.Project{Harnesses: []string{a.Name()}}, Supported: true}, {Entry: unsupported, Supported: false}})
		})
	}
}
func TestFromManifest(t *testing.T) {
	m := manifest.Entry{Type: library.MCP, Name: "docs", Harnesses: []string{"pi"}, Inline: &library.MCPDefinition{Env: map[string]string{"TOKEN": "${TOKEN}"}}}
	content := []byte("native")
	e := contract.FromManifest(m, library.Artifact{}, content)
	if e.Ref != "mcp:docs" || !e.Inline {
		t.Fatal(e)
	}
	e.Harnesses[0] = "changed"
	e.Artifact.MCP.Env["TOKEN"] = "changed"
	e.Content[0] = '!'
	if m.Harnesses[0] != "pi" || m.Inline.Env["TOKEN"] != "${TOKEN}" || content[0] != 'n' {
		t.Fatal("manifest alias")
	}
	inherited := contract.FromManifest(manifest.Entry{Type: library.Skill, Name: "x"}, library.Artifact{}, nil)
	if inherited.Harnesses != nil {
		t.Fatal("omission normalized")
	}
}
