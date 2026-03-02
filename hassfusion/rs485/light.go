package rs485

import (
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/ws"
)

type LightController struct {
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
	statusOnPrefix     string
	statusOffPrefix    string
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

func (lc *LightController) processStatusResponse(resp string) {
	if len(resp) >= 6 {
		numStr := resp[4:6]
		idx, _ := strconv.ParseInt(numStr, 16, 64)

		if idx >= 1 && idx <= 5 {
			arrayIdx := int(idx) - 1

			lc.statusMu.Lock()
			oldStatus := lc.lightStatus[arrayIdx]
			lc.statusMu.Unlock()

			var newStatus int
			if strings.HasPrefix(resp, lc.statusOnPrefix) {
				newStatus = 1
			} else if strings.HasPrefix(resp, lc.statusOffPrefix) {
				newStatus = 0
			} else {
				return
			}

			if oldStatus != newStatus {
				lc.statusMu.Lock()
				lc.lightStatus[arrayIdx] = newStatus
				lc.statusMu.Unlock()
				lc.publishStateIfChanged(arrayIdx, newStatus)
			}
		}
	}
}

func NewLightController(portName string, wsServer *ws.Server) *LightController {
	if portName == "" {
		return nil
	}

	lc := &LightController{
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
		statusOnPrefix:  "B001",
		statusOffPrefix: "B000",
	}

	if err := lc.connectSerial(portName); err != nil {
		return nil
	}

	wsServer.RegisterHandler("light", lc.wsCommandRouter)
	go lc.statusQueryLoop(portName)

	return lc
}

func (lc *LightController) connectSerial(portName string) error {
	config := &serial.Config{
		Name:        portName,
		Baud:        9600,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: 100 * time.Millisecond,
	}

	for {
		log.Printf("[LIGHT] 시리얼 포트 %s 연결 시도 중...", portName)
		port, err := serial.OpenPort(config)
		if err != nil {
			log.Printf("[LIGHT] 시리얼 포트 연결 실패: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		lc.port = port
		break
	}

	log.Println("[LIGHT] 시리얼 포트 연결 성공!")
	return nil
}

func (lc *LightController) statusQueryLoop(portName string) {
	var packetBuf []byte
	var lastReadTime time.Time

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
			hexBytes, _ := hex.DecodeString(pkt)
			_, err := lc.port.Write(hexBytes)
			if err != nil {
				lc.serialMu.Unlock()
				log.Printf("[LIGHT] 시리얼 포트 쓰기 에러: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			n, err := lc.port.Read(lc.readBuffer)
			lc.serialMu.Unlock()

			if err != nil && err.Error() != "EOF" {
				log.Printf("[LIGHT] 시리얼 포트 읽기 에러: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if n > 0 {
				now := time.Now()
				if now.Sub(lastReadTime) > 200*time.Millisecond {
					packetBuf = packetBuf[:0]
				}
				lastReadTime = now

				packetBuf = append(packetBuf, lc.readBuffer[:n]...)
				resp := strings.ToUpper(hex.EncodeToString(packetBuf))

				idx := strings.Index(resp, "B0")
				if idx != -1 {
					if len(resp) >= idx+6 {
						lc.processStatusResponse(resp[idx:])
						packetBuf = packetBuf[:0]
					}
				} else if len(packetBuf) > 64 {
					// [수정3] 버퍼가 길어졌다고 무조건 날리지 않고, 앞부분만 잘라내어 파편화 패킷 유실 방지
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

	// [수정1] 상태 캐시(cur)와 상관없이 무조건 물리적인 제어 명령을 전송하도록 변경
	if action == "turn_on" {
		pkt = lc.onPackets[idx]
		log.Printf("[LIGHT] 조명%d ON 명령 전송 (캐시상태: %d)\n", idx+1, cur)
	} else if action == "turn_off" {
		pkt = lc.offPackets[idx]
		log.Printf("[LIGHT] 조명%d OFF 명령 전송 (캐시상태: %d)\n", idx+1, cur)
	} else {
		return
	}

	// [수정2] 빠른 연속 클릭 시 채널이 꽉 차서 데드락이 발생하는 것을 막기 위해 select 문 사용
	select {
	case lc.pauseStatusQuery <- true:
	default:
		// 이미 멈춤 신호가 가 있다면 무시
	}

	// 시리얼 포트 접근 권한 획득
	lc.serialMu.Lock()

	// 버퍼에 남은 쓰레기 데이터 비우기 (응답 충돌 방지)
	drainBuf := make([]byte, 128)
	for {
		dn, _ := lc.port.Read(drainBuf)
		if dn == 0 {
			break
		}
	}

	hexBytes, _ := hex.DecodeString(pkt)
	lc.port.Write(hexBytes)

	lc.serialMu.Unlock()

	// 기기가 명령을 처리하고 응답할 충분한 시간 대기
	time.Sleep(100 * time.Millisecond)

	// 상태 조회 재개
	select {
	case lc.resumeStatusQuery <- true:
	default:
	}
}

func (lc *LightController) BroadcastAll() {
	lc.statusMu.Lock()
	defer lc.statusMu.Unlock()

	for i := 0; i < 5; i++ {
		cur := lc.lightStatus[i]
		if cur == -1 {
			continue
		}
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
