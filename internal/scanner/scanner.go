package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chrisdias/vorg/internal/arena"
	"gopkg.in/yaml.v3"
)

// FileEntry represents a single file discovered during a scan.
type FileEntry struct {
	Path        string
	RelPath     string
	Zone        string
	ModTime     time.Time
	Size        int64
	IsDir       bool
	Frontmatter map[string]string
	Excerpt     string
}

// Options controls scanner behavior.
type Options struct {
	SkipHidden bool // skip files/dirs starting with '.'
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{SkipHidden: true}
}

// Scan walks the arena root and returns all discovered FileEntry records,
// sorted oldest-first.
func Scan(cfg *arena.ArenaConfig, opts Options) ([]FileEntry, error) {
	var entries []FileEntry

	err := filepath.WalkDir(cfg.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}

		rel, _ := filepath.Rel(cfg.Root, path)
		if rel == "." {
			return nil
		}

		name := d.Name()

		if opts.SkipHidden && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		fe := FileEntry{
			Path:    path,
			RelPath: rel,
			Zone:    zoneFor(rel, cfg.Zones),
			ModTime: info.ModTime(),
			Size:    info.Size(),
			IsDir:   d.IsDir(),
		}

		if !d.IsDir() && strings.EqualFold(filepath.Ext(name), ".md") {
			fe.Frontmatter, fe.Excerpt = parseMD(path)
		}

		entries = append(entries, fe)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ModTime.Before(entries[j].ModTime)
	})
	return entries, nil
}

// zoneFor returns the zone name whose path prefix most-specifically contains relPath.
func zoneFor(relPath string, zones []arena.ZoneConfig) string {
	best := ""
	bestLen := -1
	for _, z := range zones {
		zonePath := filepath.Clean(z.Path)
		if zonePath == "." {
			// root zone matches everything but only as fallback
			if bestLen < 0 {
				best = z.Name
				bestLen = 0
			}
			continue
		}
		prefix := zonePath + string(filepath.Separator)
		if strings.HasPrefix(relPath, prefix) || relPath == zonePath {
			if len(zonePath) > bestLen {
				best = z.Name
				bestLen = len(zonePath)
			}
		}
	}
	return best
}

// parseMD extracts YAML frontmatter and a content excerpt from a markdown file.
func parseMD(path string) (fm map[string]string, excerpt string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	content := string(data)

	if strings.HasPrefix(content, "---") {
		rest := content[3:]
		end := strings.Index(rest, "\n---")
		if end >= 0 {
			fmBlock := rest[:end]
			fm = parseYAMLFM(fmBlock)
			content = rest[end+4:]
		}
	}

	if len(content) > 500 {
		content = content[:500]
	}
	return fm, strings.TrimSpace(content)
}

func parseYAMLFM(block string) map[string]string {
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		case []interface{}:
			// Flatten string slices (e.g. tags) as comma-separated.
			parts := make([]string, 0, len(val))
			for _, item := range val {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			result[k] = strings.Join(parts, ",")
		default:
			if v != nil {
				result[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	return result
}
