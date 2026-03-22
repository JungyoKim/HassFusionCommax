package rs485

import (
	"log"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/config"
	"hassfusion/ws"
)

type DoorbellController struct {
	portSpec   string // 원본 포트 설정값 (e.g., "usb:1-2.1.3")
	port       *serial.Port
	wsServer   *ws.Server
	readBuffer []byte
	packetBuf  []byte
	serialMu   sync.Mutex
}

var (
	bellRingPacket1   = []byte{0x02, 0x10, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x10, 0x00, 0x00, 0x00, 0x40, 0x03}
	bellRingPacket2   = []byte{0x02, 0x10, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x10, 0x00, 0x00, 0x00, 0x5A, 0x03}
	doorOpenCmdPacket = []byte{0x02, 0x11, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x05, 0x40, 0x00, 0x01, 0x77, 0x03}
)

func NewDoorbellController(portSpec string, wsServer *ws.Server) *DoorbellController {
	if portSpec == "" {
		return nil
	}

	dc := &DoorbellController{
		portSpec:   portSpec,
		wsServer:   wsServer,
		readBuffer: make([]byte, 256),
		packetBuf:  make([]byte, 0, 512),
	}

	if err := dc.connectSerial(); err != nil {
		return nil
	}

	wsServer.RegisterHandler("doorbell_button", dc.wsCommandRouter)

	go dc.monitorLoop()

	return dc
}

func (dc *DoorbellController) connectSerial() error {
	for {
		portName := config.ResolveSerialPort(dc.portSpec)
		if portName == "" {
			log.Printf("[DOORBELL] USB 장치를 찾을 수 없습니다: %s, 3초 후 재시도...", dc.portSpec)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[DOORBELL] 시리얼 포트 %s 연결 시도 중...", portName)
		port, err := serial.OpenPort(&serial.Config{
			Name:        portName,
			Baud:        9600,
			ReadTimeout: 1 * time.Second,
		})
		if err != nil {
			log.Printf("[DOORBELL] 시리얼 포트 연결 실패: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		dc.port = port
		break
	}

	log.Println("[DOORBELL] 시리얼 포트 연결 성공!")
	return nil
}

func (dc *DoorbellController) reconnectSerial() {
	dc.serialMu.Lock()
	if dc.port != nil {
		dc.port.Close()
		dc.port = nil
	}
	dc.serialMu.Unlock()

	log.Printf("[DOORBELL] 시리얼 포트 재연결 시도 중... (%s)", dc.portSpec)
	dc.connectSerial()
}

func matchPacket(data, pattern []byte) (bool, int) {
	if len(data) < len(pattern) {
		return false, -1
	}
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return true, i + len(pattern)
		}
	}
	return false, -1
}

func (dc *DoorbellController) monitorLoop() {
	var lastReadTime time.Time
	consecutiveErrors := 0

	for {
		dc.serialMu.Lock()
		n, err := dc.port.Read(dc.readBuffer)
		dc.serialMu.Unlock()

		if err != nil && err.Error() != "EOF" {
			log.Printf("[DOORBELL] 시리얼 읽기 에러: %v", err)
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				consecutiveErrors = 0
				dc.reconnectSerial()
			}
			time.Sleep(1 * time.Second)
			continue
		}

		consecutiveErrors = 0

		if n > 0 {
			now := time.Now()
			if now.Sub(lastReadTime) > 200*time.Millisecond {
				dc.packetBuf = dc.packetBuf[:0]
			}
			lastReadTime = now

			dc.packetBuf = append(dc.packetBuf, dc.readBuffer[:n]...)

			if len(dc.packetBuf) >= 15 {
				matched, endIdx := matchPacket(dc.packetBuf, bellRingPacket1)
				if !matched {
					matched, endIdx = matchPacket(dc.packetBuf, bellRingPacket2)
				}

				if matched {
					log.Println("[DOORBELL] Ring detected!")
					dc.wsServer.Broadcast(ws.WSMsg{
						Type:     "event",
						Domain:   "binary_sensor",
						DeviceID: "doorbell",
						State:    "on",
					})

					go func() {
						time.Sleep(5 * time.Second)
						dc.wsServer.Broadcast(ws.WSMsg{
							Type:     "event",
							Domain:   "binary_sensor",
							DeviceID: "doorbell",
							State:    "off",
						})
					}()

					dc.packetBuf = dc.packetBuf[endIdx:]
				}
			}

			if len(dc.packetBuf) > 256 {
				dc.packetBuf = dc.packetBuf[128:]
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (dc *DoorbellController) wsCommandRouter(msg ws.WSMsg) {
	if msg.Action == "turn_on" || msg.Action == "press" {
		dc.serialMu.Lock()
		defer dc.serialMu.Unlock()

		_, err := dc.port.Write(doorOpenCmdPacket)
		if err != nil {
			log.Printf("[DOORBELL] 문열림 명령 전송 실패: %v", err)
		} else {
			log.Printf("[DOORBELL] Doorbell command sent: OPEN")
		}

		time.Sleep(100 * time.Millisecond)
		drBuf := make([]byte, 128)
		dc.port.Read(drBuf)
	}
}

func (dc *DoorbellController) Close() {
	if dc.port != nil {
		dc.port.Close()
	}
}
