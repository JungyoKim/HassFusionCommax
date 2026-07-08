package rs485

import (
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/config"
	"hassfusion/ws"
)

type LightController struct {
	portSpec          string // 원본 포트 설정값 (e.g., "usb:1-2.1.1")
	port              *serial.Port
	wsServer          *ws.Server
	lightStatus       []int
	prevStatus        []int
	statusMu          sync.Mutex
	serialMu          sync.Mutex
	pauseStatusQuery  chan bool
	resumeStatusQuery chan bool
	isQueryPaused     bool
	readBuffer        []byte

	// Packet Definitions
	statusQueryPackets [5]string
	onPackets          [5]string
	offPackets         [5]string
}

func (lc *LightController) publishStateIfChanged(idx int, newStatus int) {
	lc.statusMu.Lock()
	defer lc.statusMu.Unlock()

	if lc.prevStatus[idx] != newStatus {
		statusStr := "off"
		if newStatus == 1 {
			statusStr = "on"
		}

		log.Printf("[LIGHT] 조명 %d 상태 변경: -> %s\n", idx+1, statusStr)

		lc.wsServer.Broadcast(ws.WSMsg{
			Type:     "event",
			Domain:   "light",
			DeviceID: fmt.Sprintf("light_%d", idx+1),
			State:    statusStr,
		})

		lc.prevStatus[idx] = newStatus
	}
}

func validLightChecksum(pkt []byte) bool {
	if len(pkt) != 8 {
		return false
	}
	var sum byte
	for i := 0; i < 7; i++ {
		sum += pkt[i]
	}
	return sum == pkt[7]
}

// processStatusResponse handles a validated 8-byte light ack:
// [0]=0xB0, [1]=state(0x01 on / 0x00 off), [2]=light number, [7]=checksum.
func (lc *LightController) processStatusResponse(packet []byte) {
	if len(packet) != 8 || packet[0] != 0xB0 {
		return
	}

	num := int(packet[2])
	if num < 1 || num > 5 {
		return
	}
	arrayIdx := num - 1

	var newStatus int
	switch packet[1] {
	case 0x01:
		newStatus = 1
	case 0x00:
		newStatus = 0
	default:
		return
	}

	lc.statusMu.Lock()
	lc.lightStatus[arrayIdx] = newStatus
	lc.statusMu.Unlock()

	// publishStateIfChanged dedups against prevStatus (init -1), so the first
	// observation always emits — even for a light that is off at startup.
	lc.publishStateIfChanged(arrayIdx, newStatus)
}

func NewLightController(portSpec string, wsServer *ws.Server) *LightController {
	if portSpec == "" {
		return nil
	}

	lc := &LightController{
		portSpec:          portSpec,
		wsServer:          wsServer,
		lightStatus:       make([]int, 5),
		prevStatus:        []int{-1, -1, -1, -1, -1},
		pauseStatusQuery:  make(chan bool, 1),
		resumeStatusQuery: make(chan bool, 1),
		readBuffer:        make([]byte, 128),

		statusQueryPackets: [5]string{
			"3001000000000031",
			"3002000000000032",
			"3003000000000033",
			"3004000000000034",
			"3005000000000035",
		},
		onPackets: [5]string{
			"3101010000000033",
			"3102010000000034",
			"3103010000000035",
			"3104010000000036",
			"3105010000000037",
		},
		offPackets: [5]string{
			"3101000000000032",
			"3102000000000033",
			"3103000000000034",
			"3104000000000035",
			"3105000000000036",
		},
	}

	// Connect in the background so a missing/slow USB adapter can never block
	// daemon startup (and thus the other controllers + WS handlers). The command
	// router and query loop are nil-port-safe until the port comes up.
	wsServer.RegisterHandler("light", lc.wsCommandRouter)
	go func() {
		lc.connectSerial()
		lc.statusQueryLoop()
	}()

	return lc
}

