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
// Signal order: structural → metadata → semantic (Ollama). Each level is only
// reached when the cheaper signals produce no confident match.
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
		if cfg.Rules.Semantic.Enabled {
			if c := semantic(fe, cfg); c != nil && c.Confidence >= minConfidence {
				candidates = append(candidates, *c)
			}
		}
	}

	return candidates
}
