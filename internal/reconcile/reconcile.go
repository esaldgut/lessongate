// Package reconcile decides, before EMIT, whether a candidate is NOVEL, OVERLAPS
// an existing public skill (→ edit it), or is a DUPLICATE (→ drop). It keeps a
// local index of the public repo's skills (names + descriptions) refreshed each
// run, avoiding fragmented near-duplicate skills (threat I2).
//
// Built in Fase 4. This file declares the contract.
package reconcile

import "strings"

// Decision is the reconcile outcome for a candidate.
type Decision int

const (
	Novel     Decision = iota // create a new skill
	Overlaps                  // open a PR that edits an existing skill
	Duplicate                 // drop; no meaningful delta
)

func (d Decision) String() string {
	switch d {
	case Novel:
		return "novel"
	case Overlaps:
		return "overlaps"
	case Duplicate:
		return "duplicate"
	default:
		return "unknown"
	}
}

// SkillRef is one existing public skill in the index.
type SkillRef struct {
	Name        string
	Description string
	Path        string
}

// Reconciler classifies a candidate against the public skill index.
type Reconciler interface {
	Classify(candidateName, candidateSummary string) (Decision, SkillRef)
}

// reconciler classifies by name-token overlap against the index. This is the
// deterministic v0.1 strategy (a Claude classification pass is a future upgrade);
// it favors proposing an edit (Overlaps) over creating a near-duplicate (threat
// I2), and only declares Duplicate on an exact name match.
type reconciler struct {
	idx []SkillRef
}

// New builds a Reconciler over the current public skill index.
func New(idx []SkillRef) Reconciler { return &reconciler{idx: idx} }

// overlapThreshold is the fraction of the smaller name's tokens that must be
// shared for two skills to be considered overlapping.
const overlapThreshold = 0.5

func (r *reconciler) Classify(candidateName, _ string) (Decision, SkillRef) {
	cand := tokenize(candidateName)
	best := -1.0
	var bestRef SkillRef

	for _, ref := range r.idx {
		if ref.Name == candidateName {
			return Duplicate, ref
		}
		sim := jaccard(cand, tokenize(ref.Name))
		if sim > best {
			best, bestRef = sim, ref
		}
	}

	if best >= overlapThreshold {
		return Overlaps, bestRef
	}
	return Novel, SkillRef{}
}

// tokenize splits a kebab/space name into a set of lowercase tokens.
func tokenize(s string) map[string]bool {
	set := map[string]bool{}
	for f := range strings.FieldsSeq(strings.ReplaceAll(strings.ToLower(s), "-", " ")) {
		set[f] = true
	}
	return set
}

// jaccard returns |a∩b| / |a∪b| for two token sets (0 when either is empty).
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
