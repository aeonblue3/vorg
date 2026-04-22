package triage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chrisdias/vorg/internal/classifier"
	"golang.org/x/term"
)

// Decision records the human outcome for a single candidate.
type Decision struct {
	Candidate   classifier.Candidate
	Action      string // "archive", "move", "keep", "skip"
	Destination string
}

// Run presents candidates one-by-one and collects decisions.
// Returns decisions only for candidates marked "archive" or "move".
func Run(candidates []classifier.Candidate, dryRun bool) ([]Decision, error) {
	if len(candidates) == 0 {
		fmt.Println("No candidates found.")
		return nil, nil
	}

	// Switch terminal to raw mode so we can read single keypresses.
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to line-mode if raw mode fails (e.g. piped input).
		return runLineMode(candidates, dryRun)
	}
	defer term.Restore(fd, oldState)

	var decisions []Decision
	var skipList []string

	for i, c := range candidates {
		// Check skip list.
		skipped := false
		for _, s := range skipList {
			if s == c.File.RelPath {
				skipped = true
				break
			}
		}
		if skipped {
			continue
		}

		term.Restore(fd, oldState)
		printCandidate(i+1, len(candidates), c)

		rawAgain, _ := term.MakeRaw(fd)

		action, dest, err := readDecision(c, rawAgain != nil)
		if rawAgain != nil {
			term.Restore(fd, rawAgain)
		}

		if err != nil {
			term.Restore(fd, oldState)
			return decisions, err
		}

		switch action {
		case "quit":
			term.Restore(fd, oldState)
			goto summary
		case "skip":
			skipList = append(skipList, c.File.RelPath)
			continue
		case "keep":
			// no decision recorded
		case "archive", "move":
			decisions = append(decisions, Decision{
				Candidate:   c,
				Action:      action,
				Destination: dest,
			})
		}
	}

summary:
	term.Restore(fd, oldState)
	return confirmAndReturn(decisions, dryRun)
}

func printCandidate(n, total int, c classifier.Candidate) {
	sep := strings.Repeat("─", 60)
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("[%d/%d] %s CANDIDATE\n", n, total, strings.ToUpper(c.SuggestedZone))
	fmt.Printf("  File:       %s\n", c.File.RelPath)
	fmt.Printf("  Reason:     %s\n", c.Reason)
	fmt.Printf("  Confidence: %.0f%%\n", c.Confidence*100)
	if len(c.InboundLinks) > 0 {
		fmt.Printf("  Links:      %d inbound wikilinks will be rewritten\n", len(c.InboundLinks))
	}
	fmt.Printf("\n  Suggested destination:\n  %s\n", c.SuggestedPath)
	fmt.Printf("\n  [a] approve  [k] keep  [e] edit destination  [s] skip always  [?] help  [q] quit\n")
	fmt.Printf("> ")
}

func readDecision(c classifier.Candidate, rawMode bool) (action, dest string, err error) {
	dest = c.SuggestedPath

	reader := bufio.NewReader(os.Stdin)

	for {
		var key string
		if rawMode {
			b := make([]byte, 1)
			_, err := os.Stdin.Read(b)
			if err != nil {
				return "quit", dest, err
			}
			key = string(b)
		} else {
			line, err := reader.ReadString('\n')
			if err != nil {
				return "quit", dest, err
			}
			key = strings.TrimSpace(line)
			if len(key) > 0 {
				key = string(key[0])
			}
		}

		switch strings.ToLower(key) {
		case "a":
			fmt.Println("a")
			return "archive", dest, nil
		case "k":
			fmt.Println("k")
			return "keep", dest, nil
		case "s":
			fmt.Println("s")
			return "skip", dest, nil
		case "q", "\x03": // q or Ctrl-C
			fmt.Println("q")
			return "quit", dest, nil
		case "e":
			fmt.Println("e")
			newDest, err := editDestination(dest, rawMode)
			if err != nil {
				return "quit", dest, err
			}
			dest = newDest
			fmt.Printf("  Updated destination: %s\n", dest)
			fmt.Printf("  [a] approve  [k] keep  [e] edit destination  [s] skip always  [?] help  [q] quit\n> ")
		case "?":
			fmt.Println("?")
			printHelp()
			fmt.Printf("> ")
		default:
			// ignore unknown keys in raw mode, re-prompt
			fmt.Printf("\r> ")
		}
	}
}

func editDestination(current string, rawMode bool) (string, error) {
	// Temporarily restore cooked mode for text entry.
	if rawMode {
		fd := int(os.Stdin.Fd())
		state, err := term.GetState(fd)
		if err == nil {
			defer term.Restore(fd, state)
		}
		// We can't restore to cooked from raw without the old state.
		// Just read a full line in raw mode character by character.
	}
	fmt.Printf("\n  Edit destination (Enter to confirm):\n  [%s]\n  New: ", current)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return current, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return current, nil
	}
	return filepath.Clean(line), nil
}

func printHelp() {
	fmt.Println()
	fmt.Println("  a — approve move to suggested destination")
	fmt.Println("  k — keep file in current location")
	fmt.Println("  e — edit the suggested destination path")
	fmt.Println("  s — skip this file in all future sessions")
	fmt.Println("  ? — show this help")
	fmt.Println("  q — quit session without committing remaining decisions")
}

func confirmAndReturn(decisions []Decision, dryRun bool) ([]Decision, error) {
	skipped := 0
	kept := 0
	var toCommit []Decision
	for _, d := range decisions {
		switch d.Action {
		case "archive", "move":
			toCommit = append(toCommit, d)
		case "keep":
			kept++
		case "skip":
			skipped++
		}
	}

	fmt.Printf("\n%d candidates reviewed. Queue: %d to move, %d kept, %d skipped.\n",
		len(decisions), len(toCommit), kept, skipped)

	if len(toCommit) == 0 {
		fmt.Println("Nothing to commit.")
		return nil, nil
	}

	if dryRun {
		fmt.Println("(dry-run mode — no changes will be made)")
		for _, d := range toCommit {
			fmt.Printf("  would move: %s → %s\n", d.Candidate.File.RelPath, d.Destination)
		}
		return nil, nil
	}

	fmt.Printf("\nCommit %d moves? [y/N] ", len(toCommit))
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return nil, nil
	}

	return toCommit, nil
}

// runLineMode is the fallback when raw terminal mode is unavailable.
func runLineMode(candidates []classifier.Candidate, dryRun bool) ([]Decision, error) {
	var decisions []Decision
	reader := bufio.NewReader(os.Stdin)

	for i, c := range candidates {
		printCandidate(i+1, len(candidates), c)
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		key := strings.TrimSpace(strings.ToLower(line))
		if len(key) == 0 {
			continue
		}
		switch string(key[0]) {
		case "a":
			decisions = append(decisions, Decision{Candidate: c, Action: "archive", Destination: c.SuggestedPath})
		case "k":
			// keep — no decision
		case "q":
			goto done
		}
	}
done:
	return confirmAndReturn(decisions, dryRun)
}
