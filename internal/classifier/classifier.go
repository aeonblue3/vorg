package classifier

import (
	"github.com/chrisdias/vorg/internal/arena"
	"github.com/chrisdias/vorg/internal/scanner"
)

// Candidate is a file surfaced as a relocation suggestion.
type Candidate struct {
	File           scanner.FileEntry
	SuggestedZone  string
	SuggestedPath  string
	Confidence     float64
	Reason         string
	ClassifierUsed string
	InboundLinks   []string
}

// Classify runs the three-signal pipeline against a set of FileEntry records
// and returns all candidates that exceeded their confidence thresholds.
// Phase 1: structural and metadata only (semantic skipped).
func Classify(entries []scanner.FileEntry, cfg *arena.ArenaConfig, minConfidence float64) []Candidate {
	var candidates []Candidate

	for _, fe := range entries {
		if c := structural(fe, cfg); c != nil && c.Confidence >= minConfidence {
			candidates = append(candidates, *c)
			continue
		}
		if len(fe.Frontmatter) > 0 {
			if c := metadata(fe, cfg); c != nil && c.Confidence >= minConfidence {
				candidates = append(candidates, *c)
				continue
			}
		}
		// Phase 3: semantic — not implemented yet
	}

	return candidates
}
