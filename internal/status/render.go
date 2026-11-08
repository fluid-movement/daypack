package status

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
)

func display(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}

func harnessLabel(h string) string {
	if h == "" {
		return "-"
	}
	return display(h)
}

// RenderAttention writes only entries and checks that need action.
func RenderAttention(w io.Writer, r Report) error {
	var b bytes.Buffer
	for _, e := range r.Entries {
		if e.State != OK {
			fmt.Fprintf(&b, "%s %s: %s\n", display(e.Entry), harnessLabel(e.Harness), e.State)
		}
	}
	writeChecks(&b, r, true)
	return write(w, b.Bytes())
}

// RenderText writes the full report.
func RenderText(w io.Writer, r Report) error {
	var b bytes.Buffer
	b.WriteString("Entries:\n")
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "  %s %s: %s\n", display(e.Entry), harnessLabel(e.Harness), e.State)
	}
	if len(r.Entries) == 0 {
		b.WriteString("  none\n")
	}
	if len(r.Changes.Operations) > 0 {
		b.WriteString("Changes:\n")
		for _, o := range r.Changes.Operations {
			fmt.Fprintf(&b, "  %s %s %s %s", o.Glyph, o.Action, display(o.Entry), display(o.Path))
			if o.Key != "" {
				fmt.Fprintf(&b, " key=%s", strconv.Quote(o.Key))
			}
			if o.Marker != "" {
				fmt.Fprintf(&b, " section=%s", strconv.Quote(o.Marker))
			}
			if o.Reason != "" {
				fmt.Fprintf(&b, " [%s]", display(o.Reason))
			}
			b.WriteString("\n")
		}
	}
	if len(r.Checks) > 0 {
		b.WriteString("Checks:\n")
		writeChecks(&b, r, false)
	}
	if len(r.Readers) > 0 {
		b.WriteString("Readers:\n")
		for _, t := range r.Readers {
			fmt.Fprintf(&b, "  %s: selected [%s]; also read by [%s]\n", display(t.Target), display(strings.Join(t.Selected, ", ")), display(strings.Join(t.OtherReaders, ", ")))
		}
	}
	for _, n := range r.Changes.Notes {
		fmt.Fprintf(&b, "note %s %s: %s\n", display(n.Harness), display(n.Code), display(n.Message))
	}
	if r.NeedsAttention() {
		b.WriteString("Needs attention.\n")
	} else {
		b.WriteString("All ok.\n")
	}
	return write(w, b.Bytes())
}

func writeChecks(b *bytes.Buffer, r Report, onlyErrors bool) {
	for _, c := range r.Checks {
		if onlyErrors && c.Severity != library.Error {
			continue
		}
		fmt.Fprintf(b, "  %s %s", c.Severity, c.Code)
		for _, f := range []struct{ name, v string }{{"entry", c.Entry}, {"path", c.Path}, {"variable", c.Variable}} {
			if f.v != "" {
				fmt.Fprintf(b, " %s=%s", f.name, strconv.Quote(f.v))
			}
		}
		fmt.Fprintf(b, ": %s\n", display(c.Message))
	}
}

func write(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}
