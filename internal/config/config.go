package config

import (
	"errors"
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

const defaultAddress = "127.0.0.1:9001"

type Config struct {
	Server   ServerConfig `toml:"server"`
	Programs []Program    `toml:"program"`
}

type ServerConfig struct {
	Address string `toml:"address"`
}

type Program struct {
	Name      string   `toml:"name"`
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	Directory string   `toml:"directory"`
	Autostart bool     `toml:"autostart"`
}

func Load(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("parse TOML config: %w", err)
	}

	if cfg.Server.Address == "" {
		cfg.Server.Address = defaultAddress
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	seen := make(map[string]struct{}, len(c.Programs))
	for _, p := range c.Programs {
		if p.Name == "" {
			return errors.New("program name is required")
		}
		if p.Command == "" {
			return fmt.Errorf("program %q command is required", p.Name)
		}
		if _, exists := seen[p.Name]; exists {
			return fmt.Errorf("program %q is duplicated", p.Name)
		}
		seen[p.Name] = struct{}{}
	}
	return nil
}
