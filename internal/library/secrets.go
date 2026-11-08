package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fluid-movement/daypack/internal/secrets"
)

// secretsMetadata never opens secrets.env or follows its leaf symlink.
func secretsMetadata(root string) ([]Problem, error) {
	file := filepath.Join(root, "secrets.env")
	info, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect secrets.env metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return []Problem{problem(file, "file", "secrets.env must be a regular file, not a symlink or special file", "")}, nil
	}
	if info.Mode().Perm()&0077 != 0 {
		return []Problem{{Warning, file, "permissions", "secrets.env is accessible to group or others", "Run chmod 600 on the quoted secrets.env path."}}, nil
	}
	return nil, nil
}

func privateSecretsPath(root, projectRoot string) (string, error) {
	if root == "" || projectRoot == "" {
		return "", fmt.Errorf("library and project roots are required")
	}
	root, err := safePath(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("library root must be an existing directory")
	}
	project, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	// Resolve aliases in the project as well as rejecting aliases in the library.
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	dest := filepath.Join(root, "secrets.env")
	rel, err := filepath.Rel(project, dest)
	if err != nil {
		return "", err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return "", fmt.Errorf("secrets.env must be outside the project/worktree")
	}
	if _, err := safePath(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func checkSecretsDestination(root string) error {
	problems, err := secretsMetadata(root)
	if err != nil {
		return err
	}
	for _, p := range problems {
		return fmt.Errorf("%s: %s; %s", p.File, p.Message, p.Hint)
	}
	hygiene, err := secretsHygiene(root)
	if err != nil {
		return err
	}
	for _, p := range hygiene {
		return fmt.Errorf("%s: %s; %s", p.File, p.Message, p.Hint)
	}
	return nil
}

// CheckSecrets performs the same storage checks as WriteSecrets without writing.
func CheckSecrets(root, projectRoot string, values map[string]string) error {
	_, _, _, err := prepareSecrets(root, projectRoot, values)
	return err
}

func prepareSecrets(root, projectRoot string, values map[string]string) (string, []byte, []string, error) {
	if len(values) == 0 {
		return "", nil, nil, nil
	}
	for name, value := range values {
		if !secrets.ValidName(name) {
			return "", nil, nil, fmt.Errorf("invalid secret variable name")
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return "", nil, nil, fmt.Errorf("invalid secret value for %s: NUL, CR and LF are not supported", name)
		}
	}
	dest, err := privateSecretsPath(root, projectRoot)
	if err != nil {
		return "", nil, nil, err
	}
	root = filepath.Dir(dest)
	if err := checkSecretsDestination(root); err != nil {
		return "", nil, nil, err
	}
	data, err := os.ReadFile(dest)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, nil, err
	}
	existing, err := parseSecrets(data)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%s: %w", dest, err)
	}
	missing := []string{}
	for name, value := range values {
		if old, exists := existing[name]; exists {
			if old != value {
				return "", nil, nil, fmt.Errorf("secret variable %s already has a different value", name)
			}
		} else {
			missing = append(missing, name)
		}
	}
	return dest, data, missing, nil
}

// WriteSecrets stores a complete importer batch in the selected library only.
// It validates the entire batch before staging, preserves supported existing
// bytes, and atomically replaces the file. Concurrent writers are not coordinated.
func WriteSecrets(root, projectRoot string, values map[string]string) (err error) {
	if len(values) == 0 {
		return nil
	}
	dest, data, missing, err := prepareSecrets(root, projectRoot, values)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	var out strings.Builder
	out.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		out.WriteByte('\n')
	}
	for _, name := range missing {
		out.WriteString(name + "='" + strings.ReplaceAll(values[name], "'", "'\\''") + "'\n")
	}
	stage, clean, err := stagePath(dest)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, clean()) }()
	// MkdirTemp is private but honors umask; restore owner access even when
	// the caller has an unusually restrictive mask.
	if err := os.Chmod(filepath.Dir(stage), 0700); err != nil {
		return err
	}
	if err := writePrivateSecrets(stage, out.String()); err != nil {
		return err
	}
	return publishSecrets(stage, dest)
}

func writePrivateSecrets(path, data string) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if _, err = file.WriteString(data); err != nil {
		return err
	}
	return file.Sync()
}

func publishSecrets(stage, dest string) error {
	if _, err := safePath(dest); err != nil {
		return err
	}
	if err := checkSecretsDestination(filepath.Dir(dest)); err != nil {
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		return fmt.Errorf("publish secrets.env: %w", err)
	}
	return nil
}

// parseSecrets accepts only blank lines, whole-line comments and NAME=value.
// Literal segments are safe unquoted ASCII, single quotes and the apostrophe
// splice \' outside quotes. It never executes or evaluates its input.
func parseSecrets(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for index, line := range strings.Split(string(data), "\n") {
		fail := func() error { return fmt.Errorf("unsupported secrets.env syntax at line %d", index+1) }
		if strings.ContainsAny(line, "\x00\r") {
			return nil, fail()
		}
		trimmed := strings.Trim(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, encoded, ok := strings.Cut(line, "=")
		if !ok || !secrets.ValidName(name) {
			return nil, fail()
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("duplicate variable %s at line %d", name, index+1)
		}
		value, ok := decodeSecret(encoded)
		if !ok {
			return nil, fail()
		}
		values[name] = value
	}
	return values, nil
}

func decodeSecret(encoded string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(encoded); {
		switch encoded[i] {
		case '\'':
			end := strings.IndexByte(encoded[i+1:], '\'')
			if end < 0 {
				return "", false
			}
			end += i + 1
			out.WriteString(encoded[i+1 : end])
			i = end + 1
		case '\\':
			if i+1 >= len(encoded) || encoded[i+1] != '\'' {
				return "", false
			}
			out.WriteByte('\'')
			i += 2
		default:
			c := encoded[i]
			valid := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("_./:@%+,=-", rune(c))
			if !valid {
				return "", false
			}
			out.WriteByte(c)
			i++
		}
	}
	return out.String(), true
}

// StoredSecrets reads a checked private store for importer content comparisons.
// It never expands references or consults the process environment.
func StoredSecrets(root, projectRoot string) (map[string]string, error) {
	dest, err := privateSecretsPath(root, projectRoot)
	if err != nil {
		return nil, err
	}
	if err = checkSecretsDestination(root); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(dest)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return parseSecrets(data)
}
