package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
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
	port       *serial.Port
	portName   string
	mqttClient mqtt.Client
	mqttPrefix string

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
func NewBoilerController(portName string, mqttBroker string, mqttPrefix string) (*BoilerController, error) {
	bc := &BoilerController{
		portName:        portName,
		mqttPrefix:      mqttPrefix,
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
		// utils.go에 있는 hexStringToBytes 함수를 사용합니다.
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

	if err := bc.initMQTT(mqttBroker); err != nil {
		return nil, fmt.Errorf("MQTT 초기화 실패: %w", err)
	}

	return bc, nil
}

// initSerial 시리얼 포트를 초기화하고 연결합니다.
func (bc *BoilerController) initSerial() error {
	// 무한 재연결 시도
	for {
		log.Printf("[BOILER] 시리얼 포트 %s 연결 시도 중...", bc.portName)
		sp, err := openSerialPort(bc.portName)
		if err == nil {
			bc.port = sp
			log.Println("[BOILER] 시리얼 포트 연결 성공!")
			return nil
		}
		log.Printf("[BOILER] 시리얼 포트 연결 실패: %v. 3초 후 재시도...", err)
		time.Sleep(3 * time.Second)
	}
}

// reconnectSerialPort 시리얼 포트 재연결을 시도합니다.
func (bc *BoilerController) reconnectSerialPort() {
	// 이미 lock된 상태에서 호출될 수 있으므로 defer를 사용하지 않음
	if bc.port != nil {
		bc.port.Close()
		bc.port = nil
	}

	// 무한 재연결 시도
	for {
		log.Printf("[BOILER] 시리얼 포트 %s 재연결 시도 중...", bc.portName)
		sp, err := openSerialPort(bc.portName)
		if err == nil {
			bc.port = sp
			log.Println("[BOILER] 시리얼 포트 재연결 성공!")
			return
		}
		log.Printf("[BOILER] 시리얼 포트 재연결 실패: %v. 3초 후 재시도...", err)
		time.Sleep(3 * time.Second)
	}
}

// monitorSerialConnection 시리얼 포트 연결 상태를 지속적으로 모니터링합니다.
func (bc *BoilerController) monitorSerialConnection() {
	ticker := time.NewTicker(60 * time.Second) // 1분마다 체크
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if bc.port == nil {
				log.Println("[BOILER] 시리얼 포트가 nil입니다. 재연결 시도...")
				bc.reconnectSerialPort()
			} else {
				// 간단한 연결 테스트
				testPacket := []byte{0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
				bc.serialMu.Lock()
				_, err := bc.port.Write(testPacket)
				bc.serialMu.Unlock()

				if err != nil {
					log.Printf("[BOILER] 시리얼 포트 연결 테스트 실패: %v. 재연결 시도...", err)
					bc.reconnectSerialPort()
				} else {
					log.Println("[BOILER] 시리얼 포트 연결 상태 정상")
				}
			}
		}
	}
}

// initMQTT MQTT 클라이언트를 초기화하고 연결합니다.
func (bc *BoilerController) initMQTT(mqttBroker string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 연결 중 패닉 복구: %v", r)
		}
	}()

	// 무한 재연결 시도
	for {
		log.Printf("[BOILER] MQTT 브로커 %s 연결 시도 중...", mqttBroker)

		opts := mqtt.NewClientOptions().
			AddBroker(mqttBroker).
			SetClientID("usbBOILER-boiler").
			SetCleanSession(false).                    // 세션 유지로 재연결 시 구독 복원
			SetAutoReconnect(true).                    // 자동 재연결 활성화
			SetConnectRetry(true).                     // 연결 재시도 활성화
			SetConnectRetryInterval(10 * time.Second). // 재연결 간격 증가
			SetMaxReconnectInterval(2 * time.Minute).  // 최대 재연결 간격 증가
			SetConnectionLostHandler(bc.onMQTTConnectionLost).
			SetOnConnectHandler(bc.onMQTTConnect).
			SetOrderMatters(false).             // 메시지 순서 무시로 성능 향상
			SetResumeSubs(true).                // 재연결 시 구독 자동 복원
			SetKeepAlive(60 * time.Second).     // Keep-Alive 간격 증가
			SetPingTimeout(20 * time.Second).   // Ping 타임아웃 증가
			SetConnectTimeout(60 * time.Second) // 연결 타임아웃 증가

		bc.mqttClient = mqtt.NewClient(opts)

		// 연결 시도 전에 잠시 대기
		time.Sleep(1 * time.Second)

		if token := bc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("[BOILER] MQTT 연결 실패: %v. 15초 후 재시도...\n", token.Error())
			time.Sleep(15 * time.Second)
			continue
		}

		log.Println("[BOILER] MQTT 연결 성공!")

		// 연결 상태 모니터링 시작
		go bc.monitorMQTTConnection()
		return nil
	}
}

