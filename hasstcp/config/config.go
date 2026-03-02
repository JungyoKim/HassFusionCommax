package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SSH     SSHConfig     `yaml:"ssh"`
	MQTT    MQTTConfig    `yaml:"mqtt"`
	Network NetworkConfig `yaml:"network"`
}

type SSHConfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Key      string `yaml:"key"`
	Command  string `yaml:"command"`
}

type MQTTConfig struct {
	Broker   string `yaml:"broker"`
	ClientID string `yaml:"client_id"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Topic    string `yaml:"topic"`
}

type NetworkConfig struct {
	LocalCIDRs []string `yaml:"local_cidrs"`
}

// Load reads config from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Set defaults
	if cfg.SSH.Host == "" {
		cfg.SSH.Host = "192.168.0.60:22"
	}
	if cfg.SSH.User == "" {
		cfg.SSH.User = "root"
	}
	if cfg.MQTT.ClientID == "" {
		cfg.MQTT.ClientID = "hasstcp"
	}
	if cfg.MQTT.Topic == "" {
		cfg.MQTT.Topic = "hasstcp/parking"
	}

	return &cfg, nil
}
