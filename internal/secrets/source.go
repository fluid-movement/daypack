package secrets

import "fmt"

// Source maps a reference name to an argv read command. Future providers can
// implement this contract while renderers choose their runtime strategy.
// Optional defaults remain the harness's responsibility.
type Source interface {
	ReadCommand(name string) ([]string, error)
}

// EnvSource reads from the harness's inherited environment at runtime.
type EnvSource struct{}

// ReadCommand does not look up values or run commands. The fixed POSIX shell
// template fails for unset variables and succeeds for set-but-empty variables.
func (EnvSource) ReadCommand(name string) ([]string, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid environment variable name")
	}
	return []string{"sh", "-c", "test \"${" + name + "+x}\" = x || exit 1; printf '%s' \"$" + name + "\""}, nil
}
