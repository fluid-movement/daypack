package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/fluid-movement/daypack/internal/plan"
)

func PlanResult(p plan.Plan, code int) (Result, error) {
	r, err := p.Report()
	if err != nil {
		return Result{}, err
	}
	normalized, err := plan.Normalize(p)
	if err != nil {
		return Result{}, err
	}
	return Result{Code: code, Operations: r.Operations, Summary: &r.Summary, Outcomes: r.Outcomes, Notes: &r.Notes, plan: &normalized}, nil
}

// ConfirmPlan only obtains consent; the caller executes remaining operations and
// selects Attention when any required operation was skipped.
func ConfirmPlan(p plan.Plan, options Options, yes, dryRun bool, streams IO) (plan.Plan, error) {
	normalized, err := plan.Normalize(p)
	if err != nil {
		return plan.Plan{}, err
	}
	mode := plan.ConfirmationMode(normalized, options.Interactive && !options.JSON, yes, dryRun)
	if mode == plan.None || mode == plan.Preview {
		return normalized, nil
	}
	names := plan.GatedEntries(normalized)
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
	}
	label := strings.Join(quoted, ", ")
	reason := "confirmation required for " + label
	if mode == plan.Prompt {
		out := streams.Err
		if out == nil {
			out = io.Discard
		}
		prompt := fmt.Sprintf("Allow inline MCP commands for %s? [y/N] ", label)
		n, err := io.WriteString(out, prompt)
		if err != nil || n != len(prompt) {
			return plan.Plan{}, errors.New("could not write confirmation prompt")
		}
		in := streams.In
		if in == nil {
			in = strings.NewReader("")
		}
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return plan.Plan{}, errors.New("could not read confirmation response")
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if err == nil && (answer == "y" || answer == "yes") {
			return normalized, nil
		}
		reason = "confirmation declined for " + label
	}
	return plan.SkipGated(normalized, reason), nil
}
