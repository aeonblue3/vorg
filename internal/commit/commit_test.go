package commit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildLinkMap(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "Active", "daily.md"),
		"See [[Widget Service]] and [[Another Note|display text]].")
	writeFile(t, filepath.Join(dir, "Active", "notes.md"),
		"Reference: [Widget Service](Reviews/Widget Service.md).")
	writeFile(t, filepath.Join(dir, "Active", "Reviews", "Widget Service.md"),
		"# Widget Service\nContent.")

	lm, err := BuildLinkMap(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(lm["Widget Service"]) != 2 {
		t.Errorf("Widget Service: expected 2 referrers, got %d: %v", len(lm["Widget Service"]), lm["Widget Service"])
	}
	if len(lm["Another Note"]) != 1 {
		t.Errorf("Another Note: expected 1 referrer, got %d", len(lm["Another Note"]))
	}
}

// TestRewriteLinks_PathWikilink tests that path-style wikilinks are updated when a file moves.
// Bare-name wikilinks ([[filename]]) do NOT need rewriting when the filename doesn't change,
// because Obsidian resolves them by searching for the filename regardless of location.
func TestRewriteLinks_PathWikilink(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "Active", "Reviews", "Widget Service.md")
	dst := filepath.Join(dir, "Archive", "Reviews", "Widget Service.md")

	referrer := filepath.Join(dir, "Active", "daily.md")
	writeFile(t, referrer,
		"See [[Active/Reviews/Widget Service]] for context.\nAlso [[Active/Reviews/Widget Service|display text]].")

	n, err := rewriteLinks(referrer, src, dst, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 path wikilinks rewritten, got %d", n)
	}

	result, _ := os.ReadFile(referrer)
	if strings.Contains(string(result), "Active/Reviews/Widget Service") {
		t.Error("path wikilink still contains old path")
	}
	if !strings.Contains(string(result), "Archive/Reviews/Widget Service|display text") {
		t.Error("display-text path wikilink not rewritten correctly")
	}
}

// TestRewriteLinks_BareWikilink verifies that bare [[filename]] links are left alone
// when the file moves but keeps the same name (Obsidian resolves by name, not path).
func TestRewriteLinks_BareWikilink(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "Active", "Reviews", "Widget Service.md")
	dst := filepath.Join(dir, "Archive", "Reviews", "Widget Service.md")

	referrer := filepath.Join(dir, "Active", "daily.md")
	original := "See [[Widget Service]] for context."
	writeFile(t, referrer, original)

	n, err := rewriteLinks(referrer, src, dst, dir)
	if err != nil {
		t.Fatal(err)
	}
	// A bare wikilink with the same filename doesn't need rewriting.
	if n != 0 {
		t.Errorf("expected 0 bare wikilinks rewritten (no-op), got %d", n)
	}
	result, _ := os.ReadFile(referrer)
	if string(result) != original {
		t.Error("file was modified when no rewrite was needed")
	}
}

func TestRewriteLinks_MarkdownLink(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "Active", "Reviews", "Widget Service.md")
	dst := filepath.Join(dir, "Archive", "Reviews", "Widget Service.md")

	referrer := filepath.Join(dir, "Active", "notes.md")
	writeFile(t, referrer, "See [Widget Service](Reviews/Widget Service.md) for details.")

	n, err := rewriteLinks(referrer, src, dst, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 markdown link rewritten, got %d", n)
	}

	result, _ := os.ReadFile(referrer)
	resultStr := string(result)
	if !strings.Contains(resultStr, "Archive/Reviews/Widget Service.md") {
		t.Errorf("markdown link not rewritten to archive path; got: %s", resultStr)
	}
	// The old inline path (without parent traversal) should be gone.
	if strings.Contains(resultStr, "(Reviews/Widget Service.md)") {
		t.Error("old markdown link still present after rewrite")
	}
}

func TestRewriteLinks_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "Active", "Widget Service.md")
	dst := filepath.Join(dir, "Archive", "Widget Service.md")

	referrer := filepath.Join(dir, "Active", "daily.md")
	original := "See [[Other Note]] for context."
	writeFile(t, referrer, original)

	n, err := rewriteLinks(referrer, src, dst, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 links rewritten (no match), got %d", n)
	}
	result, _ := os.ReadFile(referrer)
	if string(result) != original {
		t.Error("file was modified when no links matched")
	}
}

func TestMoveFile_SameDevice(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "subdir", "dest.md")

	writeFile(t, src, "content")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(src, dst); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file still exists after move")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Errorf("destination content: got %q", string(data))
	}
}
