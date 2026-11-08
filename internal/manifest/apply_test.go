package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

func TestValidateForApplyOnlyDefersMissing(t *testing.T) {
	libDir := t.TempDir()
	lib, err := library.Load(libDir)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Harnesses: []string{"claude-code"}, Skills: []Entry{{Type: library.Skill, Name: "missing"}}}
	if len(m.Validate(lib)) == 0 {
		t.Fatal("ordinary validation must reject unresolved entry")
	}
	if p := m.ValidateForApply(lib); len(p) != 0 {
		t.Fatalf("missing entry should be deferred: %v", p)
	}
	for _, invalid := range []*Manifest{
		{Version: 2, Harnesses: m.Harnesses, Skills: m.Skills},
		{Version: 1, Harnesses: m.Harnesses, Skills: []Entry{{Type: library.Skill, Name: "missing", Harnesses: []string{"pi"}}}},
		{Version: 1, Harnesses: m.Harnesses, Skills: []Entry{{Type: library.Skill, Name: "INVALID"}}},
		{Version: 1, Harnesses: m.Harnesses, Skills: append(m.Skills, m.Skills...)},
	} {
		if len(invalid.ValidateForApply(lib)) == 0 {
			t.Fatal("structural invalidity deferred")
		}
	}
	if err = os.MkdirAll(filepath.Join(libDir, "skills", "broken"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(libDir, "skills", "broken", "SKILL.md"), []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	lib, err = library.Load(libDir)
	if err != nil {
		t.Fatal(err)
	}
	m.Skills = append(m.Skills, Entry{Type: library.Skill, Name: "broken"})
	if len(m.ValidateForApply(lib)) == 0 {
		t.Fatal("invalid artifact deferred as missing")
	}
}
