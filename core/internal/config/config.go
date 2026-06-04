// Package config is the typed, layered settings store for the sidecar.
// Layering (lowest to highest precedence):
//  1. compiled-in defaults
//  2. config file (YAML) under $XDG_CONFIG_HOME/foundry-copilot/config.yaml
//  3. environment variables prefixed FOUNDRY_COPILOT_
//  4. JSON-RPC `config/set` calls from the extension at runtime
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// Config is the typed view of all sidecar settings.
type Config struct {
	Endpoint             string `mapstructure:"endpoint"`
	ChatDeployment       string `mapstructure:"chat_deployment"`
	CompletionDeployment string `mapstructure:"completion_deployment"`
	EmbeddingDeployment  string `mapstructure:"embedding_deployment"`
	LogLevel             string `mapstructure:"log_level"`
	AgentMaxSteps        int    `mapstructure:"agent_max_steps"`
	AgentAllowShell      bool   `mapstructure:"agent_allow_shell"`
	AgentAllowWrite      bool   `mapstructure:"agent_allow_write"`
	WorkspaceRoot        string `mapstructure:"workspace_root"`
	RAGIndexDir          string `mapstructure:"rag_index_dir"`
	SessionDir           string `mapstructure:"session_dir"`
}

var (
	mu   sync.RWMutex
	curr Config
)

// Defaults returns the compiled-in defaults.
func Defaults() Config {
	home, _ := os.UserHomeDir()
	cfgRoot := filepath.Join(home, ".foundry-copilot")
	return Config{
		LogLevel:      "info",
		AgentMaxSteps: 24,
		RAGIndexDir:   filepath.Join(cfgRoot, "index"),
		SessionDir:    filepath.Join(cfgRoot, "sessions"),
	}
}

// Load reads defaults → config file → env. Returns the resolved Config and
// also stores it in the package-level singleton accessible via Get().
func Load() (Config, error) {
	v := viper.New()
	d := Defaults()
	v.SetDefault("log_level", d.LogLevel)
	v.SetDefault("agent_max_steps", d.AgentMaxSteps)
	v.SetDefault("rag_index_dir", d.RAGIndexDir)
	v.SetDefault("session_dir", d.SessionDir)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		v.AddConfigPath(filepath.Join(xdg, "foundry-copilot"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "foundry-copilot"))
		v.AddConfigPath(filepath.Join(home, ".foundry-copilot"))
	}

	v.SetEnvPrefix("FOUNDRY_COPILOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !asConfigFileNotFound(err, &notFound) {
			return Config{}, fmt.Errorf("config: read: %w", err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return Config{}, fmt.Errorf("config: unmarshal: %w", err)
	}

	mu.Lock()
	curr = c
	mu.Unlock()
	return c, nil
}

// asConfigFileNotFound is a small helper to keep error handling readable.
func asConfigFileNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	if cf, ok := err.(viper.ConfigFileNotFoundError); ok {
		*target = cf
		return true
	}
	return false
}

// Get returns a copy of the currently loaded config.
func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	return curr
}

// Update merges values from in into the live config and returns the new
// snapshot. It is intentionally minimal — only the fields the extension is
// expected to set at runtime are honoured.
func Update(in Config) Config {
	mu.Lock()
	defer mu.Unlock()
	if in.Endpoint != "" {
		curr.Endpoint = in.Endpoint
	}
	if in.ChatDeployment != "" {
		curr.ChatDeployment = in.ChatDeployment
	}
	if in.CompletionDeployment != "" {
		curr.CompletionDeployment = in.CompletionDeployment
	}
	if in.EmbeddingDeployment != "" {
		curr.EmbeddingDeployment = in.EmbeddingDeployment
	}
	if in.LogLevel != "" {
		curr.LogLevel = in.LogLevel
	}
	if in.AgentMaxSteps > 0 {
		curr.AgentMaxSteps = in.AgentMaxSteps
	}
	if in.WorkspaceRoot != "" {
		curr.WorkspaceRoot = in.WorkspaceRoot
	}
	// Booleans always overwrite — the extension sends the user's current
	// trust-level toggles every time it calls config/set.
	curr.AgentAllowShell = in.AgentAllowShell
	curr.AgentAllowWrite = in.AgentAllowWrite
	return curr
}
