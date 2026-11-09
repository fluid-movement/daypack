package plan

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
)

type Action string

const (
	Create      Action = "create"
	Update      Action = "update"
	Remove      Action = "remove"
	Adopt       Action = "adopt"
	Conflict    Action = "conflict"
	Drift       Action = "drift"
	Unresolved  Action = "unresolved"
	Unsupported Action = "unsupported"
)

type Outcome string

const (
	Planned   Outcome = "planned"
	Applied   Outcome = "applied"
	Skipped   Outcome = "skipped"
	Failed    Outcome = "failed"
	Unchanged Outcome = "unchanged"
)

type MCPPreview struct {
	Type              string   `json:"type"`
	Command           string   `json:"command"`
	Args              []string `json:"args"`
	URL               string   `json:"url"`
	RequiredVariables []string `json:"required_variables"`
}
type Operation struct {
	Target  string      `json:"target,omitempty"`
	Harness string      `json:"harness,omitempty"`
	Entry   string      `json:"entry"`
	Kind    lock.Kind   `json:"kind,omitempty"`
	Path    string      `json:"path,omitempty"`
	Link    string      `json:"link,omitempty"`
	Key     string      `json:"key,omitempty"`
	Marker  string      `json:"section,omitempty"`
	Member  string      `json:"member,omitempty"`
	Action  Action      `json:"action"`
	Outcome Outcome     `json:"outcome"`
	Gated   bool        `json:"gated"`
	Reason  string      `json:"reason,omitempty"`
	MCP     *MCPPreview `json:"mcp,omitempty"`
}
type Summary struct {
	Create     int `json:"create"`
	Update     int `json:"update"`
	Remove     int `json:"remove"`
	Conflict   int `json:"conflict"`
	Drift      int `json:"drift"`
	Unresolved int `json:"unresolved"`
}
type Plan struct {
	Operations []Operation
	Notes      []contract.Note
}
type ReportOperation struct {
	Operation
	Glyph string `json:"marker"`
}
type Report struct {
	Operations []ReportOperation `json:"operations"`
	Summary    Summary           `json:"summary"`
	Outcomes   map[string]int    `json:"outcomes"`
	Notes      []contract.Note   `json:"notes"`
}

func PreviewMCP(d library.MCPDefinition) MCPPreview {
	return MCPPreview{Type: d.Type, Command: d.Command, Args: append([]string{}, d.Args...), URL: d.URL, RequiredVariables: append([]string{}, d.RequiredVariables()...)}
}
func glyph(a Action) string {
	switch a {
	case Create:
		return "+"
	case Update, Adopt:
		return "~"
	case Remove:
		return "-"
	case Conflict:
		return "!"
	case Drift:
		return "?"
	case Unresolved, Unsupported:
		return "x"
	}
	return ""
}
func clone(p Plan) Plan {
	p.Operations = append([]Operation{}, p.Operations...)
	p.Notes = append([]contract.Note{}, p.Notes...)
	for i := range p.Operations {
		if m := p.Operations[i].MCP; m != nil {
			c := *m
			c.Args = append([]string{}, m.Args...)
			c.RequiredVariables = append([]string{}, m.RequiredVariables...)
			p.Operations[i].MCP = &c
		}
	}
	return p
}

// Normalize returns an isolated presentation copy. Its order is not execution order.
func Normalize(p Plan) (Plan, error) {
	p = clone(p)
	for i := range p.Operations {
		o := &p.Operations[i]
		bad := func(field string) (Plan, error) { return Plan{}, fmt.Errorf("invalid %s at operation %d", field, i+1) }
		if glyph(o.Action) == "" {
			return bad("action")
		}
		if o.Outcome == "" {
			o.Outcome = Planned
		}
		switch o.Outcome {
		case Planned, Applied, Skipped, Failed, Unchanged:
		default:
			return bad("outcome")
		}
		typ, name, ok := strings.Cut(o.Entry, ":")
		if !ok || typ == "" || name == "" {
			return bad("entry")
		}
		diagnostic := o.Action == Conflict || o.Action == Drift || o.Action == Unresolved || o.Action == Unsupported
		if !diagnostic && (o.Target == "" || o.Path == "") {
			return bad("target/path")
		}
		switch o.Kind {
		case lock.Symlink:
			if o.Link == "" {
				return bad("link")
			}
		case lock.File:
		case lock.JSONEntry, lock.TOMLTable:
			if o.Key == "" {
				return bad("key")
			}
		case lock.JSONArrayElement:
			if o.Key == "" || o.Member == "" {
				return bad("key/member")
			}
		case lock.MDSection:
			if o.Marker == "" {
				return bad("section")
			}
		case "":
			if !diagnostic {
				return bad("kind")
			}
		default:
			return bad("kind")
		}
		if o.MCP != nil {
			slices.Sort(o.MCP.RequiredVariables)
		}
	}
	slices.SortFunc(p.Operations, func(a, b Operation) int {
		aa := []string{a.Target, harnessFallback(a), a.Entry, a.Path, a.Key, a.Marker, a.Member, string(a.Action)}
		bb := []string{b.Target, harnessFallback(b), b.Entry, b.Path, b.Key, b.Marker, b.Member, string(b.Action)}
		if c := slices.Compare(aa, bb); c != 0 {
			return c
		}
		// All remaining displayed fields are part of a deterministic tie-break.
		return slices.Compare(tieFields(a), tieFields(b))
	})
	slices.SortFunc(p.Notes, func(a, b contract.Note) int {
		return slices.Compare([]string{a.Harness, a.Code, a.Message}, []string{b.Harness, b.Code, b.Message})
	})
	return p, nil
}
func (p Plan) Report() (Report, error) {
	p, err := Normalize(p)
	if err != nil {
		return Report{}, err
	}
	r := Report{Operations: []ReportOperation{}, Notes: p.Notes, Outcomes: map[string]int{"planned": 0, "applied": 0, "skipped": 0, "failed": 0, "unchanged": 0}}
	for _, o := range p.Operations {
		g := glyph(o.Action)
		r.Operations = append(r.Operations, ReportOperation{o, g})
		switch g {
		case "+":
			r.Summary.Create++
		case "~":
			r.Summary.Update++
		case "-":
			r.Summary.Remove++
		case "!":
			r.Summary.Conflict++
		case "?":
			r.Summary.Drift++
		case "x":
			r.Summary.Unresolved++
		}
		r.Outcomes[string(o.Outcome)]++
	}
	return r, nil
}
func (p Plan) MarshalJSON() ([]byte, error) {
	r, err := p.Report()
	if err != nil {
		return nil, err
	}
	return json.Marshal(r)
}

func tieFields(o Operation) []string {
	fields := []string{o.Harness, string(o.Kind), o.Link, string(o.Outcome), fmt.Sprint(o.Gated), o.Reason}
	if o.MCP != nil {
		fields = append(fields, "mcp", o.MCP.Type, o.MCP.Command, o.MCP.URL)
		fields = append(fields, fmt.Sprint(len(o.MCP.Args)))
		fields = append(fields, o.MCP.Args...)
		fields = append(fields, o.MCP.RequiredVariables...)
	}
	return fields
}

func harnessFallback(o Operation) string {
	if o.Target == "" {
		return o.Harness
	}
	return ""
}
