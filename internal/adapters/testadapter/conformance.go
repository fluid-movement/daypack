package testadapter

import (
	"reflect"
	"slices"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
)

type Case struct {
	Entry     contract.Entry
	Project   contract.Project
	Supported bool
}

// CheckAdapter checks the memory-only part of conformance. Apply/removal,
// handwritten content preservation and secret leakage require the future engine
// and concrete feature packages; these fixtures do not claim to test disk I/O.
func CheckAdapter(t *testing.T, a contract.Adapter, cases []Case) {
	t.Helper()
	for _, c := range cases {
		before := contract.CloneEntry(c.Entry)
		p := c.Project
		p.Harnesses = slices.Clone(c.Project.Harnesses)
		if a.Supports(c.Entry.Artifact.Type) != c.Supported {
			t.Fatal("capability mismatch")
		}
		if !c.Supported {
			continue
		}
		first, err := a.Desired(c.Entry, c.Project)
		if err != nil {
			t.Fatal(err)
		}
		second, err := a.Desired(c.Entry, c.Project)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatal("Desired is not deterministic")
		}
		if !reflect.DeepEqual(before, c.Entry) || !reflect.DeepEqual(p, c.Project) {
			t.Fatal("adapter mutated input")
		}
		for _, item := range first {
			found := false
			for _, target := range a.Targets() {
				if target.ID == item.Target && target.Contains(item.Path) && slices.Contains(target.Types, c.Entry.Artifact.Type) {
					found = true
				}
			}
			if !found || item.Entry != c.Entry.Ref {
				t.Fatal("item does not conform to declared target/entry")
			}
		}
	}
}
