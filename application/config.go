package application

import (
	"fmt"
	"os"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

// RetryConfig holds configuration for retry behavior.
type RetryConfig struct {
	// IntervalSeconds is how long to wait before retrying a stuck event
	IntervalSeconds int `yaml:"interval_seconds"`
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int `yaml:"max_attempts"`
}

// BridgeConfig holds configuration for bridge contracts and token mappings.
type BridgeConfig struct {
	// Contracts maps chainID -> bridge contract address
	Contracts map[uint64]string `yaml:"contracts"`
	// TokenMappings maps sourceChainID -> sourceToken -> destToken
	TokenMappings map[uint64]map[string]string `yaml:"token_mappings"`
	// Retry configuration for stuck events
	Retry RetryConfig `yaml:"retry"`
}

// AppConfig embeds the SDK config and adds bridge-specific configuration.
type AppConfig struct {
	gosdk.InitConfig `yaml:",inline"`
	Bridge           BridgeConfig `yaml:"bridge"`
	MetricsPort      int          `yaml:"metrics_port"`
}

// LoadConfig loads both SDK and bridge configuration from a YAML file.
// If path is empty, returns default configuration.
func LoadConfig(path string) (*AppConfig, error) {
	if path == "" {
		return &AppConfig{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

// GetBridgeContracts returns the bridge contracts map with parsed addresses.
func (c *BridgeConfig) GetBridgeContracts() map[uint64]common.Address {
	contracts := make(map[uint64]common.Address)

	for chainID, addr := range c.Contracts {
		contracts[chainID] = common.HexToAddress(addr)
	}

	return contracts
}

// GetTokenMappings returns the token mappings with parsed addresses.
func (c *BridgeConfig) GetTokenMappings() map[uint64]map[common.Address]common.Address {
	mappings := make(map[uint64]map[common.Address]common.Address)

	for chainID, tokens := range c.TokenMappings {
		mappings[chainID] = make(map[common.Address]common.Address)

		for src, dst := range tokens {
			mappings[chainID][common.HexToAddress(src)] = common.HexToAddress(dst)
		}
	}

	return mappings
}

// GetRetryConfig returns retry configuration with defaults if not set.
func (c *BridgeConfig) GetRetryConfig() RetryConfig {
	cfg := c.Retry

	// Apply defaults
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 120 // Default: 2 minutes
	}

	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 10 // Default: 10 attempts
	}

	return cfg
}
