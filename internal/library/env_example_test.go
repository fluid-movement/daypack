package library

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExampleReferencesAndRefresh(t *testing.T) {
	root := mutationRoot(t)
	writeNative(t, filepath.Join(root, "mcp", "stdio.toml"), []byte(`type = "stdio"
command = "${COMMAND:-server}"
args = ["${ARG}", "${SHARED:-DO_NOT_COPY_DEFAULT}"]
[env]
LITERAL_KEY = "${ENV} ${SHARED}"
`))
	writeNative(t, filepath.Join(root, "mcp", "http.toml"), []byte(`type = "http"
url = "https://${HOST:-example.test}/${ARG}"
[headers]
Authorization = "Bearer ${HEADER}"
`))
	example := filepath.Join(root, ".env.example")
	writeNative(t, example, []byte("notes\r\n# daypack:begin env-example\nSTALE=\n# daypack:end env-example\ntrailing"))
	for _, name := range []string{".env", "secrets.env", "fragments/notes.md"} {
		writeNative(t, filepath.Join(root, name), []byte("PRIVATE_SENTINEL"))
	}
	before := instructionSnapshot(t, root)
	delete(before, example)
	delete(before, filepath.Join(root, "AGENTS.md"))
	delete(before, filepath.Join(root, ".gitignore"))
	if p, err := Doctor(root); err != nil || !doctorHasProblem(p, Warning, "stale") {
		t.Fatalf("%v %v", p, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := RefreshHousekeeping(root); err != nil {
			t.Fatal(err)
		}
	}
	got := string(mustRead(t, example))
	want := "notes\r\n# daypack:begin env-example\nARG=\nCOMMAND=\nENV=\nHEADER=\nHOST=\nSHARED=\n# daypack:end env-example\ntrailing"
	if got != want {
		t.Fatalf("example: %q", got)
	}
	after := instructionSnapshot(t, root)
	for path, old := range before {
		if !reflect.DeepEqual(old, after[path]) {
			t.Fatalf("changed private/user record %s", path)
		}
	}
	snapshot := instructionSnapshot(t, root)
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, instructionSnapshot(t, root)) {
		t.Fatal("non-idempotent init")
	}
	if err := os.Remove(filepath.Join(root, "mcp", "stdio.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshHousekeeping(root); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustRead(t, example)), "COMMAND=") {
		t.Fatal("stale variable")
	}
}

func TestExampleInvalidMCPAndUnsafeMarkers(t *testing.T) {
	root := mutationRoot(t)
	example := filepath.Join(root, ".env.example")
	original := mustRead(t, example)
	writeNative(t, filepath.Join(root, "mcp", "bad.toml"), []byte("type = [PRIVATE_SENTINEL"))
	p, err := InitWithProblems(root)
	if err != nil || len(p) == 0 || strings.Contains(p[0].Message, "PRIVATE_SENTINEL") {
		t.Fatalf("%v %v", p, err)
	}
	if string(mustRead(t, example)) != string(original) {
		t.Fatal("overwrote incomplete scan")
	}
	if p, err = Doctor(root); err != nil || !doctorHasProblem(p, Error, "TOML") {
		t.Fatalf("%v %v", p, err)
	}
	if err = os.Remove(filepath.Join(root, "mcp", "bad.toml")); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"# daypack:begin env-example\n", "# daypack:end env-example\n# daypack:begin env-example\n", "# daypack:begin env-example\n# daypack:begin env-example\n# daypack:end env-example\n"} {
		writeNative(t, example, []byte(content))
		before := instructionSnapshot(t, root)
		if _, err = RefreshHousekeeping(root); err == nil {
			t.Fatal("accepted broken markers")
		}
		if !reflect.DeepEqual(before, instructionSnapshot(t, root)) {
			t.Fatal("destructive marker failure")
		}
	}
	if err = os.Remove(example); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside-example")
	writeNative(t, outside, []byte("untouched"))
	if err = os.Symlink(outside, example); err != nil {
		t.Fatal(err)
	}
	if err = Init(root); err == nil {
		t.Fatal("accepted symlink")
	}
	if string(mustRead(t, outside)) != "untouched" {
		t.Fatal("wrote outside")
	}
}

