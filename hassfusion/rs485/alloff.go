package rs485

import (
	"log"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/ws"
)

type AllOffController struct {
	port       *serial.Port
	wsServer   *ws.Server
	lastState  string
	readBuffer []byte
	packetBuf  []byte

	statusReqPacket []byte
	onCmdPacket     []byte
	offCmdPacket    []byte
	elevatorPacket  []byte

	pauseChan     chan bool
	resumeChan    chan bool
	isQueryPaused bool
	statusMu      sync.Mutex
}

func NewAllOffController(devicePath string, wsServer *ws.Server) *AllOffController {
	if devicePath == "" {
		return nil
	}

	config := &serial.Config{
		Name:        devicePath,
		Baud:        9600,
		ReadTimeout: 100 * time.Millisecond,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("Failed to open alloff serial %s: %v", devicePath, err)
		return nil
	}

	ac := &AllOffController{
		port:            port,
		wsServer:        wsServer,
		lastState:       "off",
		readBuffer:      make([]byte, 128),
		packetBuf:       make([]byte, 0, 128),
		pauseChan:       make(chan bool, 1),
		resumeChan:      make(chan bool, 1),
		statusReqPacket: []byte{0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21},
		onCmdPacket:     []byte{0x22, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x25},
		offCmdPacket:    []byte{0x22, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x24},
		elevatorPacket:  []byte{0xA0, 0x01, 0x01, 0x00, 0x08, 0x15, 0x00, 0xBF},
	}

	wsServer.RegisterHandler("switch", ac.wsCommandRouter)
	wsServer.RegisterHandler("elevator_button", ac.wsElevatorRouter)

	go ac.pollingLoop()

	return ac
}

func parseAlloffStatusPacket(pkt []byte) (string, bool) {
	if len(pkt) < 8 {
		return "", false
	}
	if pkt[0] != 0xA0 {
		return "", false
	}
	if pkt[1] == 0x01 && pkt[2] == 0x01 {
		return "on", true
	}
	if pkt[1] == 0x00 && pkt[2] == 0x01 {
		return "off", true
	}
	return "", false
}

func (ac *AllOffController) pollingLoop() {
	var lastReadTime time.Time

	for {
		// BLOCKING Pause
		select {
		case <-ac.pauseChan:
			ac.statusMu.Lock()
			ac.isQueryPaused = true
			ac.statusMu.Unlock()
			<-ac.resumeChan
			ac.statusMu.Lock()
			ac.isQueryPaused = false
			ac.statusMu.Unlock()
		default:
		}

		if ac.port != nil {
			// Frequent Pause Check
			select {
			case <-ac.pauseChan:
				ac.statusMu.Lock()
				ac.isQueryPaused = true
				ac.statusMu.Unlock()
				<-ac.resumeChan
				ac.statusMu.Lock()
				ac.isQueryPaused = false
				ac.statusMu.Unlock()
			default:
			}

			// Atomic Write-Read with Lock
			ac.statusMu.Lock()
			_, err := ac.port.Write(ac.statusReqPacket)
			if err != nil {
				ac.statusMu.Unlock()
				log.Printf("[ALLOFF] 상태 요청 패킷 전송 실패: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			time.Sleep(75 * time.Millisecond)
			n, err := ac.port.Read(ac.readBuffer)
			ac.statusMu.Unlock()

			if err == nil && n > 0 {
				now := time.Now()
				if now.Sub(lastReadTime) > 300*time.Millisecond {
					ac.packetBuf = ac.packetBuf[:0]
				}
				lastReadTime = now

				ac.packetBuf = append(ac.packetBuf, ac.readBuffer[:n]...)

				for {
					found := false
					for j := 0; j <= len(ac.packetBuf)-8; j++ {
						if ac.packetBuf[j] == 0xA0 {
							state, ok := parseAlloffStatusPacket(ac.packetBuf[j : j+8])
							if ok {
								if state != ac.lastState {
									ac.lastState = state
									ac.broadcastState()
								}
								ac.packetBuf = ac.packetBuf[j+8:]
								found = true
								break
							}
						}
					}
					if !found {
						break
					}
				}

				// [수정3] 버퍼가 넘칠 때 전부 초기화하지 않고 앞부분만 잘라내기
				if len(ac.packetBuf) > 128 {
					ac.packetBuf = ac.packetBuf[len(ac.packetBuf)-64:]
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (ac *AllOffController) broadcastState() {
	ac.wsServer.Broadcast(ws.WSMsg{
		Type:     "event",
		Domain:   "switch",
		DeviceID: "alloff",
		State:    ac.lastState,
	})
	log.Printf("AllOff state broadcast: %s", ac.lastState)
}

func (ac *AllOffController) wsCommandRouter(msg ws.WSMsg) {
	if msg.DeviceID != "alloff" {
		return
	}

	// [수정1] 데드락 방지용 NON-BLOCKING Pause
	select {
	case ac.pauseChan <- true:
	default:
	}

	var pkt []byte
	if msg.Action == "turn_on" {
		pkt = ac.onCmdPacket
	} else if msg.Action == "turn_off" {
		pkt = ac.offCmdPacket
	} else {
		return
	}

	ac.statusMu.Lock()
	if ac.port != nil {
		// 전송 전 쓰레기 데이터 비우기
		drBuf := make([]byte, 128)
		for {
			dn, _ := ac.port.Read(drBuf)
			if dn == 0 {
				break
			}
		}

		ac.port.Write(pkt)
		log.Printf("[ALLOFF] AllOff command sent: %s", msg.Action)

		// [수정2] 명령 전송 직후 기기에서 오는 응답(ACK)을 읽어서 버퍼 꼬임 방지
		time.Sleep(75 * time.Millisecond)
		ac.port.Read(drBuf)
	}
	ac.statusMu.Unlock()

	// 3. Resume Signal
	select {
	case ac.resumeChan <- true:
	default:
	}
}

func (ac *AllOffController) wsElevatorRouter(msg ws.WSMsg) {
	if msg.DeviceID != "elevator_call" {
		return
	}

	// [수정1] 데드락 방지용 NON-BLOCKING Pause
	select {
	case ac.pauseChan <- true:
	default:
	}

	ac.statusMu.Lock()
	if ac.port != nil {
		// 전송 전 쓰레기 데이터 비우기
		drBuf := make([]byte, 128)
		for {
			dn, _ := ac.port.Read(drBuf)
			if dn == 0 {
				break
			}
		}

		ac.port.Write(ac.elevatorPacket)
		log.Printf("[ALLOFF] Elevator call packet sent")

		// [수정2] 엘리베이터 호출 응답(ACK) 읽어내기
		time.Sleep(75 * time.Millisecond)
		ac.port.Read(drBuf)
	}
	ac.statusMu.Unlock()

	// 3. Resume Signal
	select {
	case ac.resumeChan <- true:
	default:
	}
}

func (ac *AllOffController) BroadcastAll() {
	ac.wsServer.Broadcast(ws.WSMsg{
		Type:     "event",
		Domain:   "switch",
		DeviceID: "alloff",
		State:    ac.lastState,
	})
}

func (ac *AllOffController) Close() {
	if ac.port != nil {
		ac.port.Close()
	}
}
