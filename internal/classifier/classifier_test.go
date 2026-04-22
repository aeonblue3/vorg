package classifier

import (
	"testing"
	"time"

	"github.com/aeonblue3/vorg/internal/arena"
	"github.com/aeonblue3/vorg/internal/scanner"
)

func makeEntry(relPath, zone string, ageDays int, fm map[string]string) scanner.FileEntry {
	return scanner.FileEntry{
		Path:        "/vault/" + relPath,
		RelPath:     relPath,
		Zone:        zone,
		ModTime:     time.Now().Add(-time.Duration(ageDays) * 24 * time.Hour),
		Frontmatter: fm,
	}
}

var testCfg = &arena.ArenaConfig{
	Name: "vault",
	Root: "/vault",
	Zones: []arena.ZoneConfig{
		{Name: "inbox", Path: "Inbox"},
		{Name: "active", Path: "Active"},
		{Name: "reference", Path: "Reference"},
		{Name: "archive", Path: "Archive"},
	},
	Rules: arena.RulesConfig{
		Structural: []arena.StructuralRule{
			{
				Description: "Review folder idle >45 days",
				Zone:        "active",
				PathPattern: "Reviews/",
				AgeDays:     45,
				Suggest:     "archive",
				Confidence:  0.85,
			},
			{
				Description: "Non-markdown in active",
				Zone:        "active",
				Extensions:  []string{".pdf"},
				Suggest:     "reference",
				Confidence:  0.70,
			},
		},
		Metadata: []arena.MetadataRule{
			{Field: "status", Value: "complete", Suggest: "archive", Confidence: 0.90},
			{Field: "status", Value: "active", Suggest: "active", Confidence: 0.95},
			{Field: "type", Value: "reference", Suggest: "reference", Confidence: 0.90},
		},
	},
}

// ── structural ────────────────────────────────────────────────────────────────

func TestStructural_StaleReview(t *testing.T) {
	e := makeEntry("Active/Reviews/Widget Service.md", "active", 50, nil)
	c := structural(e, testCfg)
	if c == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if c.SuggestedZone != "archive" {
		t.Errorf("zone: got %q, want %q", c.SuggestedZone, "archive")
	}
	if c.ClassifierUsed != "structural" {
		t.Errorf("classifier: %q", c.ClassifierUsed)
	}
}

func TestStructural_FreshReviewSkipped(t *testing.T) {
	e := makeEntry("Active/Reviews/New Project.md", "active", 10, nil)
	c := structural(e, testCfg)
	if c != nil {
		t.Errorf("expected nil for fresh review, got candidate for zone %q", c.SuggestedZone)
	}
}

func TestStructural_WrongZoneSkipped(t *testing.T) {
	// Stale but not in 'active' zone — rule zone filter should block it.
	e := makeEntry("Archive/Reviews/Old.md", "archive", 50, nil)
	c := structural(e, testCfg)
	// The review rule requires zone=active, so this should not match.
	if c != nil && c.SuggestedZone == "archive" {
		// If it matched, it would be due to a zone-unfiltered rule — check rule
		// The PDF rule has zone=active, review rule has zone=active; archive doesn't match either.
		// So nil is expected.
		t.Errorf("expected nil for archive-zone file, got %v", c)
	}
}

func TestStructural_ExtensionRule(t *testing.T) {
	e := makeEntry("Active/report.pdf", "active", 0, nil)
	c := structural(e, testCfg)
	if c == nil {
		t.Fatal("expected candidate for .pdf in active")
	}
	if c.SuggestedZone != "reference" {
		t.Errorf("zone: got %q, want %q", c.SuggestedZone, "reference")
	}
}

// ── metadata ──────────────────────────────────────────────────────────────────

func TestMetadata_StatusComplete(t *testing.T) {
	e := makeEntry("Active/done.md", "active", 1, map[string]string{"status": "complete"})
	c := metadata(e, testCfg)
	if c == nil {
		t.Fatal("expected candidate for status:complete")
	}
	if c.SuggestedZone != "archive" {
		t.Errorf("zone: got %q, want %q", c.SuggestedZone, "archive")
	}
	if c.Confidence != 0.90 {
		t.Errorf("confidence: got %.2f, want 0.90", c.Confidence)
	}
}

func TestMetadata_TypeReference(t *testing.T) {
	e := makeEntry("Active/notes.md", "active", 0, map[string]string{"type": "reference"})
	c := metadata(e, testCfg)
	if c == nil {
		t.Fatal("expected candidate for type:reference")
	}
	if c.SuggestedZone != "reference" {
		t.Errorf("zone: got %q, want %q", c.SuggestedZone, "reference")
	}
}

func TestMetadata_NoMatchReturnsNil(t *testing.T) {
	e := makeEntry("Active/work.md", "active", 0, map[string]string{"status": "in-progress"})
	c := metadata(e, testCfg)
	if c != nil {
		t.Errorf("expected nil for unknown status value, got %v", c)
	}
}

// ── pipeline ──────────────────────────────────────────────────────────────────

func TestClassify_StructuralBeatsMetadata(t *testing.T) {
	// File matches both a structural rule AND a metadata rule; structural runs first.
	e := makeEntry("Active/Reviews/done.md", "active", 50, map[string]string{"status": "complete"})
	candidates := Classify([]scanner.FileEntry{e}, testCfg, 0.0)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ClassifierUsed != "structural" {
		t.Errorf("expected structural classifier to win, got %q", candidates[0].ClassifierUsed)
	}
}

func TestClassify_MetadataFallback(t *testing.T) {
	// Frontmatter match, but no structural rule fires (recently modified, no path pattern).
	e := makeEntry("Active/project.md", "active", 1, map[string]string{"status": "complete"})
	candidates := Classify([]scanner.FileEntry{e}, testCfg, 0.0)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ClassifierUsed != "metadata" {
		t.Errorf("expected metadata classifier, got %q", candidates[0].ClassifierUsed)
	}
}

func TestClassify_MinConfidenceFilter(t *testing.T) {
	// PDF rule has confidence 0.70; filter at 0.80 should exclude it.
	e := makeEntry("Active/report.pdf", "active", 0, nil)
	candidates := Classify([]scanner.FileEntry{e}, testCfg, 0.80)
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates above 0.80, got %d", len(candidates))
	}
}
