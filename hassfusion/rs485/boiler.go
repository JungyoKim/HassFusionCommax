package rs485

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/tarm/serial"

	"hassfusion/ws"
)

const (
	MaxRooms          = 4
	SerialBaudRate    = 9600
	SerialReadTimeout = 100 * time.Millisecond
	CommandDelay      = 75 * time.Millisecond
	StatusQueryDelay  = 75 * time.Millisecond

	MinTemp = 0x05
	MaxTemp = 0x35

	CmdTypeMode = 0x04
	CmdTypeTemp = 0x03

	ValueHeatOn = 0x81
	ValueOff    = 0x00

	ReadBufferSize = 16
)

type BoilerController struct {
	port     *serial.Port
	wsServer *ws.Server

	statusQueryPackets [MaxRooms][]byte
	boilerStatus       [MaxRooms]byte

	serialMu sync.Mutex
	statusMu sync.RWMutex

	pauseQueryChan  chan struct{}
	resumeQueryChan chan struct{}
	isQueryPaused   bool
	queryPauseMu    sync.Mutex

	prevMode        [MaxRooms]byte
	prevCurrentTemp [MaxRooms]byte
	prevSetTemp     [MaxRooms]byte

	readBuffer []byte
}

func hexStringToBytes(s string) []byte {
	var b []byte
	fmt.Sscanf(s, "%x", &b)
	if len(s)%2 != 0 {
		return nil
	}
	res := make([]byte, len(s)/2)
	for i := 0; i < len(s)/2; i++ {
		fmt.Sscanf(s[i*2:i*2+2], "%02X", &res[i])
	}
	return res
}

func bcdToByte(b byte) byte {
	return (b>>4)*10 + (b & 0x0F)
}

func byteToBcd(b byte) byte {
	return ((b / 10) << 4) | (b % 10)
}

func NewBoilerController(portName string, wsServer *ws.Server) *BoilerController {
	if portName == "" {
		return nil
	}

	bc := &BoilerController{
		wsServer:        wsServer,
		pauseQueryChan:  make(chan struct{}, 1), // [수정] 채널 버퍼 추가
		resumeQueryChan: make(chan struct{}, 1), // [수정] 채널 버퍼 추가
		readBuffer:      make([]byte, ReadBufferSize),
	}

	statusQueryStrings := [MaxRooms]string{
		"0201000000000003",
		"0202000000000004",
		"0203000000000005",
		"0204000000000006",
	}

	for i, hex := range statusQueryStrings {
		bc.statusQueryPackets[i] = hexStringToBytes(hex)
	}

	for i := 0; i < MaxRooms; i++ {
		bc.prevMode[i] = 0xFF
		bc.prevCurrentTemp[i] = 0xFF
		bc.prevSetTemp[i] = 0xFF
	}

	if err := bc.initSerial(portName); err != nil {
		log.Printf("Failed to init boiler serial: %v", err)
		return nil
	}

	wsServer.RegisterHandler("climate", bc.wsCommandRouter)
	go bc.startStatusMonitoring()

	return bc
}

func (bc *BoilerController) initSerial(portName string) error {
	config := &serial.Config{
		Name:        portName,
		Baud:        SerialBaudRate,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: SerialReadTimeout,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		return err
	}
	bc.port = port
	log.Println("[BOILER] 시리얼 포트 연결 성공!")
	return nil
}

