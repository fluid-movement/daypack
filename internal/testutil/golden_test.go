package testutil

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPlanGoldens(t *testing.T) {
	// Hand-built shape examples; these do not claim to be production renderers.
	AssertGolden(t, "testdata/plan.txt", []byte("create .agents/skills/testing\ncreate .test/skills/testing\n"))
	AssertJSONGolden(t, "testdata/plan.json", []byte(`{"operations":[{"path":".agents/skills/testing","action":"create"},{"path":".test/skills/testing","action":"create"}]}`))
}

func TestCanonicalJSON(t *testing.T) {
	for _, input := range []string{`{} {}`, `{} trailing`, ``, `[`} {
		if _, err := canonicalJSON([]byte(input)); err == nil {
			t.Fatalf("accepted invalid document %q", input)
		}
	}
	a, err := canonicalJSON([]byte(`{"b":2,"a":[2,1]}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalJSON([]byte(`{"a":[2,1],"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("object keys not canonical")
	}
	c, err := canonicalJSON([]byte(`{"a":[1,2],"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, c) {
		t.Fatal("array order lost")
	}
}

func TestNormalizeFixturePaths(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "project")
	got := NormalizeFixturePaths([]byte(child+"\n"+root+"\nsecret-preserved"), root, child)
	if string(got) != "<ROOT1>\n<ROOT0>\nsecret-preserved" {
		t.Fatalf("normalization: %s", got)
	}
}

func TestItemGolden(t *testing.T) {
	items, err := (FakeAdapter{}).Desired(Entry{Name: "testing", Source: "<LIBRARY>/skills/testing", Type: Skill}, Project{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	AssertJSONGolden(t, "testdata/items.json", data)
}
