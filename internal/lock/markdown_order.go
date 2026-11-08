package lock

import (
	"bytes"
	"fmt"
	"sort"
)

// Section is the desired body of one managed markdown section.
type Section struct {
	Marker string
	Body   []byte
}

func renderSection(s Section) []byte {
	out := []byte("<!-- daypack:begin " + s.Marker + " -->")
	out = append(out, s.Body...)
	return append(out, "<!-- daypack:end "+s.Marker+" -->"...)
}

// lineEnd returns the offset just past the line terminator that directly
// follows pos, or pos when none follows.
func lineEnd(data []byte, pos int) int {
	switch {
	case bytes.HasPrefix(data[pos:], []byte("\r\n")):
		return pos + 2
	case bytes.HasPrefix(data[pos:], []byte("\n")):
		return pos + 1
	}
	return pos
}

// plan describes how a document's managed slots map to the desired order.
type slotPlan struct {
	spans     []span
	survivors []span // managed sections that stay, in file order
	dropped   []span // managed sections no longer desired
	present   []int  // desired indexes that already exist in the file, in desired order
}

func planSlots(data []byte, desired []Section, managed []string) (slotPlan, error) {
	var p slotPlan
	spans, err := parseSpans(data)
	if err != nil {
		return p, err
	}
	p.spans = spans
	isManaged := map[string]bool{}
	for _, m := range managed {
		if err := validateMarker(m); err != nil {
			return p, err
		}
		if isManaged[m] {
			return p, fmt.Errorf("duplicate managed marker")
		}
		isManaged[m] = true
	}
	want := map[string]bool{}
	for _, s := range desired {
		if err := validateMarker(s.Marker); err != nil {
			return p, err
		}
		if !isManaged[s.Marker] || want[s.Marker] {
			return p, fmt.Errorf("desired section is unmanaged or duplicated")
		}
		if bytes.Contains(s.Body, []byte("<!-- daypack:")) {
			return p, fmt.Errorf("body contains markers")
		}
		want[s.Marker] = true
	}
	inFile := map[string]bool{}
	for _, s := range spans {
		if !isManaged[s.name] {
			continue
		}
		inFile[s.name] = true
		if want[s.name] {
			p.survivors = append(p.survivors, s)
		} else {
			p.dropped = append(p.dropped, s)
		}
	}
	for i, s := range desired {
		if inFile[s.Marker] {
			p.present = append(p.present, i)
		}
	}
	return p, nil
}

// MovedSections reports which desired markers change position relative to the
// other surviving managed sections. Creation and removal alone move nothing.
func MovedSections(data []byte, desired []string, managed []string) ([]string, error) {
	sections := make([]Section, len(desired))
	for i, m := range desired {
		sections[i] = Section{Marker: m}
	}
	p, err := planSlots(data, sections, managed)
	if err != nil {
		return nil, err
	}
	var moved []string
	for k, s := range p.survivors {
		if m := desired[p.present[k]]; m != s.name {
			moved = append(moved, m)
		}
	}
	return moved, nil
}

type edit struct {
	start, end int
	text       []byte
}

// ReconcileSections rewrites the managed sections of one document in a single
// pass so they follow the order of desired. Surviving sections keep their
// slots, and fill each other's slots only when the order changed; sections no
// longer desired are dropped together with the line terminator the section
// owned; new sections are inserted right after the section that precedes them
// in desired (the last managed slot for appended ones), else after the last
// daypack section, else at the end. Every other byte is copied unchanged.
// Sections not listed in managed are never touched. The input is not modified;
// invalid marker structure returns an error and no output.
func ReconcileSections(data []byte, desired []Section, managed []string) ([]byte, error) {
	p, err := planSlots(data, desired, managed)
	if err != nil {
		return nil, err
	}
	var edits []edit
	for _, s := range p.dropped {
		end := s.end
		if s.begin == 0 || data[s.begin-1] == '\n' {
			end = lineEnd(data, s.end)
		}
		edits = append(edits, edit{s.begin, end, nil})
	}
	slotOf := map[int]int{} // desired index -> survivor slot
	for k, s := range p.survivors {
		i := p.present[k]
		slotOf[i] = k
		edits = append(edits, edit{s.begin, s.end, renderSection(desired[i])})
	}
	// New sections attach to the nearest preceding present section.
	front := []byte{}
	after := map[int][]byte{}
	tail := []byte{}
	anchor := -1
	for i, s := range desired {
		if k, ok := slotOf[i]; ok {
			anchor = k
			continue
		}
		text := append(renderSection(s), '\n')
		switch {
		case len(p.survivors) == 0:
			tail = append(tail, text...)
		case anchor < 0:
			front = append(front, text...)
		default:
			after[anchor] = append(after[anchor], text...)
		}
	}
	insertAfter := func(pos int, text []byte) {
		if pos > 0 && data[pos-1] != '\n' {
			text = append([]byte("\n"), text...)
		}
		edits = append(edits, edit{pos, pos, text})
	}
	if len(front) > 0 {
		first := p.survivors[0].begin
		if first > 0 && data[first-1] != '\n' {
			front = append([]byte("\n"), front...)
		}
		edits = append(edits, edit{first, first, front})
	}
	for k, text := range after {
		insertAfter(lineEnd(data, p.survivors[k].end), text)
	}
	if len(tail) > 0 {
		pos := len(data)
		if n := len(p.spans); n > 0 {
			pos = lineEnd(data, p.spans[n-1].end)
		}
		insertAfter(pos, tail)
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		return edits[i].end == edits[i].start && edits[j].end != edits[j].start
	})
	out := []byte{}
	cursor := 0
	for _, e := range edits {
		if e.start < cursor {
			return nil, fmt.Errorf("internal ordering error")
		}
		out = append(out, data[cursor:e.start]...)
		out = append(out, e.text...)
		cursor = e.end
	}
	return append(out, data[cursor:]...), nil
}
