package skilldir_test

import (
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
)

func TestTargetValidationAndCopies(t *testing.T) {
	e := contract.Entry{Ref: "skill:testing", Artifact: library.Artifact{Type: library.Skill, Name: "testing", Path: "/nonexistent library/skills/testing"}}
	target := skilldir.AgentsTarget()
	before := skilldir.AgentsTarget()
	items, err := skilldir.Desired(e, target)
	want := []contract.Item{{Target: contract.AgentsSkillsDir, Entry: e.Ref, Kind: contract.Symlink, Path: ".agents/skills/testing", Link: e.Artifact.Path}}
	if err != nil || !reflect.DeepEqual(items, want) || !reflect.DeepEqual(target, before) {
		t.Fatalf("%v %v", items, err)
	}
	target.Types[0] = library.MCP
	if !reflect.DeepEqual(skilldir.AgentsTarget(), before) {
		t.Fatal("target aliases")
	}
	for _, mutate := range []func(*contract.Target){
		func(t *contract.Target) { t.ID = "" },
		func(t *contract.Target) { t.Root = "" },
		func(t *contract.Target) { t.Root = "../escape" },
		func(t *contract.Target) { t.Root = "/absolute" },
		func(t *contract.Target) { t.Mode = contract.FileRoot },
		func(t *contract.Target) { t.Types = nil },
		func(t *contract.Target) { t.Types = []contract.ArtifactType{library.Agent} },
	} {
		target := skilldir.AgentsTarget()
		mutate(&target)
		if _, err := skilldir.Desired(e, target); err == nil {
			t.Fatalf("accepted %#v", target)
		}
	}
	e.Artifact.Type = library.Agent
	if items, err := skilldir.Desired(e, contract.Target{}); err != nil || items != nil {
		t.Fatalf("unsupported: %v %v", items, err)
	}
}