// onMQTTConnectionLost MQTT 연결이 끊겼을 때 호출됩니다.
func (bc *BoilerController) onMQTTConnectionLost(client mqtt.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 연결 끊김 핸들러 패닉 복구: %v", r)
		}
	}()

	log.Printf("[BOILER] MQTT 연결 끊김: %v. 자동 재연결 시도 중...", err)
}

// onMQTTConnect MQTT 연결 성공 시 호출됩니다. (재연결 포함)
func (bc *BoilerController) onMQTTConnect(client mqtt.Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 연결 성공 핸들러 패닉 복구: %v", r)
		}
	}()

	log.Println("[BOILER] MQTT 연결 복구 또는 초기 연결 완료. 구독 설정 중...")
	bc.setupMQTTSubscriptions()

	// 연결 복구 시 즉시 상태 발행 (HA 재부팅 대응)
	go func() {
		time.Sleep(2 * time.Second) // 구독 설정 완료 대기
		bc.publishInitialStates()
	}()
}

// monitorMQTTConnection MQTT 연결 상태를 지속적으로 모니터링합니다.
func (bc *BoilerController) monitorMQTTConnection() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 모니터링 패닉 복구: %v", r)
		}
	}()

	ticker := time.NewTicker(30 * time.Second) // 30초마다 체크
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if bc.mqttClient == nil {
				log.Println("[BOILER] MQTT 클라이언트가 nil입니다. 모니터링을 중단합니다.")
				return
			}

			// 연결 상태 체크를 안전하게 수행
			if !bc.mqttClient.IsConnected() {
				log.Println("[BOILER] MQTT 연결이 끊어졌습니다. 재연결 시도 중...")

				// 재연결 시도 전에 잠시 대기
				time.Sleep(2 * time.Second)

				// 재연결 시도
				if token := bc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
					log.Printf("[BOILER] MQTT 재연결 실패: %v", token.Error())
				} else {
					log.Println("[BOILER] MQTT 재연결 성공!")
					// 재연결 후 구독 재설정
					time.Sleep(1 * time.Second)
					bc.setupMQTTSubscriptions()
				}
			} else {
				log.Println("[BOILER] MQTT 연결 상태 정상")
			}
		}
	}
}

// Run 컨트롤러를 실행합니다.
func (bc *BoilerController) Run() {
	// MQTT 연결 모니터링 시작
	go bc.monitorMQTTConnection()

	// 시리얼 연결 모니터링 시작
	go bc.monitorSerialConnection()

	bc.setupMQTTSubscriptions()

	// 초기 상태 조회 및 발행 (실제 장치 상태 조회)
	bc.publishInitialStates()

	// 정기적인 상태 발행 (HA 재부팅 대응)
	go bc.periodicStatePublish()

	// 상태 모니터링 시작 (초기 상태 조회 후 시작)
	bc.startStatusMonitoring()

	select {}
}

// publishInitialStates 초기 상태를 MQTT에 발행합니다.
func (bc *BoilerController) publishInitialStates() {
	log.Println("[BOILER] 초기 상태 조회 및 발행 시작...")

	bc.serialMu.Lock()
	defer bc.serialMu.Unlock()

	if bc.port == nil {
		log.Println("[BOILER] 시리얼 포트가 연결되지 않아 초기 상태 조회 불가")
		return
	}

	// 각 방의 실제 상태를 조회하여 발행
	for i := 0; i < MaxRooms; i++ {
		_, err := bc.port.Write(bc.statusQueryPackets[i])
		if err != nil {
			log.Printf("[BOILER] 초기 상태 쿼리 중 시리얼 포트 쓰기 에러: %v", err)
			continue
		}

		n, err := bc.port.Read(bc.readBuffer)
		if err != nil {
			log.Printf("[BOILER] 초기 상태 쿼리 중 시리얼 포트 읽기 에러: %v", err)
			continue
		}

		if n >= 8 {
			bc.processStatusResponse(bc.readBuffer[:n])
			log.Printf("[BOILER] 방 %d 초기 상태 조회 및 발행 완료", i+1)
		} else {
			log.Printf("[BOILER] 불완전한 초기 상태 응답 수신 (길이 %d): % X", n, bc.readBuffer[:n])
		}
		time.Sleep(StatusQueryDelay)
	}

	log.Println("[BOILER] 초기 상태 조회 및 발행 완료!")
}