func (lc *LightController) connectSerial() error {
	for {
		portName := config.ResolveSerialPort(lc.portSpec)
		if portName == "" {
			log.Printf("[LIGHT] USB 장치를 찾을 수 없습니다: %s, 3초 후 재시도...", lc.portSpec)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[LIGHT] 시리얼 포트 %s 연결 시도 중...", portName)
		port, err := serial.OpenPort(&serial.Config{
			Name:        portName,
			Baud:        9600,
			Size:        8,
			Parity:      serial.ParityNone,
			StopBits:    serial.Stop1,
			ReadTimeout: 100 * time.Millisecond,
		})
		if err != nil {
			log.Printf("[LIGHT] 시리얼 포트 연결 실패: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		lc.serialMu.Lock()
		lc.port = port
		lc.serialMu.Unlock()
		break
	}

	log.Println("[LIGHT] 시리얼 포트 연결 성공!")
	return nil
}

func (lc *LightController) reconnectSerial() {
	lc.serialMu.Lock()
	if lc.port != nil {
		lc.port.Close()
		lc.port = nil
	}
	lc.serialMu.Unlock()

	log.Printf("[LIGHT] 시리얼 포트 재연결 시도 중... (%s)", lc.portSpec)
	lc.connectSerial()
}

func (lc *LightController) statusQueryLoop() {
	var packetBuf []byte
	var lastReadTime time.Time
	consecutiveErrors := 0

	for {
		// BLOCKING Pause: Wait until command acknowledge
		select {
		case <-lc.pauseStatusQuery:
			lc.statusMu.Lock()
			lc.isQueryPaused = true
			lc.statusMu.Unlock()

			<-lc.resumeStatusQuery

			lc.statusMu.Lock()
			lc.isQueryPaused = false
			lc.statusMu.Unlock()
		default:
		}

	packetLoop:
		for _, pkt := range lc.statusQueryPackets {
			select {
			case <-lc.pauseStatusQuery:
				lc.statusMu.Lock()
				lc.isQueryPaused = true
				lc.statusMu.Unlock()
				<-lc.resumeStatusQuery
				lc.statusMu.Lock()
				lc.isQueryPaused = false
				lc.statusMu.Unlock()
				break packetLoop
			default:
			}

			lc.serialMu.Lock()
			if lc.port == nil {
				lc.serialMu.Unlock()
				lc.reconnectSerial()
				continue
			}
			hexBytes, _ := hex.DecodeString(pkt)
			_, err := lc.port.Write(hexBytes)
			if err != nil {
				lc.serialMu.Unlock()
				log.Printf("[LIGHT] 시리얼 포트 쓰기 에러: %v\n", err)
				consecutiveErrors++
				if consecutiveErrors >= 5 {
					consecutiveErrors = 0
					lc.reconnectSerial()
				}
				time.Sleep(1 * time.Second)
				continue
			}

			n, err := lc.port.Read(lc.readBuffer)
			lc.serialMu.Unlock()

			if err != nil && err.Error() != "EOF" {
				log.Printf("[LIGHT] 시리얼 포트 읽기 에러: %v\n", err)
				consecutiveErrors++
				if consecutiveErrors >= 5 {
					consecutiveErrors = 0
					lc.reconnectSerial()
				}
				time.Sleep(1 * time.Second)
				continue
			}

			consecutiveErrors = 0

			if n > 0 {
				now := time.Now()
				if now.Sub(lastReadTime) > 200*time.Millisecond {
					packetBuf = packetBuf[:0]
				}
				lastReadTime = now

				packetBuf = append(packetBuf, lc.readBuffer[:n]...)

				// Byte-level scan: a hex-string search matches 0xB0 at misaligned
				// nibbles. Require a checksum-valid 8-byte packet before trusting it.
				parsedOffset := 0
				for j := 0; j+8 <= len(packetBuf); j++ {
					if packetBuf[j] == 0xB0 && validLightChecksum(packetBuf[j:j+8]) {
						lc.processStatusResponse(packetBuf[j : j+8])
						parsedOffset = j + 8
						j += 7
					}
				}
				if parsedOffset > 0 {
					copy(packetBuf, packetBuf[parsedOffset:])
					packetBuf = packetBuf[:len(packetBuf)-parsedOffset]
				} else if len(packetBuf) > 64 {
					packetBuf = packetBuf[len(packetBuf)-32:]
				}
			}
			time.Sleep(75 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (lc *LightController) wsCommandRouter(msg ws.WSMsg) {
	var num int
	if _, err := fmt.Sscanf(msg.DeviceID, "light_%d", &num); err != nil {
		return
	}

	if num < 1 || num > 5 {
		return
	}

	idx := num - 1
	var pkt string

	lc.statusMu.Lock()
	cur := lc.lightStatus[idx]
	lc.statusMu.Unlock()

	action := msg.Action
	if action == "toggle" {
		if cur == 1 {
			action = "turn_off"
		} else {
			action = "turn_on"
		}
	}

	if action == "turn_on" {
		pkt = lc.onPackets[idx]
		log.Printf("[LIGHT] 조명%d ON 명령 전송 (캐시상태: %d)\n", idx+1, cur)
	} else if action == "turn_off" {
		pkt = lc.offPackets[idx]
		log.Printf("[LIGHT] 조명%d OFF 명령 전송 (캐시상태: %d)\n", idx+1, cur)
	} else {
		return
	}

	select {
	case lc.pauseStatusQuery <- true:
	default:
	}

	lc.serialMu.Lock()
	if lc.port != nil {
		drainBuf := make([]byte, 128)
		for {
			dn, _ := lc.port.Read(drainBuf)
			if dn == 0 {
				break
			}
		}

		hexBytes, _ := hex.DecodeString(pkt)
		lc.port.Write(hexBytes)
	} else {
		log.Printf("[LIGHT] 포트 재연결 중 — 명령 무시")
	}
	lc.serialMu.Unlock()

	time.Sleep(100 * time.Millisecond)

	select {
	case lc.resumeStatusQuery <- true:
	default:
	}
}

func (lc *LightController) BroadcastAll() {
	lc.statusMu.Lock()
	defer lc.statusMu.Unlock()

	for i := 0; i < 5; i++ {
		// prevStatus starts at -1 and is only set once a real poll response has
		// been seen; lightStatus starts at 0 so it can't flag "unknown". Skip
		// lights we've never actually observed to avoid broadcasting a fake "off".
		if lc.prevStatus[i] == -1 {
			continue
		}
		cur := lc.lightStatus[i]
		stateStr := "off"
		if cur == 1 {
			stateStr = "on"
		}
		lc.wsServer.Broadcast(ws.WSMsg{
			Type:     "event",
			Domain:   "light",
			DeviceID: fmt.Sprintf("light_%d", i+1),
			State:    stateStr,
		})
	}
}

func (lc *LightController) Close() {
	if lc.port != nil {
		lc.port.Close()
	}
}
