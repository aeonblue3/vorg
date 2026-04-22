# vorg

A periodic defrag tool for file system arenas. Scans directories for files that appear to be in the wrong zone, presents them for human triage via a single-keypress session, and commits approved moves atomically. For Obsidian vaults, it rewrites all wikilinks and markdown links that reference moved files.

---

## How it works

Every arena (vault, downloads, home directory) is organized into named zones — Inbox, Active, Reference, Archive, or whatever fits your structure. vorg scans the arena, runs files through a three-signal classification pipeline, and surfaces candidates with suggested destinations.

**Signal order:**

1. **Structural** — path pattern, file extension, modification age. Free, instant, covers ~70% of cases. ("A review folder untouched for 45 days is an archive candidate.")
2. **Metadata** — YAML frontmatter fields in markdown files. Deterministic once you adopt conventions. (`status: complete` → archive.)
3. **Semantic** — Ollama LLM, only reached when structural and metadata signals are inconclusive. Optional. Skipped gracefully if Ollama is not running.

Nothing moves without your explicit approval. Decisions accumulate in memory during the session; moves execute atomically at the end after a final confirmation.

---

## Install

```sh
git clone https://github.com/aeonblue3/vorg
cd vorg
go build -o vorg ./cmd/vorg

# Put the binary somewhere on your PATH
mv vorg /usr/local/bin/vorg
```

Requires Go 1.22+. Compiles to a single static binary with no runtime dependencies.

---

## Quick start

**1. Create your config directory:**

```sh
mkdir -p ~/.config/vorg/arenas
```

**2. Copy and edit an example config:**

```sh
cp config/examples/vault.yaml ~/.config/vorg/arenas/vault.yaml
# Edit root: to point at your vault
```

**3. Validate:**

```sh
vorg config validate
```

**4. Preview candidates without triage:**

```sh
vorg scan
```

**5. Run a triage session:**

```sh
vorg triage
```

---

## Commands

```
vorg scan [--arena <name>]          Scan and print candidates, no triage
vorg triage [--arena <name>]        Interactive triage session
vorg status [--arena <name>]        Summary: files and candidate count
vorg status --all                   Summary across all configured arenas
vorg log [--tail <n>]               Show recent moves from the log
vorg config validate                Validate all arena configs
```

**Global flags:**

```
--config <path>    Config directory (default: ~/.config/vorg)
--arena <name>     Arena to use (default: vault)
--dry-run          Show what would happen, execute nothing
--verbose          Extra output during scan and classify
```

---

## Triage session

```
vorg triage --arena vault

Scanning ~/Documents/Obsidian/Main... 847 files, 12 candidates found.

────────────────────────────────────────────────────────────
[1/12] ARCHIVE CANDIDATE
  File:       Active/Reviews/Widget Service.md
  Reason:     Review folder idle >45 days (52 days inactive)
  Confidence: 85%
  Links:      3 inbound wikilinks will be rewritten

  Suggested destination:
  ~/Documents/Obsidian/Main/Archive/Widget Service.md

  [a] approve  [k] keep  [e] edit destination  [s] skip always  [?] help  [q] quit
```

**Keys:**

| Key | Action |
|-----|--------|
| `a` | Approve the suggested move |
| `k` | Keep the file where it is |
| `e` | Edit the destination path before approving |
| `s` | Skip this file in all future sessions (persisted to `skip.yaml`) |
| `?` | Show help |
| `q` | Quit — decisions so far are still committed after confirmation |

At the end of the session, vorg prints a summary and asks for final `y/N` confirmation before executing any moves.

---

## Configuration

### Directory layout

```
~/.config/vorg/
├── arenas/
│   ├── vault.yaml
│   ├── downloads.yaml
│   └── home.yaml
└── skip.yaml          # files permanently excluded from triage

~/.local/share/vorg/
└── vorg.log           # append-only move log
```

### Arena config structure

```yaml
arena: vault                          # arena name (matches filename)
root: ~/Documents/Obsidian/Main       # root path of the arena
obsidian: true                        # enables wikilink rewriting on commit

zones:
  - name: inbox
    path: Inbox                       # relative to root, or absolute path
  - name: active
    path: Active
  - name: reference
    path: Reference
  - name: archive
    path: Archive

rules:
  structural:   [...]
  metadata:     [...]
  semantic:     {...}
```

`obsidian: true` enables Obsidian wikilink rewriting. When a file is moved, vorg scans all `.md` files in the vault for `[[wikilinks]]` and `[text](path.md)` links that reference the moved file and rewrites them. Omit or set to `false` for non-Obsidian arenas.

Zone paths can be relative (resolved under `root`) or absolute (e.g. `~/.Trash`).

---

### Structural rules

Match files by location, age, and type.

