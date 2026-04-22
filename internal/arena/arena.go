package arena

// ArenaConfig is the top-level configuration for a single file system arena.
type ArenaConfig struct {
	Name  string       `yaml:"arena"`
	Root  string       `yaml:"root"`
	Zones []ZoneConfig `yaml:"zones"`
	Rules RulesConfig  `yaml:"rules"`
}

type ZoneConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type RulesConfig struct {
	Structural []StructuralRule `yaml:"structural"`
	Metadata   []MetadataRule   `yaml:"metadata"`
	Semantic   SemanticConfig   `yaml:"semantic"`
}

type StructuralRule struct {
	Zone        string   `yaml:"zone"`
	PathPattern string   `yaml:"path_pattern"`
	ExcludePath string   `yaml:"exclude_path"`
	AgeDays     int      `yaml:"age_days"`
	Extensions  []string `yaml:"extensions"`
	Suggest     string   `yaml:"suggest"`
	Confidence  float64  `yaml:"confidence"`
	Description string   `yaml:"description"`
}

type MetadataRule struct {
	Field      string  `yaml:"field"`
	Value      string  `yaml:"value"`
	Suggest    string  `yaml:"suggest"`
	Confidence float64 `yaml:"confidence"`
}

type SemanticConfig struct {
	Enabled   bool    `yaml:"enabled"`
	Model     string  `yaml:"model"`
	OllamaURL string  `yaml:"ollama_url"`
	Threshold float64 `yaml:"threshold"`
	MaxTokens int     `yaml:"max_tokens"`
}
