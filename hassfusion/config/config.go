package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration format
type Config struct {
	WebSocket struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"websocket"`

	RS485 struct {
		Lights   string `yaml:"lights"`   // e.g., /dev/ttyUSB0 or COM1
		Boilers  string `yaml:"boilers"`  // e.g., /dev/ttyUSB1
		Doorbell string `yaml:"doorbell"` // e.g., /dev/ttyUSB2
		AllOff   string `yaml:"alloff"`   // e.g., /dev/ttyUSB3
	} `yaml:"rs485"`

	TCP struct {
		UseSSH bool `yaml:"use_ssh"`

		SSH struct {
			Host     string `yaml:"host"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Key      string `yaml:"key"`
			Command  string `yaml:"command"`
		} `yaml:"ssh"`

		LocalCIDRs []string `yaml:"local_cidrs"`
	} `yaml:"tcp"`

	Doors struct {
		FloorB4IP string `yaml:"floor_b4_ip"`
		FloorB3IP string `yaml:"floor_b3_ip"`
		Floor1FIP string `yaml:"floor_1f_ip"`
	} `yaml:"doors"`

	Wallpad struct {
		IP       string `yaml:"ip"`
		Mac      string `yaml:"mac"`
		DeviceID string `yaml:"device_id"`
		Dong     string `yaml:"dong"`
		Ho       string `yaml:"ho"`
	} `yaml:"wallpad"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// Make sure defaults exist
	if cfg.WebSocket.Host == "" {
		cfg.WebSocket.Host = "0.0.0.0"
	}
	if cfg.WebSocket.Port == 0 {
		cfg.WebSocket.Port = 8080 // Default WS port
	}

	return &cfg, nil
}
