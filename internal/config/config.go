// Package config loads and validates the daemon's YAML configuration.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level on-disk config shape.
type Config struct {
	// RegistryPrefix is prepended to each entry in Repos. Example: "docker.io/myorg".
	RegistryPrefix string `yaml:"registry_prefix"`

	// Repos is the list of "name:tag" entries to watch under RegistryPrefix.
	// A bare "name" is treated as "name:latest".
	Repos []string `yaml:"repos"`

	// PollInterval is how often to poll the registry. Default 5m.
	PollInterval time.Duration `yaml:"poll_interval"`

	// ArtifactsDir is the directory where pulled artifacts are stored.
	// Default /var/lib/orascout/artifacts.
	ArtifactsDir string `yaml:"artifacts_dir"`

	// StateFile is the JSON state path. Default /var/lib/orascout/state.json.
	StateFile string `yaml:"state_file"`

	// LockFile is the PID lock path. Default /var/run/orascout.lock.
	LockFile string `yaml:"lock_file"`

	// Auth is the optional registry credential set. May be left empty for
	// anonymous pulls of public artifacts.
	Auth AuthConfig `yaml:"auth"`

	// LogsPush, if non-empty, is a "name:tag" relative to RegistryPrefix where
	// the daemon will push its own log file after any cycle that performed work.
	LogsPush string `yaml:"logs_push"`

	// LogFile is the file the daemon writes structured logs to.
	// Empty = stderr only.
	LogFile string `yaml:"log_file"`

	// Insecure allows http:// registries (for local testing). Default false.
	Insecure bool `yaml:"insecure"`
}

// AuthConfig is the bearer/basic-auth pair for the registry.
type AuthConfig struct {
	// Username for basic auth. Supports $ENV_VAR expansion.
	Username string `yaml:"username"`
	// Password for basic auth. Supports $ENV_VAR expansion.
	Password string `yaml:"password"`
	// Token for bearer auth. If set, Username/Password are ignored.
	// Supports $ENV_VAR expansion.
	Token string `yaml:"token"`
}

// RepoRef is a single parsed repo:tag entry.
type RepoRef struct {
	// FullRef is the absolute reference, e.g. "docker.io/myorg/name:latest".
	FullRef string
	// Name is the bare repo name, e.g. "name".
	Name string
	// Tag is the tag, e.g. "latest".
	Tag string
}

// Load reads, parses, and validates a YAML config file. Defaults are applied
// for any optional fields the user left unset.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	c.applyDefaults()
	c.expandEnv()

	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Minute
	}
	if c.ArtifactsDir == "" {
		c.ArtifactsDir = "/var/lib/orascout/artifacts"
	}
	if c.StateFile == "" {
		c.StateFile = "/var/lib/orascout/state.json"
	}
	if c.LockFile == "" {
		c.LockFile = "/var/run/orascout.lock"
	}
}

func (c *Config) expandEnv() {
	c.Auth.Username = expand(c.Auth.Username)
	c.Auth.Password = expand(c.Auth.Password)
	c.Auth.Token = expand(c.Auth.Token)
}

// expand replaces a leading "$NAME" or "${NAME}" with the env var's value.
// A literal value (no leading $) passes through unchanged.
func expand(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "$") {
		return v
	}
	name := strings.TrimPrefix(v, "$")
	name = strings.TrimPrefix(name, "{")
	name = strings.TrimSuffix(name, "}")
	return os.Getenv(name)
}

func (c *Config) validate() error {
	if c.RegistryPrefix == "" {
		return fmt.Errorf("registry_prefix is required")
	}
	if strings.Contains(c.RegistryPrefix, "://") {
		return fmt.Errorf("registry_prefix must not include scheme (got %q)", c.RegistryPrefix)
	}
	if len(c.Repos) == 0 {
		return fmt.Errorf("at least one entry in repos is required")
	}
	if c.PollInterval < time.Second {
		return fmt.Errorf("poll_interval too small (got %s, minimum 1s)", c.PollInterval)
	}
	return nil
}

// ParsedRepos returns the validated, expanded RepoRef for each entry in Repos.
func (c *Config) ParsedRepos() []RepoRef {
	out := make([]RepoRef, 0, len(c.Repos))
	for _, r := range c.Repos {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		name, tag, ok := strings.Cut(r, ":")
		if !ok {
			tag = "latest"
			name = r
		}
		out = append(out, RepoRef{
			Name:    name,
			Tag:     tag,
			FullRef: strings.TrimSuffix(c.RegistryPrefix, "/") + "/" + name + ":" + tag,
		})
	}
	return out
}