// periodicStatePublish 정기적으로 상태를 발행하여 HA가 장치를 인식할 수 있도록 합니다.
func (bc *BoilerController) periodicStatePublish() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] 정기 상태 발행 패닉 복구: %v", r)
		}
	}()

	// 5분마다 상태 발행
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if bc.mqttClient == nil {
				log.Println("[BOILER] MQTT 클라이언트가 nil입니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			// 연결 상태 확인
			if !bc.mqttClient.IsConnected() {
				log.Println("[BOILER] MQTT 연결이 끊어져 있습니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			log.Println("[BOILER] 정기 상태 발행 시작...")

			// 현재 상태를 모두 발행
			bc.statusMu.RLock()
			for room := 1; room <= MaxRooms; room++ {
				idx := room - 1
				mode := "off"
				if bc.boilerStatus[idx] == 1 {
					mode = "heat"
				}

				// mode, 현재온도, 설정온도 모두 발행
				bc.publishMQTTOptimized("mode", room, mode)

				// 온도 값이 유효한 경우에만 발행 (0xFF가 아닌 경우)
				if bc.prevCurrentTemp[idx] != 0xFF {
					bc.publishMQTTOptimized("current_temperature", room, fmt.Sprintf("%X", bc.prevCurrentTemp[idx]))
				}
				if bc.prevSetTemp[idx] != 0xFF {
					bc.publishMQTTOptimized("set_temperature", room, fmt.Sprintf("%X", bc.prevSetTemp[idx]))
				}

				log.Printf("[BOILER] 방 %d 정기 상태 발행: mode=%s, current_temp=%X, set_temp=%X",
					room, mode, bc.prevCurrentTemp[idx], bc.prevSetTemp[idx])
			}
			bc.statusMu.RUnlock()

			log.Println("[BOILER] 정기 상태 발행 완료!")
		}
	}
}

// setupMQTTSubscriptions MQTT 구독을 설정합니다.
func (bc *BoilerController) setupMQTTSubscriptions() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 구독 패닉 복구: %v", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if bc.mqttClient == nil {
		log.Println("[BOILER] MQTT 클라이언트가 nil입니다. 구독을 건너뜁니다.")
		return
	}

	for room := 1; room <= MaxRooms; room++ {
		bc.subscribeModeCommands(room)
		bc.subscribeTemperatureCommands(room)
	}
}

// subscribeModeCommands 모드 명령 구독을 설정합니다.
func (bc *BoilerController) subscribeModeCommands(room int) {
	bc.topicBuffer.Reset()
	bc.topicBuffer.WriteString(bc.mqttPrefix)
	bc.topicBuffer.WriteString("/boilers/")
	bc.topicBuffer.WriteString(strconv.Itoa(room))
	bc.topicBuffer.WriteString("/mode/set")
	topic := bc.topicBuffer.String()

	token := bc.mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		bc.handleModeCommand(room, topic, msg.Payload())
	})
	token.Wait()
	if token.Error() != nil {
		log.Printf("[MQTT] 모드 구독 실패 (%s): %v", topic, token.Error())
	} else {
		log.Printf("[MQTT] 모드 구독 성공: %s", topic)
	}
}

