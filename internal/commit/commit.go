package commit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aeonblue3/vorg/internal/triage"
)

// LinkMap maps a filename (without extension) to all vault .md files that reference it.
type LinkMap map[string][]string

// Execute moves files according to decisions and rewrites Obsidian links.
// vaultRoot is the arena root; set to "" to skip link rewriting.
func Execute(decisions []triage.Decision, vaultRoot string, logPath string, verbose bool) error {
	if len(decisions) == 0 {
		return nil
	}

	// Validate: no destination collisions.
	for _, d := range decisions {
		if _, err := os.Stat(d.Destination); err == nil {
			return fmt.Errorf("destination already exists: %s", d.Destination)
		}
	}

	// Build link map before any moves.
	var linkMap LinkMap
	if vaultRoot != "" {
		var err error
		linkMap, err = BuildLinkMap(vaultRoot)
		if err != nil {
			return fmt.Errorf("building link map: %w", err)
		}
	}

	// Open (or create) the log file.
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening log: %w", err)
	}
	defer logFile.Close()

	fmt.Println("\nMoving files...")

	for _, d := range decisions {
		src := d.Candidate.File.Path
		dst := d.Destination

		// Create destination directory.
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", dst, err)
		}

		// Move (rename first, fall back to copy+delete for cross-device).
		if err := moveFile(src, dst); err != nil {
			return fmt.Errorf("moving %s: %w", src, err)
		}

		linksRewritten := 0
		if linkMap != nil {
			base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
			if referrers, ok := linkMap[base]; ok {
				for _, ref := range referrers {
					if ref == src {
						continue // skip the moved file itself
					}
					n, err := rewriteLinks(ref, src, dst, vaultRoot)
					if err != nil && verbose {
						fmt.Printf("  ! rewrite error in %s: %v\n", ref, err)
					}
					linksRewritten += n
				}
			}
		}

		relSrc, _ := filepath.Rel(vaultRoot, src)
		if relSrc == "" {
			relSrc = src
		}
		relDst, _ := filepath.Rel(vaultRoot, dst)
		if relDst == "" {
			relDst = dst
		}

		fmt.Printf("  ✓ %s → %s", relSrc, relDst)
		if linksRewritten > 0 {
			fmt.Printf(" (rewrote %d links)", linksRewritten)
		}
		fmt.Println()

		ts := time.Now().UTC().Format(time.RFC3339)
		fmt.Fprintf(logFile, "%s\tMOVE\t%s\t%s → %s\tlinks_rewritten=%d\n",
			ts, "vault", relSrc, relDst, linksRewritten)
	}

	fmt.Printf("\nDone. %d moves committed. Log: %s\n", len(decisions), logPath)
	return nil
}

// BuildLinkMap scans all .md files under root and maps filename (no extension)
// → list of .md files that contain wikilinks or markdown links to it.
func BuildLinkMap(root string) (LinkMap, error) {
	lm := make(LinkMap)

	wikiRe := regexp.MustCompile(`\[\[([^\]|#]+)`)
	mdRe := regexp.MustCompile(`\[([^\]]*)\]\(([^)]+\.md)\)`)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)

		for _, m := range wikiRe.FindAllStringSubmatch(content, -1) {
			target := strings.TrimSpace(m[1])
			// Wikilink may contain a path; extract the base filename.
			target = filepath.Base(target)
			target = strings.TrimSuffix(target, filepath.Ext(target))
			lm[target] = appendUnique(lm[target], path)
		}

		for _, m := range mdRe.FindAllStringSubmatch(content, -1) {
			linkPath := m[2]
			base := strings.TrimSuffix(filepath.Base(linkPath), filepath.Ext(linkPath))
			lm[base] = appendUnique(lm[base], path)
		}

		return nil
	})
	return lm, err
}

// rewriteLinks updates wikilinks and markdown links in referrerPath that point
// to the moved file (src → dst), returning the count of links rewritten.
func rewriteLinks(referrerPath, src, dst, vaultRoot string) (int, error) {
	data, err := os.ReadFile(referrerPath)
	if err != nil {
		return 0, err
	}
	original := string(data)

	srcBase := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	dstBase := strings.TrimSuffix(filepath.Base(dst), filepath.Ext(dst))

	srcRel, _ := filepath.Rel(vaultRoot, src)
	srcRelNoExt := strings.TrimSuffix(srcRel, filepath.Ext(srcRel))
	dstRel, _ := filepath.Rel(vaultRoot, dst)
	dstRelNoExt := strings.TrimSuffix(dstRel, filepath.Ext(dstRel))

	updated := original
	count := 0

	// Rewrite [[filename]] and [[filename|display]]
	wikiRe := regexp.MustCompile(`\[\[([^\]|#]+)((?:\|[^\]]*)?)\]\]`)
	updated = wikiRe.ReplaceAllStringFunc(updated, func(match string) string {
		parts := wikiRe.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		target := strings.TrimSpace(parts[1])
		display := parts[2]

		targetBase := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
		if targetBase != srcBase {
			return match
		}

		// Preserve path vs bare-name style.
		var newTarget string
		if strings.Contains(target, "/") {
			newTarget = dstRelNoExt
		} else {
			newTarget = dstBase
		}
		count++
		return "[[" + newTarget + display + "]]"
	})

	// Rewrite [text](path.md) links.
	mdRe := regexp.MustCompile(`(\[([^\]]*)\]\()([^)]+\.md)(\))`)
	updated = mdRe.ReplaceAllStringFunc(updated, func(match string) string {
		parts := mdRe.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		linkPath := parts[3]
		base := strings.TrimSuffix(filepath.Base(linkPath), filepath.Ext(linkPath))
		if base != srcBase {
			return match
		}

		// Resolve to absolute, then back to relative from referrer.
		var absLink string
		if filepath.IsAbs(linkPath) {
			absLink = linkPath
		} else {
			absLink = filepath.Join(filepath.Dir(referrerPath), linkPath)
		}
		absLink = filepath.Clean(absLink)
		srcAbs := filepath.Clean(src)
		if absLink != srcAbs {
			return match
		}

		newRel, err := filepath.Rel(filepath.Dir(referrerPath), dst)
		if err != nil {
			newRel = dstRel
		}
		count++
		return parts[1] + newRel + parts[4]
	})

	_ = srcRelNoExt // used above in future path-style wikilink matching

	if updated == original {
		return 0, nil
	}

	if err := os.WriteFile(referrerPath, []byte(updated), 0644); err != nil {
		return 0, err
	}
	return count, nil
}

func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	// Cross-device: copy then delete.
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}
