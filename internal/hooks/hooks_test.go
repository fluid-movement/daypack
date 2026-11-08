package hooks_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fluid-movement/daypack/internal/hooks"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func hookFile(t *testing.T, dir string) string {
	t.Helper()
	p := runGit(t, dir, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	return filepath.Join(p, "post-checkout")
}

func install(t *testing.T, dir, exe string) hooks.Result {
	t.Helper()
	res, err := hooks.Install(context.Background(), dir, exe)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestInstallIdempotentAndUninstall(t *testing.T) {
	f := testutil.NewFixture(t, false)
	exe := "/opt/daypack/bin/daypack"
	res := install(t, f.Project, exe)
	if !res.Changed || res.Conflict || res.Path != hookFile(t, f.Project) {
		t.Fatalf("unexpected result %+v", res)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/bin/sh\n# daypack post-checkout hook v1 (managed by 'daypack hook install'; remove with 'daypack hook uninstall')\n'/opt/daypack/bin/daypack' apply --auto </dev/null || :\nexit 0\n"
	if string(data) != want {
		t.Fatalf("script = %q", data)
	}
	info := stat(t, res.Path)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(res.Path, old, old); err != nil {
		t.Fatal(err)
	}
	again := install(t, f.Project, exe)
	if again.Changed || again.Conflict {
		t.Fatalf("repeat install %+v", again)
	}
	after := stat(t, res.Path)
	if after.ModTime().Sub(old) > time.Second {
		t.Fatal("idempotent install touched the hook")
	}
	// A moved binary still leaves the owned script removable and replaceable.
	moved := install(t, f.Project, "/elsewhere/daypack")
	if !moved.Changed || moved.Conflict {
		t.Fatalf("owned reinstall %+v", moved)
	}
	un, err := hooks.Uninstall(context.Background(), f.Project)
	if err != nil || !un.Changed || un.Conflict {
		t.Fatalf("uninstall %+v %v", un, err)
	}
	if _, err := os.Lstat(res.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("hook still present")
	}
	un, err = hooks.Uninstall(context.Background(), f.Project)
	if err != nil || un.Changed || un.Conflict {
		t.Fatalf("missing uninstall %+v %v", un, err)
	}
}

func TestForeignHooksUntouched(t *testing.T) {
	owned := string([]byte("#!/bin/sh\n# daypack post-checkout hook v1 (managed by 'daypack hook install'; remove with 'daypack hook uninstall')\n'/x/daypack' apply --auto </dev/null || :\nexit 0\n"))
	for name, tc := range map[string]struct {
		body string
		mode os.FileMode
		kind string
	}{
		"foreign":       {"#!/bin/sh\necho mine\n", 0o755, "file"},
		"nonexecutable": {"echo mine\n", 0o644, "file"},
		"edited":        {owned + "echo extra\n", 0o755, "file"},
		"marker only":   {"#!/bin/sh\n# daypack post-checkout hook v1 (managed by 'daypack hook install'; remove with 'daypack hook uninstall')\necho evil\n", 0o755, "file"},
		"bad quoting":   {strings.Replace(owned, "'/x/daypack'", "'/x/day'pack'", 1), 0o755, "file"},
		"dangling":      {"", 0, "dangling"},
		"directory":     {"", 0, "dir"},
	} {
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			path := hookFile(t, f.Project)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			switch tc.kind {
			case "file":
				if err := os.WriteFile(path, []byte(tc.body), tc.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, tc.mode); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path); err != nil {
					t.Fatal(err)
				}
			case "dir":
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			before := testutil.SnapshotTree(t, filepath.Dir(path))
			res := install(t, f.Project, "/abs/daypack")
			if !res.Conflict || res.Changed || res.Invocation != "( '/abs/daypack' apply --auto </dev/null || : )" {
				t.Fatalf("install %+v", res)
			}
			un, err := hooks.Uninstall(context.Background(), f.Project)
			if err != nil || !un.Conflict || un.Changed {
				t.Fatalf("uninstall %+v %v", un, err)
			}
			testutil.AssertTree(t, filepath.Dir(path), before)
			if tc.kind == "file" {
				info := stat(t, path)
				if info.Mode().Perm() != tc.mode {
					t.Fatalf("mode changed to %v", info.Mode())
				}
			}
		})
	}
}

func TestInvalidExecutableRejected(t *testing.T) {
	f := testutil.NewFixture(t, false)
	for _, exe := range []string{"relative/daypack", "/a\nb", "/a\rb", "/a\x00b", ""} {
		if _, err := hooks.Install(context.Background(), f.Project, exe); err == nil {
			t.Fatalf("accepted %q", exe)
		}
	}
	if _, err := os.Lstat(hookFile(t, f.Project)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("hook written for invalid path")
	}
}

func TestNotGitRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := hooks.Install(context.Background(), dir, "/abs/daypack"); err == nil {
		t.Fatal("install outside git succeeded")
	}
	if _, err := hooks.Uninstall(context.Background(), dir); err == nil {
		t.Fatal("uninstall outside git succeeded")
	}
}

