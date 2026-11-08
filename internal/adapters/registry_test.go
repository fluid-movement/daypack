package adapters_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
)

func TestDefaultRegistry(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Names(), []string{"claude-code"}) {
		t.Fatal(r.Names())
	}
	a, ok := r.Lookup("claude-code")
	if _, concrete := a.(*claudecode.Adapter); !ok || !concrete {
		t.Fatal("missing Claude adapter")
	}
	wantTargets := a.Targets()
	slices.SortFunc(wantTargets, func(a, b contract.Target) int { return strings.Compare(a.ID, b.ID) })
	if !reflect.DeepEqual(r.Targets(), wantTargets) {
		t.Fatal(r.Targets())
	}
	if !reflect.DeepEqual(adapters.Readers(contract.ClaudeSkillsDir), []string{"claude-code"}) {
		t.Fatal("readers")
	}
}
