package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkipList is the set of relative paths permanently excluded from triage.
type SkipList struct {
	path    string
	entries map[string]bool
}

// LoadSkipList reads skip.yaml from configDir, creating an empty list if absent.
func LoadSkipList(configDir string) (*SkipList, error) {
	p := filepath.Join(configDir, "skip.yaml")
	sl := &SkipList{path: p, entries: make(map[string]bool)}

	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return sl, nil
	}
	if err != nil {
		return nil, err
	}

	var paths []string
	if err := yaml.Unmarshal(data, &paths); err != nil {
		return nil, err
	}
	for _, p := range paths {
		sl.entries[p] = true
	}
	return sl, nil
}

// Contains reports whether relPath is in the skip list.
func (sl *SkipList) Contains(relPath string) bool {
	return sl.entries[relPath]
}

// Add appends relPath and persists the list to disk.
func (sl *SkipList) Add(relPath string) error {
	if sl.entries[relPath] {
		return nil
	}
	sl.entries[relPath] = true
	return sl.save()
}

func (sl *SkipList) save() error {
	if err := os.MkdirAll(filepath.Dir(sl.path), 0755); err != nil {
		return err
	}
	var paths []string
	for p := range sl.entries {
		paths = append(paths, p)
	}
	// Stable output: sort by splitting on separator.
	sortStrings(paths)
	data, err := yaml.Marshal(paths)
	if err != nil {
		return err
	}
	return os.WriteFile(sl.path, data, 0644)
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && strings.ToLower(ss[j]) < strings.ToLower(ss[j-1]); j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