// subscribeTemperatureCommands 온도 명령 구독을 설정합니다.
func (bc *BoilerController) subscribeTemperatureCommands(room int) {
	bc.topicBuffer.Reset()
	bc.topicBuffer.WriteString(bc.mqttPrefix)
	bc.topicBuffer.WriteString("/boilers/")
	bc.topicBuffer.WriteString(strconv.Itoa(room))
	bc.topicBuffer.WriteString("/temperature/set")
	topic := bc.topicBuffer.String()

	token := bc.mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		bc.handleTemperatureCommand(room, topic, msg.Payload())
	})
	token.Wait()
	if token.Error() != nil {
		log.Printf("[MQTT] 온도 구독 실패 (%s): %v", topic, token.Error())
	} else {
		log.Printf("[MQTT] 온도 구독 성공: %s", topic)
	}
}

// handleModeCommand 모드 명령을 처리합니다.
func (bc *BoilerController) handleModeCommand(room int, topic string, payload []byte) {
	log.Printf("[MQTT] 모드 명령 수신: topic=%s, payload=%s", topic, payload)

	var isHeatCmd, isOffCmd bool
	if strings.EqualFold(string(payload), "heat") {
		isHeatCmd = true
	} else if strings.EqualFold(string(payload), "off") {
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
		log.Println("[MQTT] 모드 명령 무시: 이미 해당 상태이거나 잘못된 명령")
		return
	}

	bc.sendCommandAndQuery(packet)
}

// handleTemperatureCommand 온도 명령을 처리합니다.
func (bc *BoilerController) handleTemperatureCommand(room int, topic string, payload []byte) {
	payloadStr := string(payload)
	payloadStr = strings.TrimSpace(payloadStr)
	if dot := strings.Index(payloadStr, "."); dot != -1 {
		payloadStr = payloadStr[:dot]
	}

	val, err := strconv.ParseInt(payloadStr, 16, 0)
	log.Printf("[MQTT] 온도 설정 명령 수신: topic=%s, payload=%s, val=0x%X", topic, payloadStr, val)

	if err != nil {
		log.Printf("[MQTT] 온도 명령 무시: 숫자 변환 실패 - %v", err)
		return
	}
	if byte(val) < MinTemp || byte(val) > MaxTemp {
		log.Printf("[MQTT] 온도 명령 무시: 허용 범위 (0x%X ~ 0x%X) 외 - 0x%X", MinTemp, MaxTemp, byte(val))
		return
	}

	packet := bc.makeBoilerPacket(room, CmdTypeTemp, byte(val))
	bc.sendCommandAndQuery(packet)
}

// sendCommandAndQuery 명령을 전송하고 모든 방의 상태를 조회합니다.
func (bc *BoilerController) sendCommandAndQuery(packet []byte) {
	log.Printf("[BOILER] 보낼 명령 패킷: % X", packet)

	bc.queryPauseMu.Lock()
	if !bc.isQueryPaused {
		select {
		case bc.pauseQueryChan <- struct{}{}:
			bc.isQueryPaused = true
			log.Println("[BOILER] 상태 쿼리 일시 중지 요청")
		default:
			log.Println("[BOILER] 상태 쿼리 일시 중지 요청 무시: 이미 일시 중지 상태이거나 채널이 가득 참")
		}
	}
	bc.queryPauseMu.Unlock()

	bc.serialMu.Lock()

	if bc.port == nil {
		log.Println("[BOILER] 시리얼 포트가 연결되지 않아 명령 전송 불가. 재연결 시도...")
		bc.serialMu.Unlock()
		bc.reconnectSerialPort()
		if bc.port == nil {
			log.Println("[BOILER] 시리얼 포트 재연결 실패. 명령 전송 중단.")
			return
		}
		bc.serialMu.Lock()
	}

	_, err := bc.port.Write(packet)
	if err != nil {
		log.Printf("[BOILER] 시리얼 포트 쓰기 에러 발생: %v. 재연결 시도...", err)
		bc.serialMu.Unlock()
		bc.reconnectSerialPort()
		if bc.port == nil {
			log.Println("[BOILER] 시리얼 포트 재연결 실패. 명령 전송 중단.")
			return
		}
		bc.serialMu.Lock()
	}
	time.Sleep(CommandDelay)

	for i := 0; i < MaxRooms; i++ {
		_, err := bc.port.Write(bc.statusQueryPackets[i])
		if err != nil {
			log.Printf("[BOILER] 시리얼 포트 쓰기 에러 (상태 쿼리 중): %v. 재연결 시도...", err)
			bc.serialMu.Unlock()
			bc.reconnectSerialPort()
			if bc.port == nil {
				log.Println("[BOILER] 시리얼 포트 재연결 실패. 상태 쿼리 중단.")
				break
			}
			bc.serialMu.Lock()
			continue
		}

		n, err := bc.port.Read(bc.readBuffer)
		if err != nil {
			log.Printf("[BOILER] 시리얼 포트 읽기 에러 (상태 쿼리 중): %v. 재연결 시도...", err)
			bc.serialMu.Unlock()
			bc.reconnectSerialPort()
			if bc.port == nil {
				log.Println("[BOILER] 시리얼 포트 재연결 실패. 상태 쿼리 중단.")
				break
			}
			bc.serialMu.Lock()
			continue
		}

		if n >= 8 {
			bc.processStatusResponse(bc.readBuffer[:n])
		} else {
			log.Printf("[BOILER] 불완전한 상태 응답 수신 (길이 %d): % X", n, bc.readBuffer[:n])
		}
		time.Sleep(StatusQueryDelay)
	}

	bc.serialMu.Unlock()

	bc.queryPauseMu.Lock()
	if bc.isQueryPaused {
		select {
		case bc.resumeQueryChan <- struct{}{}:
			bc.isQueryPaused = false
			log.Println("[BOILER] 상태 쿼리 재개 요청")
		default:
			log.Println("[BOILER] 상태 쿼리 재개 요청 무시: 이미 재개 상태이거나 채널이 가득 참")
		}
	}
	bc.queryPauseMu.Unlock()
}

