package secrets

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestEnvReadCommand(t *testing.T) {
	var source Source = EnvSource{}
	t.Setenv("DAYPACK_READ_TEST", "first")
	argv, err := source.ReadCommand("DAYPACK_READ_TEST")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DAYPACK_READ_TEST", "second")
	again, err := source.ReadCommand("DAYPACK_READ_TEST")
	if err != nil || !reflect.DeepEqual(argv, again) {
		t.Fatal("command generation looked up value")
	}
	again[0] = "changed"
	if argv[0] != "sh" {
		t.Fatal("shared argv")
	}
	for _, tc := range []struct {
		name, value string
		set         bool
	}{
		{"unset", "", false}, {"empty", "", true}, {"literal", "spaces ' \" $ ` # = \\ $(exit 9)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Env = []string{}
			if tc.set {
				cmd.Env = append(cmd.Env, "DAYPACK_READ_TEST="+tc.value)
			}
			output, err := cmd.Output()
			if (err == nil) != tc.set || string(output) != tc.value {
				t.Fatal("incorrect runtime unset/empty/literal semantics")
			}
		})
	}
}
