package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aeonblue3/vorg/internal/arena"
	"gopkg.in/yaml.v3"
)

// DefaultConfigDir returns the default vorg config directory.
func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vorg")
}

// DefaultLogPath returns the path for the move log.
func DefaultLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "vorg", "vorg.log")
}

// SkipListPath returns the path for the skip list.
func SkipListPath(configDir string) string {
	return filepath.Join(configDir, "skip.yaml")
}

// LoadArena loads a single arena config by name from the arenas subdirectory.
func LoadArena(configDir, name string) (*arena.ArenaConfig, error) {
	path := filepath.Join(configDir, "arenas", name+".yaml")
	return LoadArenaFile(path)
}

// LoadArenaFile loads an arena config from an explicit path.
func LoadArenaFile(path string) (*arena.ArenaConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading arena config %s: %w", path, err)
	}
	var cfg arena.ArenaConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing arena config %s: %w", path, err)
	}
	if err := expandPaths(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListArenas returns all arena names found in the arenas subdirectory.
func ListArenas(configDir string) ([]string, error) {
	dir := filepath.Join(configDir, "arenas")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return names, nil
}

// Validate checks the arena config for common problems.
func Validate(cfg *arena.ArenaConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("arena name is required")
	}
	if cfg.Root == "" {
		return fmt.Errorf("arena root is required")
	}
	info, err := os.Stat(cfg.Root)
	if err != nil {
		return fmt.Errorf("arena root %q does not exist: %w", cfg.Root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("arena root %q is not a directory", cfg.Root)
	}
	seen := map[string]bool{}
	for _, z := range cfg.Zones {
		if seen[z.Name] {
			return fmt.Errorf("duplicate zone name %q", z.Name)
		}
		seen[z.Name] = true
	}
	for _, r := range cfg.Rules.Structural {
		if r.Suggest != "" && !seen[r.Suggest] {
			return fmt.Errorf("structural rule references unknown zone %q", r.Suggest)
		}
	}
	for _, r := range cfg.Rules.Metadata {
		if r.Suggest != "" && !seen[r.Suggest] {
			return fmt.Errorf("metadata rule references unknown zone %q", r.Suggest)
		}
	}
	return nil
}

func expandPaths(cfg *arena.ArenaConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg.Root = expandHome(cfg.Root, home)
	for i := range cfg.Zones {
		cfg.Zones[i].Path = expandHome(cfg.Zones[i].Path, home)
	}
	return nil
}

func expandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return filepath.Clean(p)
}
