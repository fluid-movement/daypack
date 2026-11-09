package lock

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOMLTableOwnership(t *testing.T) {
	i := Item{Target: "codex", Entry: "mcp:docs", Kind: TOMLTable, Path: ".codex/config.toml", Key: "mcp_servers.docs"}
	before := []byte("# personal\nmodel = 'my-model' # keep\n[mcp_servers]\n# existing container\n[mcp_servers.user]\ncommand = 'mine'\n\n[other]\ntext = '''\n[mcp_servers.fake]\n'''\n")
	body := []byte("command = 'echo'\nargs = ['a', 'b']\nenv_vars = ['TOKEN']\n[env]\nMODE = 'test'\n")
	out, err := SetTOML(before, i, body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, before) {
		t.Fatalf("user bytes changed: %s", out)
	}
	hash, exists, err := observeTOML(out, i.Key)
	desired, e := HashTOML(body)
	if err != nil || e != nil || !exists || hash != desired {
		t.Fatal(hash, desired, exists, err, e)
	}
	formatted := bytes.ReplaceAll(out, []byte(`command = "echo"`), []byte("command='echo' # same semantics"))
	other, _, err := observeTOML(formatted, i.Key)
	if err != nil || other != hash {
		t.Fatal("format drift", err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, i.Path), out, 0600); err != nil {
		t.Fatal(err)
	}
	i.Hash = desired
	l := New()
	if err := l.Put(i); err != nil {
		t.Fatal(err)
	}
	obs, err := l.Status(root, i)
	if err != nil || obs.State != OK {
		t.Fatal(obs, err)
	}
	if err := l.Save(root); err != nil {
		t.Fatal(err)
	}
	l, err = Load(root)
	if err != nil || len(l.Items()) != 1 {
		t.Fatal(err)
	}
	if err := l.RemoveOwned(root, i, []string{".codex/config.toml"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, i.Path))
	if err != nil || !bytes.Equal(got, before) {
		t.Fatalf("remove changed user content: %s %v", got, err)
	}
}
func TestTOMLQuotedKeysAndNoncontiguousDescendants(t *testing.T) {
	i := Item{Kind: TOMLTable, Key: `mcp_servers.a\.b`}
	before := []byte("[mcp_servers.'a.b'] # own\ncommand='old'\n[mcp_servers.user]\ncommand='keep'\n[mcp_servers.\"a.b\".env]\nMODE='old'\n[elsewhere]\nx=1\n")
	out, err := SetTOML(before, i, []byte("command = 'new'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("[mcp_servers.user]\ncommand='keep'\n")) || !bytes.Contains(out, []byte("[elsewhere]\nx=1\n")) || bytes.Contains(out, []byte("old")) {
		t.Fatalf("bad subtree edit: %s", out)
	}
	h, ok, err := observeTOML(out, i.Key)
	want, e := HashTOML([]byte("command = 'new'\n"))
	if !ok || err != nil || e != nil || h != want {
		t.Fatal(h, want, ok, err, e)
	}
}
func TestTOMLUnsafeLayoutsAreRejected(t *testing.T) {
	for _, input := range []string{
		"bad = [", "mcp_servers = 'text'", "mcp_servers = { docs = {command = 'x'} }", "mcp_servers = {}",
		"mcp_servers.docs.command = 'x'", "[mcp_servers]\ndocs.command = 'x'",
		"[[mcp_servers.docs]]\ncommand='x'", "[mcp_servers.docs]\n[[mcp_servers.docs.env]]\na='x'",
		"[mcp_servers.docs]\ncommand='one'\ncommand='two'", "[[mcp_servers]]\ndocs='x'",
	} {
		t.Run(input, func(t *testing.T) {
			i := Item{Kind: TOMLTable, Key: "mcp_servers.docs"}
			if _, err := SetTOML([]byte(input), i, []byte("command='new'")); err == nil {
				t.Fatal("accepted unsafe config")
			}
			if _, _, err := observeTOML([]byte(input), i.Key); err == nil {
				t.Fatal("observation accepted unsafe layout")
			}
		})
	}
}
func TestTOMLMultilineAndHeaderLookalikes(t *testing.T) {
	for _, input := range []string{
		"text = \"\"\"hello\n[mcp_servers.docs]\nworld\"\"\"\n",
		"values = [\n'[mcp_servers.docs]',\n]\n",
		"text = \"\"\"escaped \\\"\"\" more\n[mcp_servers.docs]\n\"\"\"\n",
		"text = '''literal\n[mcp_servers.docs]\n'''\n",
		"# [mcp_servers.docs]\n",
	} {
		i := Item{Kind: TOMLTable, Key: "mcp_servers.docs"}
		out, err := SetTOML([]byte(input), i, []byte("command='new'"))
		if err != nil {
			t.Fatal(err)
		}
		removed, err := RemoveTOML(out, i)
		if err != nil || string(removed) != input {
			t.Fatal("lookalike changed", string(removed), err)
		}
	}
}
func TestTOMLOwnershipIdentity(t *testing.T) {
	l := New()
	i := Item{Target: "codex", Entry: "mcp:a", Kind: TOMLTable, Path: ".codex/config.toml", Key: "mcp_servers.a"}
	if err := l.Put(i); err != nil {
		t.Fatal(err)
	}
	j := i
	j.Key = "mcp_servers.b"
	j.Entry = "mcp:b"
	if err := l.Put(j); err != nil {
		t.Fatal(err)
	}
	j.Key = "mcp_servers.a.env"
	if err := l.Put(j); err == nil {
		t.Fatal("allowed overlap")
	}
	j.Key = "mcp_servers"
	if err := l.Put(j); err == nil {
		t.Fatal("allowed parent overlap")
	}
	j = i
	j.Kind = File
	if err := l.Put(j); err == nil {
		t.Fatal("allowed file overlap")
	}
	block, err := ExcludeBlock("/project", l.Items())
	if err != nil || !strings.Contains(block, "/.codex/config.toml\n") || strings.Contains(block, "/.codex/\n") {
		t.Fatal(block, err)
	}
}
