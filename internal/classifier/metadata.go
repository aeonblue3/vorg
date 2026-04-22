package classifier

import (
	"path/filepath"

	"github.com/aeonblue3/vorg/internal/arena"
	"github.com/aeonblue3/vorg/internal/scanner"
)

func metadata(fe scanner.FileEntry, cfg *arena.ArenaConfig) *Candidate {
	for _, rule := range cfg.Rules.Metadata {
		val, ok := fe.Frontmatter[rule.Field]
		if !ok || val != rule.Value {
			continue
		}

		suggestedPath := ""
		for _, z := range cfg.Zones {
			if z.Name == rule.Suggest {
				suggestedPath = zonePath(cfg.Root, z.Path, filepath.Base(fe.Path))
				break
			}
		}

		return &Candidate{
			File:           fe,
			SuggestedZone:  rule.Suggest,
			SuggestedPath:  suggestedPath,
			Confidence:     rule.Confidence,
			Reason:         "frontmatter " + rule.Field + ": " + rule.Value,
			ClassifierUsed: "metadata",
		}
	}
	return nil
}
