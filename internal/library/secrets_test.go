package library

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func secretProject(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func readSecretTest(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestWriteSecretsRoundTrip(t *testing.T) {
	root, project := mutationRoot(t), secretProject(t)
	isolatedGit(t)
	file := filepath.Join(root, "secrets.env")
	values := map[string]string{"Z": "", "A": "spaces ' dollar$ backtick` hash# equals= slash\\ $(touch forbidden)"}
	old := unix.Umask(0)
	err := WriteSecrets(root, project, values)
	unix.Umask(old)
	if err != nil {
		t.Fatal(err)
	}
	data := readSecretTest(t, file)
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("secrets file is not private")
	}
	parsed, err := parseSecrets(data)
	if err != nil || !reflect.DeepEqual(values, parsed) {
		t.Fatal("secrets failed to round trip")
	}
	if !bytes.HasPrefix(data, []byte("A=")) {
		t.Fatal("assignments not sorted")
	}
	cmd := exec.Command("sh", "-c", `. "$1"; printf '%s' "$A"`, "sh", file)
	cmd.Dir = project
	cmd.Env = []string{}
	output, err := cmd.Output()
	if err != nil || string(output) != values["A"] {
		t.Fatal("shell source failed literal round trip")
	}
	if _, err := os.Stat(filepath.Join(project, "forbidden")); !os.IsNotExist(err) {
		t.Fatal("shell evaluated stored value")
	}
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecrets(root, project, values); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(file)
	if err != nil || !after.ModTime().Equal(stamp) || !bytes.Equal(data, readSecretTest(t, file)) {
		t.Fatal("identical write changed file")
	}
	before := []byte("# keep order and bytes\nZ=safe/one=two\nA='it'\\''s literal$`#\\'")
	writeNative(t, file, before)
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecrets(root, project, map[string]string{"Z": "safe/one=two", "B": "new"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readSecretTest(t, file), append(append([]byte{}, before...), []byte("\nB='new'\n")...)) {
		t.Fatal("existing bytes changed")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".daypack-stage-") {
			t.Fatal("staging directory leaked")
		}
	}
}
func TestWriteSecretsRejectsBatch(t *testing.T) {
	for _, tc := range []struct {
		name, existing string
		values         map[string]string
	}{
		{"collision", "A='original'\n", map[string]string{"A": "SENTINEL_SENSITIVE-different", "B": "SENTINEL_SENSITIVE-new"}},
		{"invalid name", "A='original'\n", map[string]string{"BAD;private": "SENTINEL_SENSITIVE"}},
		{"newline", "A='original'\n", map[string]string{"B": "SENTINEL_SENSITIVE\nvalue"}},
		{"carriage", "A='original'\n", map[string]string{"B": "SENTINEL_SENSITIVE\rvalue"}},
		{"nul", "A='original'\n", map[string]string{"B": "SENTINEL_SENSITIVE\x00value"}},
		{"duplicate", "A=SENTINEL_SENSITIVE\nA=SENTINEL_SENSITIVE\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"expansion", "A=$SENTINEL_SENSITIVE\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"command", "A=$(SENTINEL_SENSITIVE)\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"double quote", "A=\"SENTINEL_SENSITIVE\"\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"unclosed", "A='SENTINEL_SENSITIVE\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"export", "export A=SENTINEL_SENSITIVE\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"inline comment", "A=SENTINEL_SENSITIVE # comment\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"CR", "A=SENTINEL_SENSITIVE\r\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
		{"NUL", "A='SENTINEL_SENSITIVE\x00'\n", map[string]string{"B": "SENTINEL_SENSITIVE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, project := mutationRoot(t), secretProject(t)
			isolatedGit(t)
			file := filepath.Join(root, "secrets.env")
			writeNative(t, file, []byte(tc.existing))
			if err := os.Chmod(file, 0600); err != nil {
				t.Fatal(err)
			}
			err := WriteSecrets(root, project, tc.values)
			if err == nil || strings.Contains(err.Error(), "SENTINEL_SENSITIVE") {
				t.Fatal("missing or unsafe validation error")
			}
			if string(readSecretTest(t, file)) != tc.existing {
				t.Fatal("rejected batch changed original")
			}
		})
	}
}
func TestWriteSecretsDestinations(t *testing.T) {
	for _, kind := range []string{"empty", "inside", "equal", "prefix sibling", "symlink", "ancestor symlink", "directory", "loose", "not ignored", "tracked", "missing root", "missing project", "invalid library"} {
		t.Run(kind, func(t *testing.T) {
			root, project := mutationRoot(t), secretProject(t)
			isolatedGit(t)
			values := map[string]string{"A": "SENTINEL_SENSITIVE-sentinel"}
			file := filepath.Join(root, "secrets.env")
			switch kind {
			case "empty":
				values = nil
			case "inside":
				project = filepath.Dir(root)
			case "equal":
				project = root
			case "prefix sibling":
				project = root + "-project"
				if err := os.Mkdir(project, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(project, "target")
				writeNative(t, target, []byte("unchanged"))
				if err := os.Symlink(target, file); err != nil {
					t.Fatal(err)
				}
			case "ancestor symlink":
				alias := filepath.Join(project, "alias")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				root = alias
			case "directory":
				if err := os.Mkdir(file, 0700); err != nil {
					t.Fatal(err)
				}
			case "loose":
				writeNative(t, file, []byte("A='original'\n"))
				if err := os.Chmod(file, 0640); err != nil {
					t.Fatal(err)
				}
			case "not ignored":
				writeNative(t, filepath.Join(root, ".gitignore"), []byte(""))
			case "tracked":
				writeNative(t, file, []byte("A='original'\n"))
				if err := os.Chmod(file, 0600); err != nil {
					t.Fatal(err)
				}
				gitTest(t, root, "init", "--quiet")
				gitTest(t, root, "add", "--force", "--", "secrets.env")
			case "missing root":
				root = filepath.Join(root, "missing")
			case "missing project":
				project = ""
			case "invalid library":
				root = ""
			}
			err := WriteSecrets(root, project, values)
			success := kind == "empty" || kind == "prefix sibling"
			if (err == nil) != success {
				t.Fatalf("unexpected result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "SENTINEL_SENSITIVE-sentinel") {
				t.Fatal("secret leaked")
			}
			if kind == "empty" {
				if _, err := os.Stat(file); !os.IsNotExist(err) {
					t.Fatal("empty batch created file")
				}
			}
			if kind == "loose" || kind == "tracked" {
				if string(readSecretTest(t, file)) != "A='original'\n" {
					t.Fatal("original changed")
				}
			}
			if kind == "symlink" {
				if string(readSecretTest(t, filepath.Join(project, "target"))) != "unchanged" {
					t.Fatal("link target changed")
				}
			}
		})
	}
}

func TestSecretsPublicationFailure(t *testing.T) {
	root := mutationRoot(t)
	isolatedGit(t)
	dest := filepath.Join(root, "secrets.env")
	writeNative(t, dest, []byte("A='original'\n"))
	if err := os.Chmod(dest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishSecrets(filepath.Join(root, "absent-stage"), dest); err == nil {
		t.Fatal("missing stage published")
	}
	if string(readSecretTest(t, dest)) != "A='original'\n" {
		t.Fatal("failed publication changed file")
	}
	// A destination that becomes unsafe after staging must be refused too.
	stage, clean, err := stagePath(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := clean(); err != nil {
			t.Error(err)
		}
	}()
	if err := writePrivateSecrets(stage, "A='SENTINEL_SENSITIVE'\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dest, 0644); err != nil {
		t.Fatal(err)
	}
	if err := publishSecrets(stage, dest); err == nil {
		t.Fatal("unsafe publication accepted")
	}
	if string(readSecretTest(t, dest)) != "A='original'\n" {
		t.Fatal("refused publication changed file")
	}
}

func TestWriteSecretsRestrictiveUmask(t *testing.T) {
	root, project := mutationRoot(t), secretProject(t)
	isolatedGit(t)
	old := unix.Umask(0777)
	err := WriteSecrets(root, project, map[string]string{"TOKEN": "synthetic"})
	unix.Umask(old)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "secrets.env"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("restrictive umask affected file mode")
	}
}
