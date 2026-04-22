package classifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chrisdias/vorg/internal/arena"
	"github.com/chrisdias/vorg/internal/scanner"
)

// ollamaAvailable probes the default Ollama port. Tests that require a live
// Ollama instance call t.Skip() when it isn't reachable.
func ollamaAvailable() bool {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// fakeOllama starts a test server that returns a fixed semantic result.
func fakeOllama(t *testing.T, zone string, confidence float64, reason string) *httptest.Server {
	t.Helper()
	payload := map[string]interface{}{
		"zone":       zone,
		"confidence": confidence,
		"reason":     reason,
	}
	inner, _ := json.Marshal(payload)
	body, _ := json.Marshal(map[string]string{"response": string(inner)})

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
}

var semanticCfg = &arena.ArenaConfig{
	Name: "vault",
	Root: "/vault",
	Zones: []arena.ZoneConfig{
		{Name: "inbox", Path: "Inbox"},
		{Name: "active", Path: "Active"},
		{Name: "reference", Path: "Reference"},
		{Name: "archive", Path: "Archive"},
	},
}

// ── unit tests with fake Ollama ───────────────────────────────────────────────

func TestSemantic_ReturnsCandidate(t *testing.T) {
	srv := fakeOllama(t, "archive", 0.88, "Completed project with no recent activity.")
	defer srv.Close()

	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: srv.URL,
		Threshold: 0.75,
	}

	fe := scanner.FileEntry{
		Path:    "/vault/Active/ambiguous.md",
		RelPath: "Active/ambiguous.md",
		Zone:    "active",
		Excerpt: "This was a project about widgets. It is now finished.",
	}

	c := semantic(fe, &cfg)
	if c == nil {
		t.Fatal("expected a candidate, got nil")
	}
	if c.SuggestedZone != "archive" {
		t.Errorf("zone: got %q, want %q", c.SuggestedZone, "archive")
	}
	if c.ClassifierUsed != "semantic" {
		t.Errorf("classifierUsed: got %q", c.ClassifierUsed)
	}
	if c.Confidence != 0.88 {
		t.Errorf("confidence: got %.2f, want 0.88", c.Confidence)
	}
}

func TestSemantic_BelowThresholdReturnsNil(t *testing.T) {
	srv := fakeOllama(t, "reference", 0.60, "Might be reference material.")
	defer srv.Close()

	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: srv.URL,
		Threshold: 0.75,
	}

	fe := scanner.FileEntry{
		Path:    "/vault/Active/ambiguous.md",
		RelPath: "Active/ambiguous.md",
		Excerpt: "Some text.",
	}

	c := semantic(fe, &cfg)
	if c != nil {
		t.Errorf("expected nil for low-confidence result, got candidate for zone %q", c.SuggestedZone)
	}
}

func TestSemantic_UnknownZoneReturnsNil(t *testing.T) {
	srv := fakeOllama(t, "limbo", 0.95, "Does not match any configured zone.")
	defer srv.Close()

	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: srv.URL,
		Threshold: 0.75,
	}

	fe := scanner.FileEntry{Path: "/vault/Active/x.md", RelPath: "Active/x.md", Excerpt: "x"}
	c := semantic(fe, &cfg)
	if c != nil {
		t.Errorf("expected nil for unknown zone, got candidate for zone %q", c.SuggestedZone)
	}
}

func TestSemantic_OllamaUnavailableReturnsNil(t *testing.T) {
	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: "http://127.0.0.1:19999", // nothing listening here
		Threshold: 0.75,
	}

	fe := scanner.FileEntry{Path: "/vault/Active/x.md", RelPath: "Active/x.md", Excerpt: "x"}
	c := semantic(fe, &cfg)
	if c != nil {
		t.Errorf("expected nil when Ollama is unavailable, got %v", c)
	}
}

func TestSemantic_DisabledReturnsNil(t *testing.T) {
	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{Enabled: false}

	fe := scanner.FileEntry{Path: "/vault/Active/x.md", RelPath: "Active/x.md", Excerpt: "x"}
	c := semantic(fe, &cfg)
	if c != nil {
		t.Error("expected nil when semantic is disabled")
	}
}

func TestSemantic_MalformedJSONReturnsNil(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"response": "not valid json at all"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: srv.URL,
		Threshold: 0.75,
	}

	fe := scanner.FileEntry{Path: "/vault/Active/x.md", RelPath: "Active/x.md", Excerpt: "x"}
	c := semantic(fe, &cfg)
	if c != nil {
		t.Error("expected nil for malformed JSON response")
	}
}

func TestBuildPrompt_ContainsZonesAndExcerpt(t *testing.T) {
	zones := []string{"- inbox", "- active", "- reference", "- archive"}
	fe := scanner.FileEntry{
		Excerpt:     "This is a note about widgets.",
		Frontmatter: map[string]string{"status": "complete"},
	}

	prompt := buildPrompt(zones, fe, 500)

	if !strings.Contains(prompt, "- archive") {
		t.Error("prompt missing zone names")
	}
	if !strings.Contains(prompt, "widgets") {
		t.Error("prompt missing excerpt content")
	}
	if !strings.Contains(prompt, "status: complete") {
		t.Error("prompt missing frontmatter")
	}
	if !strings.Contains(prompt, `{"zone"`) {
		t.Error("prompt missing JSON schema hint")
	}
}

// ── live Ollama integration test (skipped if Ollama not running) ──────────────

func TestSemantic_LiveOllama(t *testing.T) {
	if !ollamaAvailable() {
		t.Skip("Ollama not available on localhost:11434")
	}

	cfg := *semanticCfg
	cfg.Rules.Semantic = arena.SemanticConfig{
		Enabled:   true,
		Model:     "qwen2.5:3b",
		OllamaURL: "http://localhost:11434",
		Threshold: 0.50, // lower threshold for live test to avoid flakiness
	}

	fe := scanner.FileEntry{
		Path:    "/vault/Active/ambiguous.md",
		RelPath: "Active/ambiguous.md",
		Zone:    "active",
		Frontmatter: map[string]string{
			"status": "complete",
		},
		Excerpt: "Project retrospective notes from Q4. All deliverables shipped. Team feedback collected.",
	}

	c := semantic(fe, &cfg)
	if c == nil {
		t.Log("Ollama returned a low-confidence result (nil) — this is acceptable")
		return
	}

	t.Logf("Live result: zone=%q confidence=%.2f reason=%q", c.SuggestedZone, c.Confidence, c.Reason)

	validZones := map[string]bool{"inbox": true, "active": true, "reference": true, "archive": true}
	if !validZones[c.SuggestedZone] {
		t.Errorf("unexpected zone %q from live Ollama", c.SuggestedZone)
	}
}