func TestHookDiscovery(t *testing.T) {
	t.Run("subdirectory", func(t *testing.T) {
		f := testutil.NewFixture(t, false)
		sub := filepath.Join(f.Project, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		res := install(t, sub, "/abs/daypack")
		if res.Path != hookFile(t, f.Project) {
			t.Fatalf("path %s", res.Path)
		}
	})
	t.Run("linked worktree", func(t *testing.T) {
		f := testutil.NewFixture(t, true)
		res := install(t, f.Worktree, "/abs/daypack")
		if res.Path != hookFile(t, f.Worktree) || !res.Changed {
			t.Fatalf("result %+v", res)
		}
		if _, err := os.Stat(res.Path); err != nil {
			t.Fatal(err)
		}
		// Same hooks directory is shared by the main checkout.
		if res2 := install(t, f.Project, "/abs/daypack"); res2.Changed || res2.Path != res.Path {
			t.Fatalf("main checkout %+v", res2)
		}
	})
	for _, relative := range []bool{true, false} {
		name := "absolute core.hooksPath"
		if relative {
			name = "relative core.hooksPath"
		}
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			value := filepath.Join(t.TempDir(), "custom hooks")
			if relative {
				value = ".githooks"
			}
			runGit(t, f.Project, "config", "core.hooksPath", value)
			configBefore, err := os.ReadFile(filepath.Join(f.Project, ".git/config"))
			if err != nil {
				t.Fatal(err)
			}
			sub := filepath.Join(f.Project, "deep")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			res := install(t, sub, "/abs/daypack")
			want := value
			if relative {
				resolved := evalLinks(t, f.Project)
				want = filepath.Join(resolved, ".githooks")
			}
			gotDir := evalLinks(t, filepath.Dir(res.Path))
			wantDir := evalLinks(t, want)
			if gotDir != wantDir || filepath.Base(res.Path) != "post-checkout" {
				t.Fatalf("path %s, want dir %s", res.Path, want)
			}
			if _, err := os.Stat(res.Path); err != nil {
				t.Fatal(err)
			}
			configAfter := readFile(t, filepath.Join(f.Project, ".git/config"))
			if string(configBefore) != string(configAfter) {
				t.Fatal("git config edited")
			}
			if un, err := hooks.Uninstall(context.Background(), sub); err != nil || !un.Changed {
				t.Fatalf("uninstall %+v %v", un, err)
			}
		})
	}
}

func TestInstallFailureLeavesNoPartialFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	f := testutil.NewFixture(t, false)
	dir := filepath.Dir(hookFile(t, f.Project))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Error(err)
		}
	})
	if _, err := hooks.Install(context.Background(), f.Project, "/abs/daypack"); err == nil {
		t.Fatal("write failure not reported")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial files left: %v", entries)
	}
}

func TestHookExecutionQuotingAndExit(t *testing.T) {
	f := testutil.NewFixture(t, false)
	for _, name := range []string{"plain", "sp ace", "apos'trophe", "dollar$HOME", "back`touch pwned`tick", "mix ' $(touch pwned) `x` \"q\""} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(t.TempDir(), "record")
			exe := filepath.Join(dir, "daypack bin")
			stub := "#!/bin/sh\nprintf '%s|%s|' \"$PWD\" \"$*\" > '" + record + "'\ncat | wc -c >> '" + record + "'\nexit 7\n"
			if err := os.WriteFile(exe, []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			runHook(t, f, exe, record)
			if _, err := os.Lstat(filepath.Join(dir, "pwned")); err == nil {
				t.Fatal("shell interpolation executed")
			}
			if _, err := os.Lstat(filepath.Join(f.Project, "pwned")); err == nil {
				t.Fatal("shell interpolation executed")
			}
		})
	}
	t.Run("missing binary", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "gone")
		runHook(t, f, exe, "")
	})
}

func runHook(t *testing.T, f testutil.Fixture, exe, record string) {
	t.Helper()
	res := install(t, f.Project, exe)
	cmd := exec.Command("/bin/sh", res.Path)
	cmd.Dir = f.Project
	cmd.Stdin = strings.NewReader("must not be read\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must exit 0: %v\n%s", err, out)
	}
	if record == "" {
		return
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(data)), "|")
	got := strings.Join(parts, "|")
	resolved := evalLinks(t, f.Project)
	cwd := evalLinks(t, parts[0])
	if len(parts) != 3 || cwd != resolved || parts[1] != "apply --auto" || strings.TrimSpace(parts[2]) != "0" {
		t.Fatalf("stub saw %q (cwd %s, args apply --auto, empty stdin expected)", got, f.Project)
	}
	if _, err := hooks.Uninstall(context.Background(), f.Project); err != nil {
		t.Fatal(err)
	}
}

func TestNoSecretInScript(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	res := install(t, f.Project, "/abs/daypack")
	data := readFile(t, res.Path)
	testutil.AssertNoSecret(t, f.Project, sentinel, string(data), res.Invocation)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func evalLinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
