package library

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInstructionsContent(t *testing.T) {
	root := mutationRoot(t)
	data := string(mustRead(t, filepath.Join(root, "AGENTS.md")))
	for _, text := range []string{"Generated content revision: 1", "--library, DAYPACK_HOME, XDG_CONFIG_HOME/daypack", "HOME/.config/daypack", "skills/<name>/SKILL.md", "mcp/<name>.toml", "fragments/<name>.md", "agents/<name>.md", "library doctor --fix", "library import [--dry-run]", "--from <native-path>", "library edit", "library rename", "library remove", "daypack list", "secrets.env", "does not automatically source dotenv"} {
		if !strings.Contains(data, text) {
			t.Errorf("missing %q", text)
		}
	}
	// Validate the actual embedded examples with the native parsers.
	blocks := strings.Split(data, "~~~")
	counts := map[string]int{}
	for i := 1; i < len(blocks); i += 2 {
		lang, body, _ := strings.Cut(blocks[i], "\n")
		counts[lang]++
		switch lang {
		case "yaml":
			kind, name := Skill, "review"
			if counts[lang] == 2 {
				kind, name = Fragment, "style"
			}
			if _, problems := metadata([]byte(body), "example", kind, name); len(problems) != 0 {
				t.Fatal(problems)
			}
			if kind == Skill {
				if _, problems := metadata([]byte(body), "example", Agent, name); len(problems) != 0 {
					t.Fatal(problems)
				}
			}
		case "toml":
			if _, problems := ParseMCP([]byte(body), "example"); len(problems) != 0 {
				t.Fatal(problems)
			}
		default:
			t.Fatalf("unexpected example language %q", lang)
		}
	}
	if counts["toml"] != 2 || counts["yaml"] != 2 {
		t.Fatal(counts)
	}
	lib, err := Load(root)
	if err != nil || len(lib.Validate()) != 0 {
		t.Fatalf("load: %v %v", lib, err)
	}
	for _, kind := range []ArtifactType{Skill, MCP, Fragment, Agent} {
		if len(lib.List(kind)) != 0 {
			t.Fatal("instructions became an artifact")
		}
	}
}

func TestInstructionsRefresh(t *testing.T) {
	for _, initial := range []string{"", "handwritten", "notes\r\n", "before\r\n" + instructionsBegin + "\nstale revision\n" + instructionsEnd + "\r\nafter without newline"} {
		t.Run(initial, func(t *testing.T) {
			root := mutationRoot(t)
			path := filepath.Join(root, "AGENTS.md")
			writeNative(t, path, []byte(initial))
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			want, err := renderInstructions([]byte(initial))
			if err != nil {
				t.Fatal(err)
			}
			if err := Init(root); err != nil {
				t.Fatal(err)
			}
			if got := mustRead(t, path); !bytes.Equal(got, want) {
				t.Fatal("unexpected refresh")
			}
			if strings.Contains(initial, instructionsBegin) {
				if !bytes.HasPrefix(want, []byte("before\r\n")) || !bytes.HasSuffix(want, []byte("\r\nafter without newline")) {
					t.Fatal("notes changed")
				}
			} else if !bytes.HasPrefix(want, []byte(initial)) {
				t.Fatal("notes changed")
			}
			old := time.Unix(123456789, 0)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
			for _, refresh := range []func(string) error{Init, RefreshInstructions} {
				if err := refresh(root); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if !info.ModTime().Equal(old) || info.Mode().Perm() != 0600 || !bytes.Equal(mustRead(t, path), want) {
					t.Fatal("no-op changed file")
				}
			}
		})
	}
	root := mutationRoot(t)
	path := filepath.Join(root, "AGENTS.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := RefreshInstructions(root); err != nil {
		t.Fatal(err)
	}
	if len(mustRead(t, path)) == 0 {
		t.Fatal("missing guide")
	}
}

func TestInstructionsMalformedMarkers(t *testing.T) {
	for _, data := range []string{
		instructionsBegin, instructionsEnd, instructionsEnd + instructionsBegin,
		instructionsBegin + instructionsBegin + instructionsEnd + instructionsEnd,
		instructionsBegin + instructionsEnd + instructionsBegin + instructionsEnd,
		"<!-- daypack:begin library-instructions -->\n<!-- daypack:end library-instructions",
		"<!-- daypack:begin library-instructions-->" + instructionsEnd,
	} {
		t.Run(data, func(t *testing.T) {
			root := mutationRoot(t)
			path := filepath.Join(root, "AGENTS.md")
			writeNative(t, path, []byte(data))
			before := instructionSnapshot(t, root)
			for _, refresh := range []func(string) error{Init, RefreshInstructions} {
				if err := refresh(root); err == nil || !strings.Contains(err.Error(), "marker") {
					t.Fatalf("expected marker error: %v", err)
				}
				if !reflect.DeepEqual(before, instructionSnapshot(t, root)) {
					t.Fatal("failed refresh changed files")
				}
			}
		})
	}
}

