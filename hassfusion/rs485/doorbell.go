package rs485

import (
	"log"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/ws"
)

type DoorbellController struct {
	port       *serial.Port
	wsServer   *ws.Server
	readBuffer []byte
	packetBuf  []byte
	serialMu   sync.Mutex // [수정1] 시리얼 포트 동시 접근을 막기 위한 뮤텍스 추가
}

var (
	bellRingPacket1   = []byte{0x02, 0x10, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x10, 0x00, 0x00, 0x00, 0x40, 0x03}
	bellRingPacket2   = []byte{0x02, 0x10, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x10, 0x00, 0x00, 0x00, 0x5A, 0x03}
	doorOpenCmdPacket = []byte{0x02, 0x11, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x05, 0x40, 0x00, 0x01, 0x77, 0x03}
)

func NewDoorbellController(devicePath string, wsServer *ws.Server) *DoorbellController {
	if devicePath == "" {
		return nil
	}

	config := &serial.Config{
		Name:        devicePath,
		Baud:        9600,
		ReadTimeout: 1 * time.Second,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("Failed to open doorbell serial %s: %v", devicePath, err)
		return nil
	}

	dc := &DoorbellController{
		port:       port,
		wsServer:   wsServer,
		readBuffer: make([]byte, 256),
		packetBuf:  make([]byte, 0, 512), // [수정2] 버퍼 여유를 위해 크기 증가
	}

	wsServer.RegisterHandler("doorbell_button", dc.wsCommandRouter)

	go dc.monitorLoop()

	return dc
}

// [수정3] 매칭된 패턴의 길이와 위치를 반환하도록 변경
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
			return true, i + len(pattern) // 일치하는 패턴 끝 인덱스 반환
		}
	}
	return false, -1
}

func (dc *DoorbellController) monitorLoop() {
	var lastReadTime time.Time
	for {
		dc.serialMu.Lock()
		n, err := dc.port.Read(dc.readBuffer)
		dc.serialMu.Unlock()

		if err != nil && err.Error() != "EOF" {
			log.Printf("[DOORBELL] 시리얼 읽기 에러: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if n > 0 {
			now := time.Now()
			// if more than 200ms since last read, clear packet buffer
			// [수정4] 시간 딜레이를 200ms로 늘려서 파편화된 패킷이 합쳐질 시간을 줌
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

					// Auto off after 5 seconds to reset sensor
					go func() {
						time.Sleep(5 * time.Second)
						dc.wsServer.Broadcast(ws.WSMsg{
							Type:     "event",
							Domain:   "binary_sensor",
							DeviceID: "doorbell",
							State:    "off",
						})
					}()

					// [수정5] 무작정 버퍼를 날리지 않고 매칭된 패킷 뒷부분만 남김
					dc.packetBuf = dc.packetBuf[endIdx:]
				}
			}

			// [수정6] 오버플로우 방지 시에도 절반만 날리도록 변경
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

		// [수정7] 문 열림 명령 전송 후 찌꺼기 응답을 먹어치움
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
