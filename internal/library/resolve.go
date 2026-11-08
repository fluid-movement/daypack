package library

import (
	"fmt"
	"strings"
)

func (l *Library) Resolve(ref string) (Artifact, error) {
	parts := strings.Split(ref, ":")
	if len(parts) != 2 || !validType(ArtifactType(parts[0])) || !namePattern.MatchString(parts[1]) {
		return Artifact{}, fmt.Errorf("invalid artifact reference: expected type:name with a valid type and name")
	}
	kind, name := ArtifactType(parts[0]), parts[1]
	if a, ok := l.index[ref]; ok {
		if l.invalid[ref] {
			p := []Problem{}
			for _, v := range l.problems {
				if v.Severity == Error && (v.File == a.Path || (a.Type == Skill && v.File == a.Path+"/SKILL.md")) {
					p = append(p, v)
				}
			}
			return Artifact{}, &ValidationError{p}
		}
		return cloneArtifact(a), nil
	}
	closest := ""
	best := int(^uint(0) >> 1)
	for _, a := range l.List(kind) {
		d := distance(name, a.Name)
		if d < best || (d == best && a.Name < closest) {
			best = d
			closest = a.Name
		}
	}
	return Artifact{}, &NotFoundError{kind, name, closest}
}
func distance(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			old := row[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			row[j] = min(row[j]+1, row[j-1]+1, prev+cost)
			prev = old
		}
	}
	return row[len(b)]
}

// Closest returns the candidate nearest to name by edit distance, preferring the
// alphabetically first on ties; it is empty when there are no candidates.
func Closest(name string, candidates []string) string {
	closest := ""
	best := int(^uint(0) >> 1)
	for _, c := range candidates {
		d := distance(name, c)
		if d < best || (d == best && c < closest) {
			best = d
			closest = c
		}
	}
	return closest
}
