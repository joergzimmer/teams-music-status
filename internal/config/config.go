package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Azure    AzureConfig    `yaml:"azure"`
	Status   StatusConfig   `yaml:"status"`
	Behavior BehaviorConfig `yaml:"behavior"`
}

// AzureConfig contains the Entra ID app registration data.
type AzureConfig struct {
	TenantID string `yaml:"tenant_id"`
	ClientID string `yaml:"client_id"`
}

// StatusConfig controls how the Teams status message looks.
type StatusConfig struct {
	Template     string `yaml:"template"`
	ExpiryMin    int    `yaml:"expiry_minutes"`
	ClearOnPause bool   `yaml:"clear_on_pause"`
	TimeZone     string `yaml:"timezone"`
}

// BehaviorConfig controls the service runtime behavior.
type BehaviorConfig struct {
	DebounceSec int    `yaml:"debounce_seconds"`
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`
}

// DefaultConfigDir returns ~/.config/teams-music
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/teams-music"
	}
	return filepath.Join(home, ".config", "teams-music")
}

// DefaultConfigPath returns the default path to the config file.
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// Load reads the config from a YAML file and applies defaults.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config lesen (%s): %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config parsen (%s): %w", path, err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config ungültig: %w", err)
	}

	return cfg, nil
}

// applyDefaults sets missing values to sensible defaults.
func (c *Config) applyDefaults() {
	if c.Status.Template == "" {
		c.Status.Template = "🎵 {{.Name}} – {{.Artist}}"
	}
	if c.Status.ExpiryMin <= 0 {
		c.Status.ExpiryMin = 10
	}
	if c.Status.TimeZone == "" {
		c.Status.TimeZone = "W. Europe Standard Time"
	}
	if c.Behavior.DebounceSec <= 0 {
		c.Behavior.DebounceSec = 3
	}
	if c.Behavior.LogLevel == "" {
		c.Behavior.LogLevel = "info"
	}
}

// validate checks whether all required fields are set.
func (c *Config) validate() error {
	if c.Azure.TenantID == "" {
		return fmt.Errorf("azure.tenant_id ist leer")
	}
	if c.Azure.ClientID == "" {
		return fmt.Errorf("azure.client_id ist leer")
	}
	return nil
}

// EnsureConfigDir creates the config directory if needed.
func EnsureConfigDir() (string, error) {
	dir := DefaultConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("config-verzeichnis erstellen (%s): %w", dir, err)
	}
	return dir, nil
}

// WriteExample writes an example config if none exists yet.
// Returns true if the file was created.
func WriteExample(path string) (bool, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	// Already exists -> nothing to do
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, fmt.Errorf("verzeichnis erstellen: %w", err)
	}

	if err := os.WriteFile(path, []byte(exampleConfig), 0600); err != nil {
		return false, fmt.Errorf("beispiel-config schreiben: %w", err)
	}

	return true, nil
}

const exampleConfig = `# teams-music configuration
# Path: ~/.config/teams-music/config.yaml

# Microsoft Entra ID (Azure AD) App-Registration
azure:
	tenant_id: "YOUR-TENANT-ID-HERE"
	client_id: "YOUR-CLIENT-ID-HERE"

# Status message configuration
status:
	# Go text/template syntax - available fields: .Name, .Artist, .Album
  template: "🎵 {{.Name}} – {{.Artist}}"
	# Status message expiry time in minutes
  expiry_minutes: 10
	# Clear status when music is paused/stopped
  clear_on_pause: true
	# Time zone for expiry calculation
  timezone: "W. Europe Standard Time"

# Runtime behavior
behavior:
	# Wait time in seconds before updating status (prevents spam while quickly skipping tracks)
  debounce_seconds: 3
  # Log-Level: debug, info, warn, error
  log_level: "info"
	# Log file (empty = stdout only)
  # log_file: "~/Library/Logs/teams-music.log"
`
