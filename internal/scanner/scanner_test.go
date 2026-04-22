package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeonblue3/vorg/internal/arena"
)

func TestParseMD_FrontmatterString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	content := "---\nstatus: complete\ntype: reference\n---\nBody text here."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fm, excerpt := parseMD(path)
	if fm["status"] != "complete" {
		t.Errorf("status: got %q, want %q", fm["status"], "complete")
	}
	if fm["type"] != "reference" {
		t.Errorf("type: got %q, want %q", fm["type"], "reference")
	}
	if excerpt != "Body text here." {
		t.Errorf("excerpt: got %q", excerpt)
	}
}

func TestParseMD_FrontmatterTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	content := "---\ntags: [go, programming, tools]\n---\nContent."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fm, _ := parseMD(path)
	if fm["tags"] != "go,programming,tools" {
		t.Errorf("tags: got %q, want %q", fm["tags"], "go,programming,tools")
	}
}

func TestParseMD_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("Just a plain note."), 0644); err != nil {
		t.Fatal(err)
	}

	fm, excerpt := parseMD(path)
	if len(fm) != 0 {
		t.Errorf("expected no frontmatter, got %v", fm)
	}
	if excerpt != "Just a plain note." {
		t.Errorf("excerpt: got %q", excerpt)
	}
}

func TestScan_ZoneAssignment(t *testing.T) {
	dir := t.TempDir()
	makeFile := func(relPath, content string) {
		p := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	makeFile("Inbox/capture.md", "captured")
	makeFile("Active/work.md", "---\nstatus: active\n---\nWIP")
	makeFile("Archive/done.md", "---\nstatus: complete\n---\nDone")

	cfg := &arena.ArenaConfig{
		Name: "test",
		Root: dir,
		Zones: []arena.ZoneConfig{
			{Name: "inbox", Path: "Inbox"},
			{Name: "active", Path: "Active"},
			{Name: "archive", Path: "Archive"},
		},
	}

	entries, err := Scan(cfg, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	byRel := map[string]FileEntry{}
	for _, e := range entries {
		byRel[e.RelPath] = e
	}

	cases := []struct{ rel, zone string }{
		{"Inbox/capture.md", "inbox"},
		{"Active/work.md", "active"},
		{"Archive/done.md", "archive"},
	}
	for _, tc := range cases {
		e, ok := byRel[tc.rel]
		if !ok {
			t.Errorf("file %q not found in scan results", tc.rel)
			continue
		}
		if e.Zone != tc.zone {
			t.Errorf("%s: zone = %q, want %q", tc.rel, e.Zone, tc.zone)
		}
	}
}

func TestScan_OldestFirst(t *testing.T) {
	dir := t.TempDir()

	paths := []string{"a.md", "b.md", "c.md"}
	for i, name := range paths {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		// Stagger modification times.
		mtime := time.Now().Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &arena.ArenaConfig{Name: "test", Root: dir}
	entries, err := Scan(cfg, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i < len(entries); i++ {
		if entries[i].ModTime.Before(entries[i-1].ModTime) {
			t.Errorf("entries not sorted oldest-first at index %d", i)
		}
	}
}
