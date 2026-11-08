package apply_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/lock"
)

func TestInspectionIsIsolatedCopy(t *testing.T) {
	a := testadapter.New("claude-code", false)
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\"]\nskills=[\"testing\"]\n", a)
	x, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	before := x.Plan()
	in := x.Inspection()
	if len(in.Entries) != 1 || len(in.Desired) != 1 || in.Ownership == nil || !reflect.DeepEqual(in.Plan, before) {
		t.Fatalf("unexpected inspection: %+v", in)
	}
	in.Entries[0].Ref = "skill:changed"
	in.Entries[0].Harnesses[0] = "changed"
	in.Desired[0].Path = "changed"
	in.Project.Harnesses[0] = "changed"
	in.Plan.Operations[0].Path = "changed"
	if err = in.Ownership.Put(lock.Item{Target: "t", Entry: "skill:x", Kind: lock.File, Path: "x"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(x.Plan(), before) {
		t.Fatal("inspection mutation changed the plan")
	}
	again := x.Inspection()
	if again.Entries[0].Ref != "skill:testing" || again.Desired[0].Path == "changed" || again.Project.Harnesses[0] != "claude-code" || len(again.Ownership.Items()) != 0 {
		t.Fatalf("inspection mutation leaked: %+v", again)
	}
}
