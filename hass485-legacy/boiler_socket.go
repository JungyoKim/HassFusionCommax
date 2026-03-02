package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

// 상수 정의
const (
	MaxRooms          = 4
	SerialBaudRate    = 9600
	SerialReadTimeout = 100 * time.Millisecond
	CommandDelay      = 75 * time.Millisecond
	StatusQueryDelay  = 75 * time.Millisecond

	MinTemp = 0x05
	MaxTemp = 0x35

	StatusResponseHeader  = 0x82
	ControlResponseHeader = 0x84

	StateHeating = 0x83
	StateIdle    = 0x81
	StateOff     = 0x84

	CmdTypeMode = 0x04
	CmdTypeTemp = 0x03

	ValueHeatOn = 0x81
	ValueOff    = 0x00

	ReadBufferSize = 16
)

// BoilerStatus 보일러 상태 구조체
type BoilerStatus struct {
	Room        byte
	State       byte
	CurrentTemp byte
	SetTemp     byte
}

// BoilerController 보일러 컨트롤러 구조체
type BoilerController struct {
	port         *serial.Port
	portName     string
	socketClient *SocketClient
	prefix       string

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

	readBuffer  []byte
	topicBuffer strings.Builder
}

// NewBoilerController 새로운 보일러 컨트롤러를 생성합니다.
func NewBoilerController(portName string, socketPath string, prefix string) (*BoilerController, error) {
	bc := &BoilerController{
		portName:        portName,
		prefix:          prefix,
		pauseQueryChan:  make(chan struct{}),
		resumeQueryChan: make(chan struct{}),
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

	if err := bc.initSerial(); err != nil {
		return nil, fmt.Errorf("시리얼 포트 초기화 실패: %w", err)
	}

	if err := bc.initSocket(socketPath); err != nil {
		return nil, fmt.Errorf("소켓 초기화 실패: %w", err)
	}

	return bc, nil
}

// initSerial 시리얼 포트를 초기화하고 연결합니다.
func (bc *BoilerController) initSerial() error {
	var err error
	for {
		log.Printf("[보일러] 시리얼 포트 %s 연결 시도 중...", bc.portName)
		bc.port, err = openSerialPort(bc.portName)
		if err != nil {
			log.Printf("[보일러] 시리얼 포트 연결 실패: %v. 3초 후 재시도.", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("[보일러] 시리얼 포트 연결 성공!")
		return nil
	}
}

// reconnectSerialPort 시리얼 포트 재연결을 시도합니다.
func (bc *BoilerController) reconnectSerialPort() {
	bc.serialMu.Lock()
	defer bc.serialMu.Unlock()

	if bc.port != nil {
		bc.port.Close()
		bc.port = nil
	}

	var err error
	for {
		log.Printf("[보일러] 시리얼 포트 %s 재연결 시도 중...", bc.portName)
		bc.port, err = openSerialPort(bc.portName)
		if err != nil {
			log.Printf("[보일러] 시리얼 포트 재연결 실패: %v. 3초 후 재시도.", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("[보일러] 시리얼 포트 재연결 성공!")
		return
	}
}

// initSocket 소켓 클라이언트를 초기화하고 연결합니다.
func (bc *BoilerController) initSocket(socketPath string) error {
	bc.socketClient = NewSocketClient(socketPath, bc.prefix)
	if err := bc.socketClient.Connect(); err != nil {
		return err
	}

	log.Println("[보일러] 소켓 초기 연결 성공!")
	return nil
}

// Run 컨트롤러를 실행합니다.
func (bc *BoilerController) Run() {
	bc.setupSocketSubscriptions()
	bc.startStatusMonitoring()

	select {}
}

// setupSocketSubscriptions 소켓 구독을 설정합니다.
func (bc *BoilerController) setupSocketSubscriptions() {
	for room := 1; room <= MaxRooms; room++ {
		bc.subscribeModeCommands(room)
		bc.subscribeTemperatureCommands(room)
	}
}

// subscribeModeCommands 모드 명령 구독을 설정합니다.
func (bc *BoilerController) subscribeModeCommands(room int) {
	path := fmt.Sprintf("/boilers/%d/mode/set", room)

	bc.socketClient.Subscribe(path, func(msg SocketMessage) {
		bc.handleModeCommand(room, path, msg.Value)
	})
	log.Printf("[소켓] 모드 구독 성공: %s", path)
}

// subscribeTemperatureCommands 온도 명령 구독을 설정합니다.
func (bc *BoilerController) subscribeTemperatureCommands(room int) {
	path := fmt.Sprintf("/boilers/%d/temperature/set", room)

	bc.socketClient.Subscribe(path, func(msg SocketMessage) {
		bc.handleTemperatureCommand(room, path, msg.Value)
	})
	log.Printf("[소켓] 온도 구독 성공: %s", path)
}

// handleModeCommand 모드 명령을 처리합니다.
func (bc *BoilerController) handleModeCommand(room int, path string, value interface{}) {
	log.Printf("[소켓] 모드 명령 수신: path=%s, value=%v", path, value)

	var isHeatCmd, isOffCmd bool
	cmdStr := strings.ToLower(fmt.Sprintf("%v", value))
	if cmdStr == "heat" {
		isHeatCmd = true
	} else if cmdStr == "off" {
		isOffCmd = true
	}

	bc.statusMu.RLock()
	currentBoilerState := bc.boilerStatus[room-1]
	bc.statusMu.RUnlock()

	var packet []byte
	switch {
	case isHeatCmd && currentBoilerState == 0:
		packet = bc.makeBoilerPacket(room, CmdTypeMode, ValueHeatOn)
	case isOffCmd && currentBoilerState == 1:
		packet = bc.makeBoilerPacket(room, CmdTypeMode, ValueOff)
	default:
		log.Println("[소켓] 모드 명령 무시: 이미 해당 상태이거나 잘못된 명령")
		return
	}

	bc.sendCommandAndQuery(packet)
}

// handleTemperatureCommand 온도 명령을 처리합니다.
func (bc *BoilerController) handleTemperatureCommand(room int, path string, value interface{}) {
	valStr := fmt.Sprintf("%v", value)
	valStr = strings.TrimSpace(valStr)
	if dot := strings.Index(valStr, "."); dot != -1 {
		valStr = valStr[:dot]
	}

	val, err := strconv.ParseInt(valStr, 16, 0)
	log.Printf("[소켓] 온도 설정 명령 수신: path=%s, value=%s, val=0x%X", path, valStr, val)

	if err != nil {
		log.Printf("[소켓] 온도 명령 무시: 숫자 변환 실패 - %v", err)
		return
	}
	if byte(val) < MinTemp || byte(val) > MaxTemp {
		log.Printf("[소켓] 온도 명령 무시: 허용 범위 (0x%X ~ 0x%X) 외 - 0x%X", MinTemp, MaxTemp, byte(val))
		return
	}

	packet := bc.makeBoilerPacket(room, CmdTypeTemp, byte(val))
	bc.sendCommandAndQuery(packet)
}

// sendCommandAndQuery 명령을 전송하고 모든 방의 상태를 조회합니다.
func (bc *BoilerController) sendCommandAndQuery(packet []byte) {
	log.Printf("[RS485] 보낼 명령 패킷: % X", packet)

	bc.serialMu.Lock()
	defer bc.serialMu.Unlock()

	if bc.port == nil {
		log.Println("[보일러] 시리얼 포트가 연결되지 않아 명령 전송 불가. 재연결 시도...")
		bc.reconnectSerialPort()
		if bc.port == nil {
			log.Println("[보일러] 시리얼 포트 재연결 실패. 명령 전송 중단.")
			return
		}
	}

	// 명령 전송
	_, err := bc.port.Write(packet)
	if err != nil {
		log.Printf("[보일러] 시리얼 포트 쓰기 에러 발생: %v. 재연결 시도.", err)
		bc.reconnectSerialPort()
		if bc.port == nil {
			log.Println("[보일러] 시리얼 포트 재연결 실패. 명령 전송 중단.")
			return
		}
		return
	}

	log.Printf("[RS485] 명령 전송 완료, 응답 대기 중...")
	time.Sleep(CommandDelay)

	// 명령 응답 확인 (선택적)
	n, err := bc.port.Read(bc.readBuffer)
	if err != nil {
		log.Printf("[보일러] 명령 응답 읽기 에러: %v", err)
	} else if n > 0 {
		log.Printf("[RS485] 명령 응답 수신: % X", bc.readBuffer[:n])
	}
}

// startStatusMonitoring 상태 모니터링 고루틴을 시작합니다.
func (bc *BoilerController) startStatusMonitoring() {
	go func() {
		for {
			bc.serialMu.Lock()

			if bc.port == nil {
				log.Println("[보일러] 상태 쿼리 중 시리얼 포트 연결 없음. 재연결 시도...")
				bc.serialMu.Unlock()
				bc.reconnectSerialPort()
				time.Sleep(StatusQueryDelay)
				continue
			}

			// 모든 방의 상태를 순차적으로 조회
			for i := 0; i < MaxRooms; i++ {
				log.Printf("[RS485] 보일러 %d 상태 조회 중...", i+1)

				_, err := bc.port.Write(bc.statusQueryPackets[i])
				if err != nil {
					log.Printf("[보일러] 상태 쿼리 중 시리얼 포트 쓰기 에러: %v. 재연결 시도.", err)
					bc.serialMu.Unlock()
					bc.reconnectSerialPort()
					time.Sleep(StatusQueryDelay)
					goto nextIteration
				}

				n, err := bc.port.Read(bc.readBuffer)
				if err != nil {
					log.Printf("[보일러] 상태 쿼리 중 시리얼 포트 읽기 에러: %v. 재연결 시도.", err)
					bc.serialMu.Unlock()
					bc.reconnectSerialPort()
					time.Sleep(StatusQueryDelay)
					goto nextIteration
				}

				if n >= 8 {
					bc.processStatusResponse(bc.readBuffer[:n])
				} else {
					log.Printf("[RS485] 불완전한 상태 응답 수신 (길이 %d): % X", n, bc.readBuffer[:n])
				}
				time.Sleep(StatusQueryDelay)
			}

			bc.serialMu.Unlock()
			time.Sleep(StatusQueryDelay * 2)
			continue

		nextIteration:
			time.Sleep(StatusQueryDelay * 2)
		}
	}()
}

// processStatusResponse 수신된 상태 응답 패킷을 처리합니다.
func (bc *BoilerController) processStatusResponse(data []byte) {
	log.Printf("[RS485] 수신 패킷: % X", data)

	status := bc.parseBoilerStatusPacket(data)
	if status.Room == 0 || status.Room > MaxRooms {
		log.Printf("[RS485] 유효하지 않은 보일러 상태 패킷: %X", data)
		return
	}

	idx := status.Room - 1

	bc.statusMu.Lock()
	if status.State == StateHeating || status.State == StateIdle {
		bc.boilerStatus[idx] = 1
	} else {
		bc.boilerStatus[idx] = 0
	}
	bc.statusMu.Unlock()

	var mode string
	if status.State == StateHeating || status.State == StateIdle {
		mode = "heat"
	} else {
		mode = "off"
	}

	log.Printf("[방 %d] 모드: %s, 현재온도: 0x%X, 설정온도: 0x%X",
		status.Room, mode, status.CurrentTemp, status.SetTemp)

	bc.publishStatusIfChanged(status, int(idx), mode)
}

// publishStatusIfChanged 상태 변경 시에만 소켓 메시지를 발행합니다.
func (bc *BoilerController) publishStatusIfChanged(status BoilerStatus, idx int, mode string) {
	var modeChanged, currentTempChanged, setTempChanged bool

	var modeCode byte
	if mode == "heat" {
		modeCode = 1
	} else {
		modeCode = 0
	}

	if bc.prevMode[idx] != modeCode {
		modeChanged = true
		bc.prevMode[idx] = modeCode
	}

	if bc.prevCurrentTemp[idx] != status.CurrentTemp {
		currentTempChanged = true
		bc.prevCurrentTemp[idx] = status.CurrentTemp
	}

	if bc.prevSetTemp[idx] != status.SetTemp {
		setTempChanged = true
		bc.prevSetTemp[idx] = status.SetTemp
	}

	if modeChanged {
		bc.publishSocketOptimized("mode", int(status.Room), mode)
	}

	if currentTempChanged {
		bc.publishSocketOptimized("current_temp", int(status.Room), fmt.Sprintf("%X", status.CurrentTemp))
	}

	if setTempChanged {
		bc.publishSocketOptimized("set_temp", int(status.Room), fmt.Sprintf("%X", status.SetTemp))
		bc.publishSocketOptimized("temperature", int(status.Room), fmt.Sprintf("%X", status.SetTemp))
	}
}

// publishSocketOptimized 최적화된 소켓 메시지 발행 함수.
func (bc *BoilerController) publishSocketOptimized(msgType string, room int, payload string) {
	path := fmt.Sprintf("/boilers/%d/%s", room, msgType)

	log.Printf("[소켓] 상태 publish: path=%s, payload=%s", path, payload)
	bc.socketClient.Publish(path, payload)
}

// parseBoilerStatusPacket 보일러 상태 패킷을 파싱합니다.
func (bc *BoilerController) parseBoilerStatusPacket(pkt []byte) BoilerStatus {
	if len(pkt) < 8 {
		log.Printf("[RS485] 패킷 길이 부족: % X", pkt)
		return BoilerStatus{}
	}

	var sum byte = 0
	for i := 0; i < 7; i++ {
		sum += pkt[i]
	}
	if pkt[7] != sum {
		log.Printf("[RS485] 체크섬 불일치 (계산: 0x%X, 수신: 0x%X): % X", sum, pkt[7], pkt)
		return BoilerStatus{}
	}

	if pkt[0] != StatusResponseHeader && pkt[0] != ControlResponseHeader {
		log.Printf("[RS485] 유효하지 않은 헤더 (0x%X): % X", pkt[0], pkt)
		return BoilerStatus{}
	}

	return BoilerStatus{
		Room:        pkt[2],
		State:       pkt[1],
		CurrentTemp: pkt[3],
		SetTemp:     pkt[4],
	}
}

// makeBoilerPacket 보일러 제어 패킷을 생성합니다.
func (bc *BoilerController) makeBoilerPacket(deviceID int, cmdType byte, value byte) []byte {
	pkt := []byte{
		0x04,
		byte(deviceID),
		cmdType,
		value,
		0x00, 0x00, 0x00,
		0x00,
	}

	var sum byte = 0
	for i := 0; i < 7; i++ {
		sum += pkt[i]
	}
	pkt[7] = sum

	return pkt
}

// handleCommands 보일러 명령을 처리합니다.
func (bc *BoilerController) handleCommands() {
	for {
		select {
		case cmd := <-boilerCommands:
			bc.processBoilerCommand(cmd)
		}
	}
}

// processBoilerCommand 보일러 명령을 처리합니다.
func (bc *BoilerController) processBoilerCommand(cmd BoilerCommand) {
	boilerNum, err := strconv.Atoi(cmd.BoilerNum)
	if err != nil {
		log.Printf("[보일러] 잘못된 보일러 번호: %s", cmd.BoilerNum)
		return
	}

	if boilerNum < 1 || boilerNum > 4 {
		log.Printf("[보일러] 지원하지 않는 보일러 번호: %d", boilerNum)
		return
	}

	log.Printf("[보일러] 명령 처리: 보일러 %d, 액션 %s, 값 %s", boilerNum, cmd.Action, cmd.Value)

	switch cmd.Action {
	case "mode":
		bc.handleModeCommand(boilerNum, fmt.Sprintf("/boilers/%d/mode/set", boilerNum), cmd.Value)
	case "temperature":
		bc.handleTemperatureCommand(boilerNum, fmt.Sprintf("/boilers/%d/temperature/set", boilerNum), cmd.Value)
	default:
		log.Printf("[보일러] 알 수 없는 액션: %s", cmd.Action)
	}
}

// RunBoilerController는 메인 함수에서 보일러 컨트롤러를 실행합니다.
func RunBoilerController(portName, socketPath, prefix string) {
	bc, err := NewBoilerController(portName, socketPath, prefix)
	if err != nil {
		log.Fatalf("[보일러] 컨트롤러 생성 실패: %v", err)
	}

	// 명령 처리 고루틴 시작
	go bc.handleCommands()

	bc.Run()
}

// openSerialPort 시리얼 포트를 열고 설정합니다.
func openSerialPort(portName string) (*serial.Port, error) {
	config := &serial.Config{
		Name:        portName,
		Baud:        SerialBaudRate,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: SerialReadTimeout,
	}
	return serial.OpenPort(config)
}