// startStatusMonitoring 상태 모니터링 고루틴을 시작합니다.
func (bc *BoilerController) startStatusMonitoring() {
	go func() {
		for {
			bc.queryPauseMu.Lock()
			isPaused := bc.isQueryPaused
			bc.queryPauseMu.Unlock()

			if isPaused {
				log.Println("[BOILER] 상태 쿼리 루프 일시 중지됨. 재개 신호 대기 중...")
				<-bc.resumeQueryChan
				log.Println("[BOILER] 상태 쿼리 루프 재개됨.")
			}

			bc.serialMu.Lock()
			if bc.port == nil {
				log.Println("[BOILER] 상태 쿼리 중 시리얼 포트 연결 없음. 재연결 시도...")
				bc.serialMu.Unlock()
				bc.reconnectSerialPort()
				time.Sleep(StatusQueryDelay)
				continue
			}

			for i := 0; i < MaxRooms; i++ {
				_, err := bc.port.Write(bc.statusQueryPackets[i])
				if err != nil {
					log.Printf("[BOILER] 상태 쿼리 중 시리얼 포트 쓰기 에러: %v. 재연결 시도...", err)
					bc.serialMu.Unlock()
					bc.reconnectSerialPort()
					time.Sleep(StatusQueryDelay)
					break
				}

				n, err := bc.port.Read(bc.readBuffer)
				if err != nil {
					log.Printf("[BOILER] 상태 쿼리 중 시리얼 포트 읽기 에러: %v. 재연결 시도...", err)
					bc.serialMu.Unlock()
					bc.reconnectSerialPort()
					time.Sleep(StatusQueryDelay)
					break
				}

				if n >= 8 {
					bc.processStatusResponse(bc.readBuffer[:n])
				} else {
					log.Printf("[BOILER] 불완전한 상태 응답 수신 (길이 %d): % X", n, bc.readBuffer[:n])
				}
				time.Sleep(StatusQueryDelay)
			}
			bc.serialMu.Unlock()

			time.Sleep(StatusQueryDelay * 2)
		}
	}()
}