func (bc *BoilerController) startStatusMonitoring() {
	buf := make([]byte, 1024)
	bufLen := 0

	for {
		select {
		case <-bc.pauseQueryChan:
			bc.queryPauseMu.Lock()
			bc.isQueryPaused = true
			bc.queryPauseMu.Unlock()
			<-bc.resumeQueryChan
			bc.queryPauseMu.Lock()
			bc.isQueryPaused = false
			bc.queryPauseMu.Unlock()
		default:
		}

		for i := 0; i < MaxRooms; i++ {
			select {
			case <-bc.pauseQueryChan:
				bc.queryPauseMu.Lock()
				bc.isQueryPaused = true
				bc.queryPauseMu.Unlock()
				<-bc.resumeQueryChan
				bc.queryPauseMu.Lock()
				bc.isQueryPaused = false
				bc.queryPauseMu.Unlock()
			default:
			}

			bc.serialMu.Lock()
			if bc.port != nil {
				_, err := bc.port.Write(bc.statusQueryPackets[i])
				if err != nil {
					log.Printf("[BOILER] 상태 조회 중 시리얼 포트 쓰기 에러: %v\n", err)
					bc.serialMu.Unlock()
					time.Sleep(1 * time.Second)
					continue
				}

				n, err := bc.port.Read(bc.readBuffer)
				bc.serialMu.Unlock()

				if err != nil && err.Error() != "EOF" {
					log.Printf("[BOILER] 상태 조회 중 시리얼 포트 읽기 에러: %v\n", err)
					time.Sleep(1 * time.Second)
					continue
				}

				if n > 0 {
					copy(buf[bufLen:], bc.readBuffer[:n])
					bufLen += n

					parsedOffset := 0
					for j := 0; j <= bufLen-8; j++ {
						if buf[j] == 0x82 || buf[j] == 0x84 {
							if bc.validateChecksum(buf[j : j+8]) {
								bc.processStatusResponse(buf[j : j+8])
								parsedOffset = j + 8
								j += 7
							}
						}
					}

					if parsedOffset > 0 {
						copy(buf, buf[parsedOffset:bufLen])
						bufLen -= parsedOffset
					} else if bufLen > 16 {
						// [수정] 무작정 8바이트를 날리지 않고, 다음 유효 헤더를 찾을 때까지 1바이트씩 밀어냅니다.
						for bufLen > 0 && buf[0] != 0x82 && buf[0] != 0x84 {
							copy(buf, buf[1:bufLen])
							bufLen--
						}
						// 그래도 버퍼가 16바이트 이상 쌓여있다면, 안전을 위해 최근 8바이트만 남깁니다.
						if bufLen > 16 {
							copy(buf, buf[bufLen-8:bufLen])
							bufLen = 8
						}
					}
				}
			} else {
				bc.serialMu.Unlock()
			}
			time.Sleep(StatusQueryDelay)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (bc *BoilerController) processStatusResponse(packet []byte) {
	if len(packet) != 8 {
		return
	}

	room := packet[2]
	if room < 1 || room > MaxRooms {
		return
	}
	roomIndex := int(room) - 1

	stateByte := packet[1]
	currentTemp := packet[3]
	setTemp := packet[4]

	var newMode byte = 0
	if stateByte == 0x81 || stateByte == 0x83 {
		newMode = 1
	}

	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()

	modeChanged := bc.prevMode[roomIndex] != newMode || bc.prevMode[roomIndex] == 0xFF
	tempChanged := bc.prevCurrentTemp[roomIndex] != currentTemp || bc.prevSetTemp[roomIndex] != setTemp

	if modeChanged || tempChanged {
		bc.boilerStatus[roomIndex] = newMode
		bc.prevMode[roomIndex] = newMode
		bc.prevCurrentTemp[roomIndex] = currentTemp
		bc.prevSetTemp[roomIndex] = setTemp

		modeStr := "off"
		if newMode == 1 {
			modeStr = "heat"
		}

		bc.wsServer.Broadcast(ws.WSMsg{
			Type:     "event",
			Domain:   "climate",
			DeviceID: fmt.Sprintf("boiler_%d", room),
			Attributes: map[string]interface{}{
				"mode":         modeStr,
				"current_temp": int(bcdToByte(currentTemp)),
				"target_temp":  int(bcdToByte(setTemp)),
			},
		})
		log.Printf("[BOILER] %d번 방 emit: mode=%v, current=%v, target=%v", room, modeStr, bcdToByte(currentTemp), bcdToByte(setTemp))
	}
}

func (bc *BoilerController) makeBoilerPacket(room int, cmdType byte, value byte) []byte {
	packet := []byte{0x04, byte(room), cmdType, value, 0x00, 0x00, 0x00, 0x00}
	var sum byte = 0
	for i := 0; i < 7; i++ {
		sum += packet[i]
	}
	packet[7] = sum
	return packet
}

func (bc *BoilerController) validateChecksum(packet []byte) bool {
	if len(packet) != 8 {
		return false
	}
	var sum byte = 0
	for i := 0; i < 7; i++ {
		sum += packet[i]
	}
	return packet[7] == sum
}

func (bc *BoilerController) wsCommandRouter(msg ws.WSMsg) {
	var room int
	if _, err := fmt.Sscanf(msg.DeviceID, "boiler_%d", &room); err != nil {
		return
	}

	if room < 1 || room > MaxRooms {
		return
	}

	log.Printf("[WS] 보일러명령 수신: Action=%s, Val=%v", msg.Action, msg.Value)

	bc.statusMu.RLock()
	currentBoilerState := bc.boilerStatus[room-1]
	bc.statusMu.RUnlock()

	var packet []byte

	if msg.Action == "set_mode" {
		modeStr := fmt.Sprintf("%v", msg.Value)

		// [수정] 캐시 상태(currentBoilerState)와 무관하게 무조건 명령 전송
		if modeStr == "heat" {
			packet = bc.makeBoilerPacket(room, CmdTypeMode, ValueHeatOn)
			log.Printf("[WS] 보일러%d 난방 ON 명령 전송 (캐시상태: %d)\n", room, currentBoilerState)
		} else if modeStr == "off" {
			packet = bc.makeBoilerPacket(room, CmdTypeMode, ValueOff)
			log.Printf("[WS] 보일러%d 난방 OFF 명령 전송 (캐시상태: %d)\n", room, currentBoilerState)
		} else {
			return
		}
	} else if msg.Action == "set_temperature" {
		var temp int
		if t, ok := msg.Value.(float64); ok {
			temp = int(t)
		} else if t, ok := msg.Value.(int); ok {
			temp = t
		} else if tStr, ok := msg.Value.(string); ok {
			if t, err := strconv.Atoi(tStr); err == nil {
				temp = t
			}
		}

		if temp > 0 {
			packet = bc.makeBoilerPacket(room, CmdTypeTemp, byteToBcd(byte(temp)))
			log.Printf("[WS] 보일러%d 온도 %d도 설정 명령 전송\n", room, temp)
		}
	}

	if packet != nil {
		bc.sendCommandAndQuery(packet)
	}
}

func (bc *BoilerController) sendCommandAndQuery(packet []byte) {
	// 1. NON-BLOCKING Pause Signal (데드락 방지)
	select {
	case bc.pauseQueryChan <- struct{}{}:
	default:
	}

	// 2. Transmit command
	bc.serialMu.Lock()
	if bc.port != nil {
		// 명령 보내기 전 이전 쓰레기 데이터 비우기
		drBuf := make([]byte, 128)
		for {
			dn, _ := bc.port.Read(drBuf)
			if dn == 0 {
				break
			}
		}

		_, err := bc.port.Write(packet)
		if err != nil {
			log.Printf("[BOILER] 명령어 에러: %v", err)
		}

		// [수정] 명령 전송 직후 들어오는 보일러의 응답(ACK)을 읽어서 버려줌
		time.Sleep(CommandDelay)
		bc.port.Read(drBuf)

		// 3. Immediately query all rooms
		for i := 0; i < MaxRooms; i++ {
			bc.port.Write(bc.statusQueryPackets[i])
			time.Sleep(StatusQueryDelay)

			n, err := bc.port.Read(bc.readBuffer)
			if err == nil && n >= 8 {
				if bc.readBuffer[0] == 0x82 || bc.readBuffer[0] == 0x84 {
					if bc.validateChecksum(bc.readBuffer[:8]) {
						bc.processStatusResponse(bc.readBuffer[:8])
					}
				}
			}
		}
	}
	bc.serialMu.Unlock()

	// 4. Signal background polling to resume
	select {
	case bc.resumeQueryChan <- struct{}{}:
	default:
	}
}

func (bc *BoilerController) BroadcastAll() {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()

	for i := 0; i < MaxRooms; i++ {
		mode := bc.prevMode[i]
		cur := bc.prevCurrentTemp[i]
		set := bc.prevSetTemp[i]

		if mode == 0xFF || cur == 0xFF || set == 0xFF {
			continue
		}

		modeStr := "off"
		if mode == 1 {
			modeStr = "heat"
		}

		bc.wsServer.Broadcast(ws.WSMsg{
			Type:     "event",
			Domain:   "climate",
			DeviceID: fmt.Sprintf("boiler_%d", i+1),
			Attributes: map[string]interface{}{
				"mode":         modeStr,
				"current_temp": float64(bcdToByte(cur)),
				"target_temp":  float64(bcdToByte(set)),
			},
		})
	}
}

func (bc *BoilerController) Close() {
	if bc.port != nil {
		bc.port.Close()
	}
}
