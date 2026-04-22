package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chrisdias/vorg/internal/arena"
	"github.com/chrisdias/vorg/internal/classifier"
	"github.com/chrisdias/vorg/internal/commit"
	"github.com/chrisdias/vorg/internal/config"
	"github.com/chrisdias/vorg/internal/scanner"
	"github.com/chrisdias/vorg/internal/triage"
	"github.com/spf13/cobra"
)

var (
	flagConfigDir string
	flagArena     string
	flagDryRun    bool
	flagVerbose   bool
)

func main() {
	root := &cobra.Command{
		Use:   "vorg",
		Short: "vault organizer — periodic defrag for your file system arenas",
	}

	root.PersistentFlags().StringVar(&flagConfigDir, "config", config.DefaultConfigDir(), "config directory")
	root.PersistentFlags().StringVar(&flagArena, "arena", "vault", "arena name")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show what would happen, execute nothing")
	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "extra output")

	root.AddCommand(scanCmd(), triageCmd(), statusCmd(), logCmd(), configCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadArena() (*arena.ArenaConfig, error) {
	cfg, err := config.LoadArena(flagConfigDir, flagArena)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid arena config: %w", err)
	}
	return cfg, nil
}

// ── scan ─────────────────────────────────────────────────────────────────────

func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "scan and print candidates, no triage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadArena()
			if err != nil {
				return err
			}

			entries, err := scanner.Scan(cfg, scanner.DefaultOptions())
			if err != nil {
				return err
			}
			fmt.Printf("Scanned %d files in %s.\n", len(entries), cfg.Root)

			candidates := classifier.Classify(entries, cfg, 0.0)
			if len(candidates) == 0 {
				fmt.Println("No candidates found.")
				return nil
			}
			fmt.Printf("%d candidates:\n\n", len(candidates))
			for i, c := range candidates {
				fmt.Printf("[%d] %s\n    Reason:     %s\n    Classifier: %s  Confidence: %.0f%%\n    Suggested:  %s\n\n",
					i+1, c.File.RelPath, c.Reason, c.ClassifierUsed, c.Confidence*100, c.SuggestedPath)
			}
			return nil
		},
	}
}

// ── triage ───────────────────────────────────────────────────────────────────

func triageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "triage",
		Short: "interactive triage session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadArena()
			if err != nil {
				return err
			}

			fmt.Printf("Scanning %s...\n", cfg.Root)
			entries, err := scanner.Scan(cfg, scanner.DefaultOptions())
			if err != nil {
				return err
			}

			candidates := classifier.Classify(entries, cfg, 0.0)
			fmt.Printf("%d files scanned, %d candidates found.\n\n", len(entries), len(candidates))

			if len(candidates) == 0 {
				fmt.Println("Nothing to triage.")
				return nil
			}

			// Populate inbound links — only for Obsidian arenas.
			if cfg.Obsidian {
				linkMap, err := commit.BuildLinkMap(cfg.Root)
				if err != nil && flagVerbose {
					fmt.Printf("Warning: could not build link map: %v\n", err)
				}
				for i, c := range candidates {
					base := basenameNoExt(c.File.Path)
					if refs, ok := linkMap[base]; ok {
						candidates[i].InboundLinks = refs
					}
				}
			}

			skipList, err := config.LoadSkipList(flagConfigDir)
			if err != nil && flagVerbose {
				fmt.Printf("Warning: could not load skip list: %v\n", err)
			}
			decisions, err := triage.Run(candidates, skipList, flagDryRun)
			if err != nil {
				return err
			}
			if len(decisions) == 0 {
				return nil
			}

			// Pass vault root only for Obsidian arenas; empty string skips link rewriting.
			vaultRoot := ""
			if cfg.Obsidian {
				vaultRoot = cfg.Root
			}
			return commit.Execute(decisions, vaultRoot, config.DefaultLogPath(), flagVerbose)
		},
	}
}

// ── status ───────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "summary of how many files appear out of place",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				return statusAll()
			}
			cfg, err := loadArena()
			if err != nil {
				return err
			}
			return printArenaStatus(cfg)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "show status for all configured arenas")
	return cmd
}

func printArenaStatus(cfg *arena.ArenaConfig) error {
	entries, err := scanner.Scan(cfg, scanner.DefaultOptions())
	if err != nil {
		return fmt.Errorf("scanning %s: %w", cfg.Name, err)
	}
	candidates := classifier.Classify(entries, cfg, 0.0)
	obsidianTag := ""
	if cfg.Obsidian {
		obsidianTag = " [obsidian]"
	}
	fmt.Printf("%-14s %s%s\n", cfg.Name+obsidianTag, cfg.Root, "")
	fmt.Printf("  %d files, %d candidates", len(entries), len(candidates))
	if len(candidates) > 0 {
		fmt.Printf(" — run 'vorg triage --arena %s' to review", cfg.Name)
	}
	fmt.Println()
	return nil
}

func statusAll() error {
	names, err := config.ListArenas(flagConfigDir)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("No arena configs found in %s\n", flagConfigDir)
		return nil
	}

	fmt.Printf("vorg status — %d arenas\n\n", len(names))
	anyErr := false
	for _, name := range names {
		cfg, err := config.LoadArena(flagConfigDir, name)
		if err != nil {
			fmt.Printf("  %-14s error: %v\n", name, err)
			anyErr = true
			continue
		}
		if err := config.Validate(cfg); err != nil {
			fmt.Printf("  %-14s invalid config: %v\n", name, err)
			anyErr = true
			continue
		}
		if err := printArenaStatus(cfg); err != nil {
			fmt.Printf("  %-14s scan error: %v\n", name, err)
			anyErr = true
		}
	}
	if anyErr {
		return fmt.Errorf("one or more arenas had errors")
	}
	return nil
}

// ── log ──────────────────────────────────────────────────────────────────────

func logCmd() *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "show recent moves from the log",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath := config.DefaultLogPath()
			data, err := os.ReadFile(logPath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No log file found.")
					return nil
				}
				return err
			}
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			if tail > 0 && len(lines) > tail {
				lines = lines[len(lines)-tail:]
			}
			for _, l := range lines {
				fmt.Println(l)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 20, "number of recent entries to show")
	return cmd
}

// ── config validate ───────────────────────────────────────────────────────────

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "config management",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "validate all arena configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := config.ListArenas(flagConfigDir)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Println("No arena configs found in", flagConfigDir)
				return nil
			}
			allOK := true
			for _, name := range names {
				cfg, err := config.LoadArena(flagConfigDir, name)
				if err != nil {
					fmt.Printf("  ✗ %s: %v\n", name, err)
					allOK = false
					continue
				}
				if err := config.Validate(cfg); err != nil {
					fmt.Printf("  ✗ %s: %v\n", name, err)
					allOK = false
					continue
				}
				fmt.Printf("  ✓ %s\n", name)
			}
			if !allOK {
				return fmt.Errorf("one or more arena configs are invalid")
			}
			return nil
		},
	})
	return cmd
}

// ── helpers ──────────────────────────────────────────────────────────────────

func basenameNoExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
