package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
)

const (
	Success = iota
	Error
	Usage
	Attention
)

// Result is a command's typed outcome. Attention means valid input needs action.
// Payload values must be JSON encodable and must not contain secrets.
type Result struct {
	Code       int    `json:"-"`
	Items      any    `json:"items,omitempty"`
	Operations any    `json:"operations,omitempty"`
	Problems   any    `json:"problems,omitempty"`
	Help       string `json:"help,omitempty"`
	Message    string `json:"-"`
}

// Failure classifies an error without inspecting its text. Causes and hints must
// be safe to display, with no secret values.
type Failure struct {
	Code  int
	Cause error
	Hint  string
}

func (e *Failure) Error() string { return e.Cause.Error() }
func (e *Failure) Unwrap() error { return e.Cause }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func diagnostic(w io.Writer, cause, hint string) error {
	if _, err := fmt.Fprintln(w, oneLine(cause)); err != nil {
		return err
	}
	if hint != "" {
		_, err := fmt.Fprintln(w, "Hint: "+oneLine(hint))
		return err
	}
	return nil
}

func render(w io.Writer, options Options, command string, result Result) error {
	if options.JSON {
		envelope := struct {
			OK      bool   `json:"ok"`
			Command string `json:"command"`
			Result
		}{result.Code == Success, command, result}
		// Marshal first so an encoding failure cannot leave a partial object.
		data, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}
	if result.Help != "" {
		_, err := io.WriteString(w, result.Help)
		return err
	}
	if problems, ok := result.Problems.([]library.Problem); ok {
		for _, p := range problems {
			if _, err := fmt.Fprintf(w, "%s: %s: %s %s\n", p.Severity, p.File, p.Message, p.Hint); err != nil {
				return err
			}
		}
	}
	if !options.Quiet && result.Message != "" {
		_, err := fmt.Fprintln(w, result.Message)
		return err
	}
	return nil
}