// processStatusResponse 수신된 상태 응답 패킷을 처리합니다.
func (bc *BoilerController) processStatusResponse(data []byte) {
	log.Printf("[BOILER] 수신 패킷: % X", data)

	status := bc.parseBoilerStatusPacket(data)
	if status.Room == 0 || status.Room > MaxRooms {
		log.Printf("[BOILER] 유효하지 않은 보일러 상태 패킷: %X", data)
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

	log.Printf("[BOILER] 방 %d | 모드: %s, 현재온도: %X, 설정온도: %X",
		status.Room, mode, status.CurrentTemp, status.SetTemp)

	// 첫 번째 유효한 데이터인 경우 즉시 발행
	if bc.prevMode[idx] == 0xFF {
		log.Printf("[BOILER] 방 %d 첫 번째 유효한 데이터 수신, 즉시 발행", status.Room)
		bc.publishMQTTOptimized("mode", int(status.Room), mode)
		bc.publishMQTTOptimized("current_temperature", int(status.Room), fmt.Sprintf("%X", status.CurrentTemp))
		bc.publishMQTTOptimized("set_temperature", int(status.Room), fmt.Sprintf("%X", status.SetTemp))

		// 초기값 설정
		bc.prevMode[idx] = 0
		if mode == "heat" {
			bc.prevMode[idx] = 1
		}
		bc.prevCurrentTemp[idx] = status.CurrentTemp
		bc.prevSetTemp[idx] = status.SetTemp
		return
	}

	bc.publishStatusIfChanged(status, int(idx), mode)
}

// publishStatusIfChanged 상태 변경 시에만 MQTT 메시지를 발행합니다.
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
		bc.publishMQTTOptimized("mode", int(status.Room), mode)
	}

	if currentTempChanged {
		bc.publishMQTTOptimized("current_temperature", int(status.Room), fmt.Sprintf("%X", status.CurrentTemp))
	}

	if setTempChanged {
		bc.publishMQTTOptimized("set_temperature", int(status.Room), fmt.Sprintf("%X", status.SetTemp))
	}
}

// publishMQTTOptimized 최적화된 MQTT 메시지 발행 함수.
func (bc *BoilerController) publishMQTTOptimized(msgType string, room int, payload string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BOILER] MQTT 발행 패닉 복구: %v", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if bc.mqttClient == nil {
		log.Println("[BOILER] MQTT 클라이언트가 nil입니다. 상태 발행을 건너뜁니다.")
		return
	}

	bc.topicBuffer.Reset()
	bc.topicBuffer.WriteString(bc.mqttPrefix)
	bc.topicBuffer.WriteString("/boilers/")
	bc.topicBuffer.WriteString(strconv.Itoa(room))
	bc.topicBuffer.WriteByte('/')
	bc.topicBuffer.WriteString(msgType)
	topic := bc.topicBuffer.String()

	log.Printf("[MQTT] 상태 publish: topic=%s, payload=%s", topic, payload)
	token := bc.mqttClient.Publish(topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		log.Printf("[MQTT] 상태 publish 실패 (%s): %v", topic, token.Error())
	}
}

// parseBoilerStatusPacket 보일러 상태 패킷을 파싱합니다.
func (bc *BoilerController) parseBoilerStatusPacket(pkt []byte) BoilerStatus {
	if len(pkt) < 8 {
		log.Printf("[BOILER] 패킷 길이 부족: % X", pkt)
		return BoilerStatus{}
	}

	var sum byte = 0
	for i := 0; i < 7; i++ {
		sum += pkt[i]
	}
	if pkt[7] != sum {
		log.Printf("[BOILER] 체크섬 불일치 (계산: 0x%X, 수신: 0x%X): % X", sum, pkt[7], pkt)
		return BoilerStatus{}
	}

	if pkt[0] != StatusResponseHeader && pkt[0] != ControlResponseHeader {
		log.Printf("[BOILER] 유효하지 않은 헤더 (0x%X): % X", pkt[0], pkt)
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

// RunBoilerController는 메인 함수에서 보일러 컨트롤러를 실행합니다.
// 이 함수는 boiler.go 파일의 package main에 속해있으며, main.go에서 직접 호출됩니다.
func RunBoilerController(portName, mqttBroker, mqttPrefix string) {
	bc, err := NewBoilerController(portName, mqttBroker, mqttPrefix)
	if err != nil {
		log.Fatalf("[BOILER] 컨트롤러 생성 실패: %v", err) // 치명적 오류로 프로그램 종료
	}
	bc.Run()
}

// openSerialPort 시리얼 포트를 열고 설정합니다.
func openSerialPort(portName string) (*serial.Port, error) {
	// 시뮬레이션 모드 체크
	if len(portName) >= 4 && portName[:4] == "SIM_" {
		log.Printf("[BOILER] 시뮬레이션 모드: %s 포트 사용", portName)
		return &serial.Port{}, nil // 가상 포트 반환
	}

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