func TestLegacyPrivateStoreSelection(t *testing.T) {
	root := mutationRoot(t)
	project := t.TempDir()
	legacy := filepath.Join(root, "secrets.env")
	canonical := filepath.Join(root, ".env")
	writeNative(t, legacy, []byte("TOKEN='legacy'\n"))
	if err := os.Chmod(legacy, 0600); err != nil {
		t.Fatal(err)
	}
	values, err := StoredSecrets(root, project)
	if err != nil || values["TOKEN"] != "legacy" {
		t.Fatalf("%v %v", values, err)
	}
	if err = WriteSecrets(root, project, map[string]string{"TOKEN": "different"}); err == nil {
		t.Fatal("lost collision check")
	}
	if err = WriteSecrets(root, project, map[string]string{"TOKEN": "legacy"}); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, canonical)) != string(mustRead(t, legacy)) {
		t.Fatal("did not preserve legacy bytes")
	}
	writeNative(t, canonical, []byte("TOKEN='canonical'\n"))
	if err = os.Chmod(canonical, 0600); err != nil {
		t.Fatal(err)
	}
	values, err = StoredSecrets(root, project)
	if err != nil || values["TOKEN"] != "canonical" {
		t.Fatalf("%v %v", values, err)
	}
	for _, mode := range []os.FileMode{0600, 0644} {
		writeNative(t, canonical, []byte("invalid private syntax"))
		if err = os.Chmod(canonical, mode); err != nil {
			t.Fatal(err)
		}
		if _, err = StoredSecrets(root, project); err == nil {
			t.Fatal("fell back from invalid canonical")
		}
	}
	if p, err := Doctor(root); err != nil || !doctorHasProblem(p, Warning, "legacy") {
		t.Fatalf("%v %v", p, err)
	}
}

func TestHousekeepingSymlinkedLibrary(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		for _, name := range []string{".env.example", ".gitignore"} {
			t.Run(fmt.Sprintf("ancestor=%v/%s", ancestor, name), func(t *testing.T) {
				base := t.TempDir()
				real := filepath.Join(base, "real")
				if err := os.Mkdir(real, 0755); err != nil {
					t.Fatal(err)
				}
				alias := filepath.Join(base, "alias")
				if err := os.Symlink(real, alias); err != nil {
					t.Fatal(err)
				}
				if ancestor {
					real = filepath.Join(real, "nested", "library")
					alias = filepath.Join(alias, "nested", "library")
				}
				if err := Init(alias); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(real, name)
				refreshes := []func(string) error{Init, func(root string) error {
					_, err := RefreshHousekeeping(root)
					return err
				}}
				for _, refresh := range refreshes {
					writeNative(t, path, []byte("# personal notes\n"))
					if err := refresh(alias); err != nil {
						t.Fatal(err)
					}
					data := string(mustRead(t, path))
					if !strings.HasPrefix(data, "# personal notes\n") || !strings.Contains(data, "# daypack:begin ") {
						t.Fatal("housekeeping not refreshed or personal notes lost")
					}
				}
				if problems, err := Doctor(alias); err != nil || len(problems) != 0 {
					t.Fatalf("doctor through alias: %v %v", problems, err)
				}
				outside := filepath.Join(base, "outside")
				writeNative(t, outside, []byte("sentinel"))
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
				for _, refresh := range refreshes {
					if err := refresh(alias); err == nil {
						t.Fatal("accepted symlink housekeeping through library alias")
					}
				}
				if string(mustRead(t, outside)) != "sentinel" {
					t.Fatal("symlink target changed")
				}
			})
		}
	}
}
