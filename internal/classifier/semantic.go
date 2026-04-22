package classifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chrisdias/vorg/internal/arena"
	"github.com/chrisdias/vorg/internal/scanner"
)

const defaultOllamaURL = "http://localhost:11434"
const ollamaTimeout = 30 * time.Second

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

type semanticResult struct {
	Zone       string  `json:"zone"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// semantic calls Ollama to classify a file whose structural and metadata signals
// were inconclusive. Returns nil if Ollama is unavailable, the response is
// unparseable, or confidence is below the configured threshold.
func semantic(fe scanner.FileEntry, cfg *arena.ArenaConfig) *Candidate {
	sc := cfg.Rules.Semantic
	if !sc.Enabled {
		return nil
	}

	ollamaURL := sc.OllamaURL
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}

	zoneNames := make([]string, len(cfg.Zones))
	for i, z := range cfg.Zones {
		zoneNames[i] = fmt.Sprintf("- %s", z.Name)
	}

	prompt := buildPrompt(zoneNames, fe, sc.MaxTokens)

	result, err := callOllama(ollamaURL, sc.Model, prompt)
	if err != nil {
		// Ollama unavailable or timed out — silently skip.
		return nil
	}

	threshold := sc.Threshold
	if threshold <= 0 {
		threshold = 0.75
	}
	if result.Confidence < threshold {
		return nil
	}

	// Validate that the returned zone is one we know about.
	suggestedPath := ""
	for _, z := range cfg.Zones {
		if z.Name == result.Zone {
			suggestedPath = filepath.Join(cfg.Root, z.Path, filepath.Base(fe.Path))
			break
		}
	}
	if suggestedPath == "" {
		return nil // zone name Ollama returned doesn't match any configured zone
	}

	return &Candidate{
		File:           fe,
		SuggestedZone:  result.Zone,
		SuggestedPath:  suggestedPath,
		Confidence:     result.Confidence,
		Reason:         result.Reason,
		ClassifierUsed: "semantic",
	}
}

func buildPrompt(zoneNames []string, fe scanner.FileEntry, maxTokens int) string {
	excerpt := fe.Excerpt
	if maxTokens > 0 && len(excerpt) > maxTokens*4 { // rough chars-per-token estimate
		excerpt = excerpt[:maxTokens*4]
	}

	fmStr := formatFrontmatter(fe.Frontmatter)

	return fmt.Sprintf(`You are classifying a note in an Obsidian vault into one of these zones:
%s

Note frontmatter:
%s

Note excerpt:
%s

Respond with JSON only, no other text:
{"zone": "<zone>", "confidence": <0.0-1.0>, "reason": "<one sentence>"}`,
		strings.Join(zoneNames, "\n"),
		fmStr,
		excerpt,
	)
}

func formatFrontmatter(fm map[string]string) string {
	if len(fm) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s: %s\n", k, fm[k])
	}
	return strings.TrimRight(sb.String(), "\n")
}

func callOllama(baseURL, model, prompt string) (*semanticResult, error) {
	reqBody, err := json.Marshal(ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: ollamaTimeout}
	resp, err := client.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err // connection refused, timeout, etc.
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decoding ollama response: %w", err)
	}

	// Extract the JSON object from the response text.
	// Ollama may wrap the JSON in prose, so find the first {...}.
	raw := strings.TrimSpace(ollamaResp.Response)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object in ollama response: %q", raw)
	}
	raw = raw[start : end+1]

	var result semanticResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parsing semantic result: %w", err)
	}

	result.Zone = strings.TrimSpace(strings.ToLower(result.Zone))
	result.Reason = strings.TrimSpace(result.Reason)
	return &result, nil
}
