package rs485

import (
	"log"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/config"
	"hassfusion/ws"
)

type AllOffController struct {
	portSpec   string // 원본 포트 설정값 (e.g., "usb:1-2.1.4")
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

func NewAllOffController(portSpec string, wsServer *ws.Server) *AllOffController {
	if portSpec == "" {
		return nil
	}

	ac := &AllOffController{
		portSpec:        portSpec,
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

	// Connect in the background so a missing/slow USB adapter can't block startup.
	wsServer.RegisterHandler("switch", ac.wsCommandRouter)
	wsServer.RegisterHandler("elevator_button", ac.wsElevatorRouter)
	go func() {
		ac.connectSerial()
		ac.pollingLoop()
	}()

	return ac
}

func (ac *AllOffController) connectSerial() error {
	for {
		portName := config.ResolveSerialPort(ac.portSpec)
		if portName == "" {
			log.Printf("[ALLOFF] USB 장치를 찾을 수 없습니다: %s, 3초 후 재시도...", ac.portSpec)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[ALLOFF] 시리얼 포트 %s 연결 시도 중...", portName)
		port, err := serial.OpenPort(&serial.Config{
			Name:        portName,
			Baud:        9600,
			ReadTimeout: 100 * time.Millisecond,
		})
		if err != nil {
			log.Printf("[ALLOFF] 시리얼 포트 연결 실패: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		ac.statusMu.Lock()
		ac.port = port
		ac.statusMu.Unlock()
		break
	}

	log.Println("[ALLOFF] 시리얼 포트 연결 성공!")
	return nil
}

func (ac *AllOffController) reconnectSerial() {
	ac.statusMu.Lock()
	if ac.port != nil {
		ac.port.Close()
		ac.port = nil
	}
	ac.statusMu.Unlock()

	log.Printf("[ALLOFF] 시리얼 포트 재연결 시도 중... (%s)", ac.portSpec)
	ac.connectSerial()
}

func parseAlloffStatusPacket(pkt []byte) (string, bool) {
	if len(pkt) < 8 || pkt[0] != 0xA0 {
		return "", false
	}
	// Reject noise / bit-flipped frames via the trailing sum checksum before
	// trusting the state byte (unlike lights, status was previously unvalidated).
	var sum byte
	for i := 0; i < 7; i++ {
		sum += pkt[i]
	}
	if sum != pkt[7] {
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
	consecutiveErrors := 0

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
			if ac.port == nil {
				ac.statusMu.Unlock()
				time.Sleep(300 * time.Millisecond)
				continue
			}
			_, err := ac.port.Write(ac.statusReqPacket)
			if err != nil {
				ac.statusMu.Unlock()
				log.Printf("[ALLOFF] 상태 요청 패킷 전송 실패: %v\n", err)
				consecutiveErrors++
				if consecutiveErrors >= 5 {
					consecutiveErrors = 0
					ac.reconnectSerial()
				}
				time.Sleep(1 * time.Second)
				continue
			}

			time.Sleep(75 * time.Millisecond)
			n, err := ac.port.Read(ac.readBuffer)
			ac.statusMu.Unlock()

			if err != nil && err.Error() != "EOF" {
				consecutiveErrors++
				if consecutiveErrors >= 5 {
					consecutiveErrors = 0
					ac.reconnectSerial()
				}
			} else {
				consecutiveErrors = 0
			}

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
		drBuf := make([]byte, 128)
		for {
			dn, _ := ac.port.Read(drBuf)
			if dn == 0 {
				break
			}
		}

		ac.port.Write(pkt)
		log.Printf("[ALLOFF] AllOff command sent: %s", msg.Action)

		time.Sleep(75 * time.Millisecond)
		ac.port.Read(drBuf)
	}
	ac.statusMu.Unlock()

	select {
	case ac.resumeChan <- true:
	default:
	}
}

func (ac *AllOffController) wsElevatorRouter(msg ws.WSMsg) {
	if msg.DeviceID != "elevator_call" {
		return
	}

	select {
	case ac.pauseChan <- true:
	default:
	}

	ac.statusMu.Lock()
	if ac.port != nil {
		drBuf := make([]byte, 128)
		for {
			dn, _ := ac.port.Read(drBuf)
			if dn == 0 {
				break
			}
		}

		ac.port.Write(ac.elevatorPacket)
		log.Printf("[ALLOFF] Elevator call packet sent")

		time.Sleep(75 * time.Millisecond)
		ac.port.Read(drBuf)
	}
	ac.statusMu.Unlock()

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
