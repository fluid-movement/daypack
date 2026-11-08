package plan

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func display(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}
func RenderText(w io.Writer, p Plan) error {
	r, err := p.Report()
	if err != nil {
		return err
	}
	var b bytes.Buffer
	group := ""
	for _, row := range r.Operations {
		g := row.Target
		if g == "" {
			g = row.Harness
			if g == "" {
				g = "unresolved"
			}
		}
		if g != group {
			fmt.Fprintf(&b, "%s:\n", display(g))
			group = g
		}
		fmt.Fprintf(&b, "  %s %s %s", row.Glyph, row.Action, display(row.Entry))
		if row.Path != "" {
			fmt.Fprintf(&b, " %s", display(row.Path))
		}
		for _, part := range []struct{ name, value string }{{"key", row.Key}, {"member", row.Member}, {"section", row.Marker}} {
			if part.value != "" {
				fmt.Fprintf(&b, " %s=%s", part.name, strconv.Quote(part.value))
			}
		}
		if row.Link != "" {
			fmt.Fprintf(&b, " -> %s", display(row.Link))
		}
		if m := row.MCP; m != nil {
			fmt.Fprintf(&b, " mcp[%s]", display(m.Type))
			if m.Command != "" {
				fmt.Fprintf(&b, " %s", strconv.Quote(m.Command))
				for _, a := range m.Args {
					fmt.Fprintf(&b, " %s", strconv.Quote(a))
				}
			}
			if m.URL != "" {
				fmt.Fprintf(&b, " url=%s", strconv.Quote(m.URL))
			}
			fmt.Fprintf(&b, " variables=[%s]", display(strings.Join(m.RequiredVariables, ", ")))
		}
		fmt.Fprintf(&b, " [%s", row.Outcome)
		if row.Gated {
			b.WriteString("; gated")
		}
		if row.Reason != "" {
			fmt.Fprintf(&b, "; %s", display(row.Reason))
		}
		b.WriteString("]\n")
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "note %s %s: %s\n", display(n.Harness), display(n.Code), display(n.Message))
	}
	if len(r.Operations) == 0 {
		b.WriteString("No changes.\n")
	}
	s := r.Summary
	fmt.Fprintf(&b, "Summary: + %d ~ %d - %d ! %d ? %d x %d\n", s.Create, s.Update, s.Remove, s.Conflict, s.Drift, s.Unresolved)
	fmt.Fprintf(&b, "Outcomes: planned %d applied %d skipped %d failed %d unchanged %d\n", r.Outcomes["planned"], r.Outcomes["applied"], r.Outcomes["skipped"], r.Outcomes["failed"], r.Outcomes["unchanged"])
	return writeDocument(w, b.Bytes())
}
func RenderJSON(w io.Writer, p Plan) error {
	data, err := p.MarshalJSON()
	if err != nil {
		return err
	}
	return writeDocument(w, append(data, '\n'))
}

func writeDocument(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}