```yaml
rules:
  structural:
    - description: "Review folder idle >45 days"
      zone: active              # only match files currently in this zone (optional)
      path_pattern: "Reviews/"  # substring match on relative path (optional)
      exclude_path: "Resources/" # skip files whose path contains this (optional)
      age_days: 45              # file must be older than this many days (optional)
      extensions: [".pdf", ".png"]  # file must have one of these extensions (optional)
      suggest: archive          # target zone name
      confidence: 0.85          # 0.0–1.0; surface threshold
```

All conditions on a rule must match simultaneously. A rule with only `suggest` and `confidence` matches every file.

---

### Metadata rules

Match markdown files by YAML frontmatter field values.

```yaml
rules:
  metadata:
    - field: status
      value: complete
      suggest: archive
      confidence: 0.90

    - field: type
      value: reference
      suggest: reference
      confidence: 0.90
```

Array frontmatter fields (e.g. `tags: [a, b, c]`) are matched as comma-separated strings (`"a,b,c"`).

Structural rules are evaluated first. A metadata rule only fires when no structural rule produces a confident match.

---

### Semantic rules (Ollama)

Classify ambiguous files using a local LLM. Only reached when both structural and metadata signals are inconclusive.

```yaml
rules:
  semantic:
    enabled: true
    model: "qwen2.5:3b"                  # any model available in your Ollama installation
    ollama_url: "http://localhost:11434"  # default; omit to use this value
    threshold: 0.75                      # minimum confidence to surface a candidate
    max_tokens: 500                      # excerpt length sent to the model
```

vorg skips semantic classification gracefully when Ollama is not running — no error, no hang. To disable it explicitly, set `enabled: false`.

The model is sent the zone descriptions, the file's frontmatter, and a content excerpt. It returns a zone, confidence score, and one-sentence reason. The reason is shown in the triage session.

**Recommended models:** `qwen2.5:3b` (fast, good quality), `llama3.2:3b`, `mistral:7b`.

To install Ollama: https://ollama.ai — then `ollama pull qwen2.5:3b`.

---

## Example configs

### Obsidian vault

```yaml
arena: vault
root: ~/Documents/Obsidian/Main
obsidian: true

zones:
  - name: inbox
    path: Inbox
  - name: active
    path: Active
  - name: reference
    path: Reference
  - name: archive
    path: Archive

rules:
  structural:
    - description: "Review folder idle >45 days"
      zone: active
      path_pattern: "Reviews/"
      age_days: 45
      suggest: archive
      confidence: 0.85

    - description: "Any file in Unsorted/"
      path_pattern: "Unsorted/"
      suggest: inbox
      confidence: 0.90

    - description: "Non-markdown outside Resources/"
      extensions: [".pdf", ".png", ".jpg", ".zip"]
      exclude_path: "Resources/"
      suggest: reference
      confidence: 0.70

  metadata:
    - field: status
      value: complete
      suggest: archive
      confidence: 0.90

    - field: type
      value: reference
      suggest: reference
      confidence: 0.90

  semantic:
    enabled: true
    model: "qwen2.5:3b"
    threshold: 0.75
    max_tokens: 500
```

### Downloads

```yaml
arena: downloads
root: ~/Downloads

zones:
  - name: staging
    path: .
  - name: documents
    path: ~/Documents
  - name: trash
    path: ~/.Trash

rules:
  structural:
    - description: "PDFs older than 7 days"
      extensions: [".pdf"]
      age_days: 7
      suggest: documents
      confidence: 0.80

    - description: "Disk images older than 1 day"
      extensions: [".dmg", ".iso"]
      age_days: 1
      suggest: trash
      confidence: 0.85

    - description: "Archives older than 3 days"
      extensions: [".zip", ".tar.gz", ".tgz"]
      age_days: 3
      suggest: trash
      confidence: 0.70

  semantic:
    enabled: false
```

---

## Move log

Every committed move is appended to `~/.local/share/vorg/vorg.log`:

```
2026-04-21T14:32:11Z  MOVE  vault  Active/Reviews/Widget Service.md → Archive/Widget Service.md  links_rewritten=3
```

View recent entries:

```sh
vorg log
vorg log --tail 50
```

---

## Skip list

Pressing `s` during triage permanently adds a file to `~/.config/vorg/skip.yaml`. Skipped files are filtered before the session starts — they will never be surfaced again.

To un-skip a file, remove its entry from `skip.yaml`.

---

## Design principles

- **Never move without approval.** Decisions accumulate in memory; nothing touches the filesystem until you confirm at the end of the session.
- **Structural rules first, AI last.** Cheaper signals are always tried before Ollama. Most files are classified without any LLM call.
- **Conservative thresholds.** A smaller set of high-confidence candidates is better than a noisy one. Tune `confidence` values in your rules to match your tolerance.
- **Config-driven, arena-agnostic.** Zone definitions and classification rules live in YAML. The engine is the same regardless of whether you're organizing a vault, Downloads, or a home directory.
- **Always log.** Every move is recorded. The log is append-only.
