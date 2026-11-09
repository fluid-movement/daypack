package adapters_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
)

func TestDefaultRegistry(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Names(), []string{"claude-code", "codex", "pi"}) {
		t.Fatal(r.Names())
	}
	a, ok := r.Lookup("claude-code")
	if _, concrete := a.(*claudecode.Adapter); !ok || !concrete {
		t.Fatal("missing Claude adapter")
	}
	p, ok := r.Lookup("pi")
	if _, concrete := p.(*pi.Adapter); !ok || !concrete {
		t.Fatal("missing Pi adapter")
	}
	c, ok := r.Lookup("codex")
	if !ok {
		t.Fatal("missing Codex adapter")
	}
	wantTargets := append(a.Targets(), p.Targets()...)
	wantTargets = append(wantTargets, c.Targets()...)
	slices.SortFunc(wantTargets, func(a, b contract.Target) int { return strings.Compare(a.ID, b.ID) })
	wantTargets = slices.CompactFunc(wantTargets, func(a, b contract.Target) bool { return a.ID == b.ID })
	if !reflect.DeepEqual(r.Targets(), wantTargets) {
		t.Fatal(r.Targets())
	}
	if !reflect.DeepEqual(adapters.Readers(contract.ClaudeSkillsDir), []string{"claude-code"}) {
		t.Fatal("readers")
	}
}
