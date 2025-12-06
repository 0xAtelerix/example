package main

import (
	"fmt"
	"os"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// Config holds the appchain configuration.
type Config struct {
	// ChainID is the unique identifier for this appchain.
	ChainID uint64 `yaml:"chain_id"`

	// DataDir is the root data directory (shared with pelacli).
	DataDir string `yaml:"data_dir"`

	// EmitterPort is the gRPC port for the emitter API.
	EmitterPort string `yaml:"emitter_port"`

	// RPCPort is the HTTP port for JSON-RPC server.
	RPCPort string `yaml:"rpc_port"`

	// LogLevel is the zerolog log level (0=debug, 1=info, 2=warn, 3=error).
	LogLevel int `yaml:"log_level"`
}

// DefaultConfig returns a config with all defaults applied.
func DefaultConfig() *Config {
	cfg := &Config{}
	cfg.applyDefaults()

	return cfg
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()

	return &cfg, nil
}

// applyDefaults sets default values for unset fields.
func (c *Config) applyDefaults() {
	if c.ChainID == 0 {
		c.ChainID = gosdk.DefaultAppchainID
	}

	if c.DataDir == "" {
		c.DataDir = gosdk.DefaultDataDir
	}

	if c.EmitterPort == "" {
		c.EmitterPort = gosdk.DefaultEmitterPort
	}

	if c.RPCPort == "" {
		c.RPCPort = gosdk.DefaultRPCPort
	}

	if c.LogLevel == 0 {
		c.LogLevel = int(zerolog.InfoLevel)
	}
}
