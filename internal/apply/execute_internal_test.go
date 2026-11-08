package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
)

func coreFixture(t *testing.T) (string, *manifest.Manifest, *library.Library, *adapters.Registry) {
	t.Helper()
	root, libDir := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(libDir, "skills", "test"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "skills", "test", "SKILL.md"), []byte("---\nname: test\ndescription: Test\n---\nUse tests.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lib, err := library.Load(libDir)
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Harnesses: []string{"pi"}, Skills: []manifest.Entry{{Type: library.Skill, Name: "test"}}}
	registry, err := adapters.NewRegistry(testadapter.New("pi", true))
	if err != nil {
		t.Fatal(err)
	}
	return root, m, lib, registry
}
func buildCore(t *testing.T, root string, m *manifest.Manifest, lib *library.Library, registry *adapters.Registry) *Reconciliation {
	t.Helper()
	r, err := Build(context.Background(), root, m, lib, registry, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestFailureInjection(t *testing.T) {
	for _, failure := range []string{"publish", "save", "exclude", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			root, m, lib, registry := coreFixture(t)
			r := buildCore(t, root, m, lib, registry)
			injected := errors.New("injected failure")
			ctx := context.Background()
			switch failure {
			case "publish":
				r.fs.link = func(string, string, []string, string) error { return injected }
			case "save":
				r.fs.save = func(*lock.Lock, string) error { return injected }
			case "exclude":
				r.fs.exclude = func(*lock.Lock, string) error { return injected }
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			final, err := r.Execute(ctx, r.Plan())
			if err == nil {
				t.Fatal("expected failure")
			}
			for _, o := range final.Operations {
				if o.Outcome == plan.Planned {
					t.Fatal("untruthful pending outcome")
				}
			}
			owned, err := lock.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if failure != "exclude" && len(owned.Items()) != 0 {
				t.Fatal("failed publication/save claimed ownership")
			}
			if failure == "save" {
				retry := buildCore(t, root, m, lib, registry)
				if retry.Plan().Operations[0].Action != plan.Conflict {
					t.Fatal("matching unowned write must require adoption")
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if e.Name() == ".daypack-link-" {
					t.Fatal("temporary file leaked")
				}
			}
		})
	}
}
func TestApprovalCannotChangePayload(t *testing.T) {
	root, m, lib, registry := coreFixture(t)
	r := buildCore(t, root, m, lib, registry)
	p := r.Plan()
	p.Operations[0].Path = "outside"
	if _, err := r.Execute(context.Background(), p); err == nil {
		t.Fatal("forged approval accepted")
	}
	if len(r.owned.Items()) != 0 {
		t.Fatal("forged approval changed ownership")
	}
	p = r.Plan()
	p.Operations = append(p.Operations, p.Operations[0])
	if _, err := r.Execute(context.Background(), p); err == nil {
		t.Fatal("added approval accepted")
	}
}
func TestPreviewChangesAreSkipped(t *testing.T) {
	root, m, lib, registry := coreFixture(t)
	r := buildCore(t, root, m, lib, registry)
	path := filepath.Join(root, ".agents", "skills", "test")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("handwritten"), 0600); err != nil {
		t.Fatal(err)
	}
	final, err := r.Execute(context.Background(), r.Plan())
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range final.Operations {
		if o.Entry == "skill:test" && o.Outcome != plan.Skipped {
			t.Fatal("changed entry must be skipped")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "handwritten" {
		t.Fatal("preview edit overwritten")
	}
}
func TestExecutorRemovalOrder(t *testing.T) {
	root, m, lib, registry := coreFixture(t)
	r := buildCore(t, root, m, lib, registry)
	if _, err := r.Execute(context.Background(), r.Plan()); err != nil {
		t.Fatal(err)
	}
	// A supported old target remains trusted even after removing its producer.
	m.Skills = nil
	r = buildCore(t, root, m, lib, registry)
	if len(r.Plan().Operations) != 2 {
		t.Fatal("expected exactly old removals")
	}
	if _, err := r.Execute(context.Background(), r.Plan()); err != nil {
		t.Fatal(err)
	}
	owned, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned.Items()) != 0 {
		t.Fatal("removals retained ownership")
	}
}
func TestPathSafety(t *testing.T) {
	for _, attack := range []string{"forged-target", "parent-link", "daypack-link", "lock-link", "directory-leaf", "unknown-target"} {
		t.Run(attack, func(t *testing.T) {
			root, m, lib, registry := coreFixture(t)
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch attack {
			case "forged-target":
				l := lock.New()
				err = l.Put(lock.Item{Target: contract.AgentsSkillsDir, Entry: "skill:test", Kind: lock.File, Path: "sentinel", Hash: lock.HashBytes([]byte("private"))})
				if err == nil {
					err = l.Save(root)
				}
			case "unknown-target":
				l := lock.New()
				err = l.Put(lock.Item{Target: "removed-adapter", Entry: "skill:test", Kind: lock.File, Path: "sentinel", Hash: "unused"})
				if err == nil {
					err = l.Save(root)
				}
			case "parent-link":
				err = os.Symlink(outside, filepath.Join(root, ".agents"))
			case "daypack-link":
				err = os.Symlink(outside, filepath.Join(root, ".daypack"))
			case "lock-link":
				err = os.MkdirAll(filepath.Join(root, ".daypack"), 0700)
				if err == nil {
					err = os.Symlink(sentinel, filepath.Join(root, ".daypack", "lock.toml"))
				}
			case "directory-leaf":
				err = os.MkdirAll(filepath.Join(root, ".agents", "skills", "test"), 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			r, err := Build(context.Background(), root, m, lib, registry, Options{Adopt: true, Force: true})
			if attack == "unknown-target" {
				if err != nil {
					t.Fatal(err)
				}
				p, e := r.Execute(context.Background(), r.Plan())
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, o := range p.Operations {
					if o.Target == "removed-adapter" && o.Outcome == plan.Skipped {
						found = true
					}
				}
				if !found {
					t.Fatal("unknown ownership not reported")
				}
			} else if err == nil {
				t.Fatal("unsafe path accepted")
			}
			b, err := os.ReadFile(sentinel)
			if err != nil || !reflect.DeepEqual(b, []byte("private")) {
				t.Fatal("outside sentinel modified")
			}
		})
	}
}
