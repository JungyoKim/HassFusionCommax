package rs485

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

// PortMap holds the discovered port spec (usb:1-2.1.x format) for each RS485 role.
// Using the physical USB path instead of /dev/ttyUSBx ensures stable reconnection
// across reboots even if ttyUSB numbering changes.
type PortMap struct {
	Lights   string
	Boilers  string
	Doorbell string
	AllOff   string
}

var (
	probeAllOff  = []byte{0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21}
	probeLights  = []byte{0x30, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x31}
	probeBoilers = []byte{0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
)

// listTTYUSBDevices returns all /dev/ttyUSB* paths found via sysfs.
func listTTYUSBDevices() []string {
	const sysfsDir = "/sys/bus/usb-serial/devices"
	entries, err := os.ReadDir(sysfsDir)
	if err != nil {
		// fallback: glob
		matches, _ := filepath.Glob("/dev/ttyUSB*")
		return matches
	}
	var devs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ttyUSB") {
			devs = append(devs, "/dev/"+e.Name())
		}
	}
	return devs
}

// getUSBPhysicalPath converts /dev/ttyUSBx to "usb:1-2.1.x" format via sysfs.
// Falls back to the original path if resolution fails.
func getUSBPhysicalPath(dev string) string {
	name := filepath.Base(dev)
	linkPath := filepath.Join("/sys/bus/usb-serial/devices", name)
	target, err := os.Readlink(linkPath)
	if err != nil {
		return dev
	}
	// target: "../../../1-2.1.4:1.0/ttyUSB3" — extract "1-2.1.4"
	for _, seg := range strings.Split(target, "/") {
		if idx := strings.Index(seg, ":"); idx > 0 {
			candidate := seg[:idx]
			if strings.Contains(candidate, "-") && strings.Contains(candidate, ".") {
				return "usb:" + candidate
			}
		}
	}
	return dev
}

// validResponse checks 8-byte RS485 response: prefix match + checksum.
func validResponse(data []byte, prefixes ...byte) bool {
	if len(data) < 8 {
		return false
	}
	matched := false
	for _, p := range prefixes {
		if data[0] == p {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	var sum byte
	for i := 0; i < 7; i++ {
		sum += data[i]
	}
	return sum == data[7]
}

// probeRole opens a port, drains stale data, sends each probe packet, and
// returns the identified role ("lights", "boilers", "alloff") or "" (→ doorbell).
func probeRole(dev string) string {
	cfg := &serial.Config{
		Name:        dev,
		Baud:        9600,
		ReadTimeout: 100 * time.Millisecond,
	}
	port, err := serial.OpenPort(cfg)
	if err != nil {
		log.Printf("[DISCOVER] %s 열기 실패: %v", dev, err)
		return ""
	}
	defer port.Close()

	drain := make([]byte, 256)

	type probe struct {
		pkt      []byte
		role     string
		prefixes []byte
	}
	probes := []probe{
		{probeAllOff, "alloff", []byte{0xA0}},
		{probeLights, "lights", []byte{0xB0}},
		{probeBoilers, "boilers", []byte{0x82, 0x84}},
	}

	// readWindow accumulates bytes up to 'want' within 'window' deadline,
	// scanning for a valid 8-byte response with any of the given prefixes.
	readWindow := func(window time.Duration, prefixes []byte) []byte {
		buf := make([]byte, 0, 64)
		tmp := make([]byte, 64)
		deadline := time.Now().Add(window)
		for time.Now().Before(deadline) {
			n, _ := port.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				// scan buffer for any 8-byte window starting with a known prefix + valid checksum
				for i := 0; i+8 <= len(buf); i++ {
					if validResponse(buf[i:i+8], prefixes...) {
						return buf[i : i+8]
					}
				}
			}
		}
		return buf
	}

	for _, p := range probes {
		// drain stale bytes (short window)
		for t0 := time.Now(); time.Since(t0) < 100*time.Millisecond; {
			n, _ := port.Read(drain)
			if n == 0 {
				break
			}
		}

		port.Write(p.pkt)
		resp := readWindow(700*time.Millisecond, p.prefixes)
		if len(resp) == 8 && validResponse(resp, p.prefixes...) {
			log.Printf("[DISCOVER] %s probe=%s matched: %x", dev, p.role, resp)
			return p.role
		}
		if len(resp) > 0 {
			log.Printf("[DISCOVER] %s probe=%s got %dB (no match): %x", dev, p.role, len(resp), resp)
		}
		time.Sleep(80 * time.Millisecond)
	}
	return ""
}

// DiscoverPorts probes all connected USB-serial adapters in parallel and returns
// a PortMap with stable "usb:1-2.1.x" path specs for each RS485 role.
// Unidentified ports (no active query/response) are assigned as doorbell.
func DiscoverPorts() (*PortMap, error) {
	devs := listTTYUSBDevices()
	if len(devs) == 0 {
		return nil, fmt.Errorf("RS485 USB 어댑터를 찾을 수 없습니다")
	}
	log.Printf("[DISCOVER] RS485 자동 탐색 시작 — 발견된 포트: %v", devs)

	type result struct {
		dev  string
		spec string // usb:1-2.1.x
		role string
	}

	ch := make(chan result, len(devs))
	var wg sync.WaitGroup
	for _, dev := range devs {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			role := probeRole(d)
			spec := getUSBPhysicalPath(d)
			ch <- result{d, spec, role}
		}(dev)
	}
	wg.Wait()
	close(ch)

	pm := &PortMap{}
	var unidentified []result
	for r := range ch {
		switch r.role {
		case "lights":
			pm.Lights = r.spec
		case "boilers":
			pm.Boilers = r.spec
		case "alloff":
			pm.AllOff = r.spec
		default:
			unidentified = append(unidentified, r)
		}
		log.Printf("[DISCOVER] %s (%s) → %s", r.dev, r.spec, func() string {
			if r.role == "" {
				return "doorbell (미응답)"
			}
			return r.role
		}())
	}

	switch len(unidentified) {
	case 1:
		pm.Doorbell = unidentified[0].spec
	case 0:
		log.Printf("[DISCOVER] 경고: doorbell 포트를 식별하지 못했습니다")
	default:
		log.Printf("[DISCOVER] 경고: 미식별 포트 %d개 — 첫 번째를 doorbell로 지정", len(unidentified))
		pm.Doorbell = unidentified[0].spec
	}

	log.Printf("[DISCOVER] 탐색 완료: lights=%s boilers=%s doorbell=%s alloff=%s",
		pm.Lights, pm.Boilers, pm.Doorbell, pm.AllOff)

	return pm, nil
}
