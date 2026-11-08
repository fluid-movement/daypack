package secrets

import (
	"reflect"
	"strings"
	"testing"
)

func TestNamesAndReferences(t *testing.T) {
	for _, name := range []string{"A", "_", "_one2", "aZ09"} {
		if !ValidName(name) {
			t.Fatal("valid name rejected")
		}
		ref, err := EnvReference(name)
		if err != nil || ref != "${"+name+"}" {
			t.Fatal("incorrect environment reference")
		}
	}
	for _, name := range []string{"", "1A", "a-b", "a b", "é", "A;touch /tmp/unsafe", "A$(id)", "A\nB", "A}"} {
		if ValidName(name) {
			t.Fatal("invalid name accepted")
		}
		if _, err := EnvReference(name); err == nil {
			t.Fatal("invalid name reference accepted")
		}
		if _, err := (EnvSource{}).ReadCommand(name); err == nil {
			t.Fatal("invalid command name accepted")
		}
	}
	empty, fallback := "", "a $ ` # : -"
	input := "x${A}/${B:-}${A:-a $ ` # : -}${A}"
	want := []Reference{{"A", nil, 1, 5}, {"B", &empty, 6, 12}, {"A", &fallback, 12, 29}, {"A", nil, 29, 33}}
	got, err := ParseReferences(input)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("incorrect reference spans: %#v %v", got, err)
	}
	refs, err := ParseReferences("ordinary text $NAME")
	if err != nil || refs == nil || len(refs) != 0 {
		t.Fatal("ordinary text parse")
	}
	for _, text := range []string{"${}", "${1X}", "${A-default}", "${A", "${A:-private${B}}", "${A:-private{value}}", "${A${B}}", "${A} ${B:-private"} {
		if _, err := ParseReferences(text); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("unsafe malformed reference handling")
		}
	}
}
