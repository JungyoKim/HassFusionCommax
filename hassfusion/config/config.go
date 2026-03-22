package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration format
type Config struct {
	WebSocket struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"websocket"`

	RS485 struct {
		Lights   string `yaml:"lights"`   // e.g., usb:1-2.1.1 or /dev/ttyUSB0
		Boilers  string `yaml:"boilers"`  // e.g., usb:1-2.1.2
		Doorbell string `yaml:"doorbell"` // e.g., usb:1-2.1.3
		AllOff   string `yaml:"alloff"`   // e.g., usb:1-2.1.4
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

// ResolveSerialPort resolves a serial port path.
// If the path starts with "usb:", it looks up the USB physical port path
// (e.g., "usb:1-2.1.1") and returns the corresponding /dev/ttyUSB* device.
// Otherwise, returns the path as-is.
func ResolveSerialPort(portSpec string) string {
	if !strings.HasPrefix(portSpec, "usb:") {
		return portSpec
	}

	usbPath := strings.TrimPrefix(portSpec, "usb:")
	resolved := findTTYByUSBPath(usbPath)
	if resolved == "" {
		log.Printf("[CONFIG] USB 장치를 찾을 수 없습니다: %s", usbPath)
		return ""
	}
	log.Printf("[CONFIG] USB 경로 %s → %s", usbPath, resolved)
	return resolved
}

// findTTYByUSBPath scans sysfs to find which /dev/ttyUSB* device corresponds
// to the given USB physical port path (e.g., "1-2.1.1").
//
// sysfs structure:
//   /sys/bus/usb-serial/devices/ttyUSB0 -> ../../../1-2.1.1:1.0/ttyUSB0
//
// The symlink target contains the USB physical path, so we can match against it.
func findTTYByUSBPath(usbPath string) string {
	const sysfsDir = "/sys/bus/usb-serial/devices"

	entries, err := os.ReadDir(sysfsDir)
	if err != nil {
		log.Printf("[CONFIG] sysfs 디렉토리 읽기 실패: %v", err)
		return ""
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "ttyUSB") {
			continue
		}

		linkPath := filepath.Join(sysfsDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}

		// target looks like: ../../../1-2.1.1:1.0/ttyUSB0
		// Check if it contains our USB path
		if strings.Contains(target, usbPath+":") {
			return "/dev/" + entry.Name()
		}
	}

	return ""
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