type instructionFile struct {
	Data  string
	Mode  os.FileMode
	Mtime time.Time
}

func instructionSnapshot(t *testing.T, root string) map[string]instructionFile {
	t.Helper()
	result := map[string]instructionFile{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Directory mtimes change when a guide is atomically replaced; snapshot records only.
		if info.Mode().IsRegular() {
			result[path] = instructionFile{string(mustRead(t, path)), info.Mode(), info.ModTime()}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstructionsUntouchedRecords(t *testing.T) {
	root := mutationRoot(t)
	for _, name := range []string{"skills/review/SKILL.md", "skills/review/AGENTS.md", "skills/review/support/data", "mcp/invalid.toml", "fragments/style.md", "agents/review.md", "secrets.env", "unrelated"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		writeNative(t, filepath.Join(root, name), []byte("invalid or private data"))
	}
	before := instructionSnapshot(t, root)
	delete(before, filepath.Join(root, "AGENTS.md"))
	for _, initial := range []string{"stale notes", instructionsBegin + "old" + instructionsEnd, instructionsBegin} {
		writeNative(t, filepath.Join(root, "AGENTS.md"), []byte(initial))
		for _, refresh := range []func(string) error{Init, RefreshInstructions} {
			err := refresh(root)
			if (err != nil) != (initial == instructionsBegin) {
				t.Fatal(err)
			}
			after := instructionSnapshot(t, root)
			delete(after, filepath.Join(root, "AGENTS.md"))
			if !reflect.DeepEqual(before, after) {
				t.Fatal("refresh touched records")
			}
		}
	}
}

func TestInstructionsUnsafePaths(t *testing.T) {
	root := mutationRoot(t)
	outside := filepath.Join(filepath.Dir(root), "outside")
	writeNative(t, outside, []byte("outside sentinel"))
	for _, directory := range []bool{false, true} {
		target := outside
		if directory {
			target = t.TempDir()
			var err error
			target, err = filepath.EvalSymlinks(target)
			if err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(root, "AGENTS.md")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		for _, refresh := range []func(string) error{Init, RefreshInstructions} {
			if err := refresh(root); err == nil {
				t.Fatal("accepted symlink guide")
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		writeNative(t, path, []byte("notes"))
	}
	if string(mustRead(t, outside)) != "outside sentinel" {
		t.Fatal("outside target changed")
	}
	path := filepath.Join(root, "AGENTS.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := RefreshInstructions(root); err == nil {
		t.Fatal("accepted directory guide")
	}
	link := filepath.Join(filepath.Dir(root), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, refresh := range []func(string) error{Init, RefreshInstructions} {
		if err := refresh(link); err == nil {
			t.Fatal("accepted symlink root")
		}
		if err := refresh(filepath.Join(link, "child")); err == nil {
			t.Fatal("accepted symlink ancestor")
		}
	}
	if err := RefreshInstructions(filepath.Join(root, "missing")); err == nil {
		t.Fatal("initialized missing library")
	}
}

func TestInstructionsWriteFailure(t *testing.T) {
	root := mutationRoot(t)
	path := filepath.Join(root, "AGENTS.md")
	writeNative(t, path, []byte("preserve me"))
	before := instructionSnapshot(t, root)
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0755) })
	err := RefreshInstructions(root)
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	if err == nil {
		t.Fatal("expected write failure")
	}
	if !reflect.DeepEqual(before, instructionSnapshot(t, root)) {
		t.Fatal("failed write changed files")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".daypack-stage-") {
			t.Fatal("staging file leaked")
		}
	}
}

func TestInstructionsFIFOAndEmptyRoot(t *testing.T) {
	root := mutationRoot(t)
	path := filepath.Join(root, "AGENTS.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	for _, refresh := range []func(string) error{Init, RefreshInstructions} {
		if err := refresh(root); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("accepted FIFO: %v", err)
		}
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("changed FIFO")
	}
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Init(empty); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(empty)
	if err != nil || len(entries) != 1 || entries[0].Name() != "AGENTS.md" {
		t.Fatalf("populated empty root: %v %v", entries, err)
	}
}
