package classifier

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/chrisdias/vorg/internal/arena"
	"github.com/chrisdias/vorg/internal/scanner"
)

func structural(fe scanner.FileEntry, cfg *arena.ArenaConfig) *Candidate {
	var best *Candidate

	for _, rule := range cfg.Rules.Structural {
		if !matchesStructural(fe, rule) {
			continue
		}
		if best == nil || rule.Confidence > best.Confidence {
			c := buildCandidate(fe, rule, cfg)
			best = c
		}
	}
	return best
}

func matchesStructural(fe scanner.FileEntry, rule arena.StructuralRule) bool {
	// Zone filter: if rule specifies a zone, file must be in that zone.
	if rule.Zone != "" && fe.Zone != rule.Zone {
		return false
	}

	// Exclude path: if file path contains the exclude string, skip.
	if rule.ExcludePath != "" && strings.Contains(fe.RelPath, rule.ExcludePath) {
		return false
	}

	// Path pattern: file must contain the pattern.
	if rule.PathPattern != "" && !strings.Contains(fe.RelPath, rule.PathPattern) {
		return false
	}

	// Extension filter.
	if len(rule.Extensions) > 0 {
		ext := strings.ToLower(filepath.Ext(fe.Path))
		matched := false
		for _, e := range rule.Extensions {
			if strings.ToLower(e) == ext {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Age threshold.
	if rule.AgeDays > 0 {
		age := time.Since(fe.ModTime)
		if age < time.Duration(rule.AgeDays)*24*time.Hour {
			return false
		}
	}

	return true
}

func buildCandidate(fe scanner.FileEntry, rule arena.StructuralRule, cfg *arena.ArenaConfig) *Candidate {
	suggestedPath := ""
	for _, z := range cfg.Zones {
		if z.Name == rule.Suggest {
			suggestedPath = zonePath(cfg.Root, z.Path, filepath.Base(fe.Path))
			break
		}
	}

	reason := rule.Description
	if reason == "" {
		reason = "structural rule match"
	}
	if rule.AgeDays > 0 {
		days := int(time.Since(fe.ModTime).Hours() / 24)
		reason += " (" + itoa(days) + " days inactive)"
	}

	return &Candidate{
		File:           fe,
		SuggestedZone:  rule.Suggest,
		SuggestedPath:  suggestedPath,
		Confidence:     rule.Confidence,
		Reason:         reason,
		ClassifierUsed: "structural",
	}
}

// zonePath computes the suggested destination path. When the zone path is
// already absolute (e.g. ~/.Trash expanded to /Users/x/.Trash), it is used
// directly. When relative, it is joined under the arena root.
func zonePath(arenaRoot, zPath, base string) string {
	if filepath.IsAbs(zPath) {
		return filepath.Join(zPath, base)
	}
	return filepath.Join(arenaRoot, zPath, base)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
