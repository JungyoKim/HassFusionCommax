package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// ElevatorController: 엘리베이터 호출 관리 구조체
type ElevatorController struct {
	port       string
	mqttBroker string
	mqttPrefix string
	serialPort *serial.Port
	mqttClient mqtt.Client
	state      string
	callChan   chan bool
	writeMu    sync.Mutex
	alloff     *AllOffSwitchController
}

// 엘리베이터 호출 패킷 (8바이트 고정, 새 값)
var elevatorCallPacket = []byte{0xA0, 0x01, 0x01, 0x00, 0x08, 0x15, 0x00, 0xBF}

// 엘리베이터 컨트롤러 생성자 (시리얼, MQTT 초기화)
func NewElevatorController(port string, mqttBroker string, mqttPrefix string) *ElevatorController {
	ec := &ElevatorController{
		port:       port,
		mqttBroker: mqttBroker,
		mqttPrefix: mqttPrefix,
		state:      "idle",
		callChan:   make(chan bool),
	}

	// 시리얼 포트 초기화 (무한 재연결 루프)
	var sp *serial.Port
	var err error

	// 무한 재연결 시도
	for {
		log.Printf("[ELEVATOR] 시리얼 포트 %s 연결 시도 중...", port)
		sp, err = openSerialElevator(port)
		if err == nil {
			log.Println("[ELEVATOR] 시리얼 포트 연결 성공!")
			break
		}
		log.Printf("[ELEVATOR] 시리얼 포트 연결 실패: %v. 3초 후 재시도...\n", err)
		time.Sleep(3 * time.Second)
	}

	ec.serialPort = sp

	// 연결 상태 모니터링 시작
	go ec.monitorSerialConnection(port)

	// MQTT 클라이언트 초기화 - 무한 재연결 시도
	for {
		log.Printf("[ELEVATOR] MQTT 브로커 %s 연결 시도 중...", mqttBroker)

		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ELEVATOR] MQTT 연결 중 패닉 복구: %v\n", r)
			}
		}()

		opts := mqtt.NewClientOptions().
			AddBroker(mqttBroker).
			SetClientID("usbrs485-elevator").
			SetCleanSession(false).                            // 세션 유지로 재연결 시 구독 복원
			SetAutoReconnect(true).                            // 자동 재연결 활성화
			SetConnectRetry(true).                             // 연결 재시도 활성화
			SetConnectRetryInterval(10 * time.Second).         // 재연결 간격 증가
			SetMaxReconnectInterval(2 * time.Minute).          // 최대 재연결 간격 증가
			SetConnectionLostHandler(ec.onMQTTConnectionLost). // 연결 끊김 핸들러 추가
			SetOnConnectHandler(ec.onMQTTConnect).             // 연결 성공/복구 핸들러 추가
			SetOrderMatters(false).                            // 메시지 순서 무시로 성능 향상
			SetResumeSubs(true).                               // 재연결 시 구독 자동 복원
			SetKeepAlive(60 * time.Second).                    // Keep-Alive 간격 증가
			SetPingTimeout(20 * time.Second).                  // Ping 타임아웃 증가
			SetConnectTimeout(60 * time.Second)                // 연결 타임아웃 증가

		client := mqtt.NewClient(opts)

		// 연결 시도 전에 잠시 대기
		time.Sleep(1 * time.Second)

		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("[ELEVATOR] MQTT 연결 실패: %v. 15초 후 재시도...\n", token.Error())
			time.Sleep(15 * time.Second)
			continue
		}

		ec.mqttClient = client
		log.Println("[ELEVATOR] MQTT 연결 성공!")

		// 연결 상태 모니터링 시작
		go ec.monitorMQTTConnection()
		break
	}

	// 일괄소등 컨트롤러 생성 및 할당
	ec.alloff = NewAllOffSwitchController(ec.serialPort, ec.mqttClient, mqttPrefix, &ec.writeMu)

	return ec
}

// onMQTTConnectionLost MQTT 연결 끊김 핸들러
func (ec *ElevatorController) onMQTTConnectionLost(client mqtt.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ELEVATOR] MQTT 연결 끊김 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Printf("[ELEVATOR] MQTT 연결 끊김: %v. 자동 재연결 시도 중...\n", err)

	// 연결 상태 모니터링 시작
	go ec.monitorMQTTConnection()
}

// onMQTTConnect MQTT 연결 성공/복구 핸들러
func (ec *ElevatorController) onMQTTConnect(client mqtt.Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ELEVATOR] MQTT 연결 성공 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Println("[ELEVATOR] MQTT 연결 복구 또는 초기 연결 완료. 구독 설정 중...")

	// 연결 상태 확인
	if client.IsConnected() {
		log.Println("[ELEVATOR] MQTT 클라이언트 연결 상태 확인됨")
		ec.SubscribeMqtt() // 연결 복구 시 구독 재설정

		// 일괄소등 구독도 함께 복원
		if ec.alloff != nil {
			ec.alloff.SubscribeMqtt()
		}
	} else {
		log.Println("[ELEVATOR] MQTT 클라이언트 연결 상태 확인 실패")
	}
}

// monitorMQTTConnection MQTT 연결 상태를 지속적으로 모니터링합니다.
func (ec *ElevatorController) monitorMQTTConnection() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ELEVATOR] MQTT 모니터링 패닉 복구: %v\n", r)
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if ec.mqttClient == nil {
				log.Println("[ELEVATOR] MQTT 클라이언트가 nil입니다. 모니터링을 중단합니다.")
				return
			}

			// 연결 상태 체크를 안전하게 수행
			if !ec.mqttClient.IsConnected() {
				log.Println("[ELEVATOR] MQTT 연결이 끊어졌습니다. 재연결 시도 중...")

				// 재연결 시도 전에 잠시 대기
				time.Sleep(2 * time.Second)

				// 수동 재연결 시도
				if token := ec.mqttClient.Connect(); token.Wait() && token.Error() != nil {
					log.Printf("[ELEVATOR] MQTT 재연결 실패: %v\n", token.Error())
				} else {
					log.Println("[ELEVATOR] MQTT 재연결 성공!")
					// 재연결 후 구독 재설정
					time.Sleep(1 * time.Second)
					ec.SubscribeMqtt()
				}
			} else {
				log.Println("[ELEVATOR] MQTT 연결 상태 정상")
			}
		}
	}
}

// monitorSerialConnection 시리얼 포트 연결 상태를 지속적으로 모니터링합니다.
func (ec *ElevatorController) monitorSerialConnection(portName string) {
	ticker := time.NewTicker(60 * time.Second) // 1분마다 체크
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if ec.serialPort == nil {
				log.Println("[ELEVATOR] 시리얼 포트가 nil입니다. 재연결 시도...")
				// 무한 재연결 시도
				for {
					sp, err := openSerialElevator(portName)
					if err == nil {
						ec.serialPort = sp
						log.Println("[ELEVATOR] 시리얼 포트 재연결 성공!")
						break
					}
					log.Printf("[ELEVATOR] 시리얼 포트 재연결 실패: %v. 3초 후 재시도...\n", err)
					time.Sleep(3 * time.Second)
				}
			} else {
				log.Println("[ELEVATOR] 시리얼 포트 연결 상태 정상")
			}
		}
	}
}

// 엘리베이터 호출 패킷 송신 (실제 RS485 송신)
func (ec *ElevatorController) SendCallPacket() {
	// 일괄소등 상태조회 일시정지
	if ec.alloff != nil {
		select {
		case ec.alloff.pauseChan <- true:
			log.Println("[ELEVATOR] 일괄소등 상태조회 일시정지 요청!")
		default:
		}
	}

	ec.writeMu.Lock()
	_, err := ec.serialPort.Write(elevatorCallPacket)
	ec.writeMu.Unlock()
	if err != nil {
		log.Printf("[ELEVATOR] 시리얼 포트 에러 발생, 재연결 시도: %v\n", err)
		ec.serialPort.Close()
		for {
			ec.serialPort, err = openSerialElevator(ec.port)
			if err != nil {
				log.Printf("[ELEVATOR] 시리얼 포트 재연결 실패, 3초 후 재시도: %v\n", err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Println("[ELEVATOR] 시리얼 포트 재연결 성공!")
			break
		}
		return
	}
	log.Printf("[ELEVATOR] 호출 패킷 전송: % X (err: %v)\n", elevatorCallPacket, err)

	// 2초 후 일괄소등 상태조회 재개
	if ec.alloff != nil {
		go func() {
			time.Sleep(2 * time.Second)
			select {
			case ec.alloff.resumeChan <- true:
				log.Println("[ELEVATOR] 일괄소등 상태조회 재개 요청!")
			default:
			}
		}()
	}
}

// MQTT 명령 구독 (실제 구현)
func (ec *ElevatorController) SubscribeMqtt() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ELEVATOR] MQTT 구독 패닉 복구: %v\n", r)
		}
	}()

	log.Println("[ELEVATOR] SubscribeMqtt() 진입")

	// MQTT 클라이언트 nil 체크
	if ec.mqttClient == nil {
		log.Println("[ELEVATOR] MQTT 클라이언트가 nil입니다. 구독을 건너뜁니다.")
		return
	}

	topic := fmt.Sprintf("%s/elevator/call/set", ec.mqttPrefix)
	token := ec.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		log.Println("[ELEVATOR] 콜백 함수 진입")
		cmd := string(msg.Payload())
		log.Printf("[ELEVATOR] 호출 명령 수신: topic=%s, payload=%s\n", topic, cmd)
		if cmd == "ON" || cmd == "1" {
			log.Println("[ELEVATOR] callChan <- true (호출 명령 이벤트)")
			ec.callChan <- true
		}
	})
	token.Wait()
	log.Printf("[ELEVATOR] 엘리베이터 호출 명령 구독: %s\n", topic)
}

// 상태 MQTT publish (필요시 호출 직후 사용 가능, 옵션)
func (ec *ElevatorController) PublishState() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ELEVATOR] 상태 발행 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if ec.mqttClient == nil {
		log.Println("[ELEVATOR] MQTT 클라이언트가 nil입니다. 상태 발행을 건너뜁니다.")
		return
	}

	topic := fmt.Sprintf("%s/elevator/state", ec.mqttPrefix)
	ec.mqttClient.Publish(topic, 0, false, ec.state)
	log.Printf("[ELEVATOR] %s → %s\n", topic, ec.state)
}

// 엘리베이터 컨트롤러 메인 루프 (응답 루프 제거, 단순화)
func (ec *ElevatorController) Run() {
	// 초기 구독은 onMQTTConnect 핸들러에서 자동으로 설정됩니다.
	// ec.SubscribeMqtt() // 이 줄을 제거
	log.Println("[ELEVATOR] Run() 시작, 이벤트 루프 진입")
	for {
		select {
		case <-ec.callChan:
			log.Println("[ELEVATOR] callChan 이벤트 감지 → SendCallPacket() 실행")
			ec.SendCallPacket()
			// 필요시 호출 직후 상태 publish: (예시)
			// ec.state = "called"
			// ec.PublishState()
		}
	}
}

// 보일러/조명과 동일한 스타일의 실행 함수 추가
func RunElevatorController(port, mqttBroker, mqttPrefix string) {
	ec := NewElevatorController(port, mqttBroker, mqttPrefix)

	// alloff 컨트롤러가 nil이 아닌 경우에만 실행
	if ec.alloff != nil {
		go ec.alloff.Run()
	} else {
		log.Println("[ELEVATOR] AllOffSwitchController가 nil입니다. 일괄소등 기능을 건너뜁니다.")
	}

	ec.Run()
}

// 일괄소등 스위치 컨트롤러 구조체
// 조명과 유사한 방식으로 구현

type AllOffSwitchController struct {
	serialPort *serial.Port
	mqttClient mqtt.Client
	mqttPrefix string
	state      string // "ON" or "OFF"
	writeMu    *sync.Mutex
	pauseChan  chan bool
	resumeChan chan bool
}

// 일괄소등 관련 패킷
var (
	alloffStatusReqPacket = []byte{0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21}
	alloffOnCmdPacket     = []byte{0x22, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x25}
	alloffOffCmdPacket    = []byte{0x22, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x24}
)

// 상태 패킷 파싱 (8바이트)
func parseAlloffStatusPacket(pkt []byte) (string, bool) {
	if len(pkt) < 8 {
		return "", false
	}
	if pkt[0] != 0xA0 {
		return "", false
	}
	if pkt[1] == 0x01 && pkt[2] == 0x01 {
		return "ON", true
	}
	if pkt[1] == 0x00 && pkt[2] == 0x01 {
		return "OFF", true
	}
	return "", false
}

// 컨트롤러 생성자 (포트/클라이언트/프리픽스 공유)
func NewAllOffSwitchController(serialPort *serial.Port, mqttClient mqtt.Client, mqttPrefix string, writeMu *sync.Mutex) *AllOffSwitchController {
	return &AllOffSwitchController{
		serialPort: serialPort,
		mqttClient: mqttClient,
		mqttPrefix: mqttPrefix,
		state:      "OFF",
		writeMu:    writeMu,
		pauseChan:  make(chan bool, 1),
		resumeChan: make(chan bool, 1),
	}
}

// MQTT 명령 구독
func (ac *AllOffSwitchController) SubscribeMqtt() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ALLOFF] MQTT 구독 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if ac.mqttClient == nil {
		log.Println("[ALLOFF] MQTT 클라이언트가 nil입니다. 구독을 건너뜁니다.")
		return
	}

	topic := fmt.Sprintf("%s/alloff/set", ac.mqttPrefix)
	ac.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		cmd := string(msg.Payload())
		log.Printf("[ALLOFF] 명령 수신: topic=%s, payload=%s\n", topic, cmd)
		if cmd == "ON" {
			ac.SendCommand(true)
		} else if cmd == "OFF" {
			ac.SendCommand(false)
		}
	})
	log.Printf("[ALLOFF] 명령 구독: %s\n", topic)
}

// 제어 패킷 송신
func (ac *AllOffSwitchController) SendCommand(on bool) {
	ac.writeMu.Lock()
	var pkt []byte
	if on {
		pkt = alloffOnCmdPacket
	} else {
		pkt = alloffOffCmdPacket
	}
	n, err := ac.serialPort.Write(pkt)
	ac.writeMu.Unlock()
	log.Printf("[ALLOFF] 제어 패킷 전송: % X (bytes: %d, err: %v)\n", pkt, n, err)
}

// 상태 MQTT publish
func (ac *AllOffSwitchController) PublishState() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ALLOFF] 상태 발행 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if ac.mqttClient == nil {
		log.Println("[ALLOFF] MQTT 클라이언트가 nil입니다. 상태 발행을 건너뜁니다.")
		return
	}

	topic := fmt.Sprintf("%s/alloff/state", ac.mqttPrefix)
	ac.mqttClient.Publish(topic, 0, false, ac.state)
	log.Printf("[ALLOFF] 상태 publish: %s → %s\n", topic, ac.state)
}

// 상태 주기적 조회 및 패킷 수신 루프
func (ac *AllOffSwitchController) Run() {
	// 초기 구독은 엘리베이터 컨트롤러의 onMQTTConnect 핸들러에서 자동으로 설정됩니다.
	// ac.SubscribeMqtt() // 이 줄을 제거

	// serialPort가 nil인지 확인
	if ac.serialPort == nil {
		log.Println("[ALLOFF] 시리얼 포트가 nil입니다. AllOffSwitchController 실행을 중단합니다.")
		return
	}

	// 초기 상태 발행
	ac.publishInitialState()

	// 정기적인 상태 발행 (HA 재부팅 대응)
	go ac.periodicStatePublish()

	buf := make([]byte, 128)
	paused := false
	for {
		select {
		case <-ac.pauseChan:
			paused = true
			log.Println("[ALLOFF] 상태조회 일시정지!")
			<-ac.resumeChan
			paused = false
			log.Println("[ALLOFF] 상태조회 재개!")
		default:
			if paused {
				time.Sleep(75 * time.Millisecond)
				continue
			}

			// serialPort가 여전히 nil인지 다시 확인
			if ac.serialPort == nil {
				log.Println("[ALLOFF] 시리얼 포트가 nil입니다. 루프를 종료합니다.")
				return
			}

			// 상태 요청 패킷 송신
			ac.writeMu.Lock()
			_, err := ac.serialPort.Write(alloffStatusReqPacket)
			ac.writeMu.Unlock()
			if err != nil {
				log.Printf("[ALLOFF] 상태 요청 패킷 전송 실패: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}
			log.Printf("[ALLOFF] 상태 요청 패킷 전송: % X\n", alloffStatusReqPacket)

			time.Sleep(100 * time.Millisecond)
			n, err := ac.serialPort.Read(buf)
			if err == nil && n >= 8 {
				state, ok := parseAlloffStatusPacket(buf[:8])
				if ok && state != ac.state {
					ac.state = state
					ac.PublishState()
				}
			}
			time.Sleep(75 * time.Millisecond)
		}
	}
}

// publishInitialState 초기 상태를 MQTT에 발행합니다.
func (ac *AllOffSwitchController) publishInitialState() {
	log.Println("[ALLOFF] 초기 상태 발행 시작...")

	topic := fmt.Sprintf("%s/alloff/state", ac.mqttPrefix)
	statusStr := "OFF" // 기본값은 OFF

	log.Printf("[ALLOFF] 일괄소등 초기 상태 발행: %s -> %s\n", topic, statusStr)

	token := ac.mqttClient.Publish(topic, 0, false, statusStr)
	if token.Wait() && token.Error() != nil {
		log.Printf("[ALLOFF] 일괄소등 초기 상태 발행 실패: %v\n", token.Error())
	}

	log.Println("[ALLOFF] 초기 상태 발행 완료!")
}

// periodicStatePublish 정기적으로 상태를 발행하여 HA가 장치를 인식할 수 있도록 합니다.
func (ac *AllOffSwitchController) periodicStatePublish() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ALLOFF] 정기 상태 발행 패닉 복구: %v\n", r)
		}
	}()

	// 5분마다 상태 발행
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if ac.mqttClient == nil {
				log.Println("[ALLOFF] MQTT 클라이언트가 nil입니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			// 연결 상태 확인
			if !ac.mqttClient.IsConnected() {
				log.Println("[ALLOFF] MQTT 연결이 끊어져 있습니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			log.Println("[ALLOFF] 정기 상태 발행 시작...")

			// 현재 상태 발행
			topic := fmt.Sprintf("%s/alloff/state", ac.mqttPrefix)

			log.Printf("[ALLOFF] 정기 상태 발행: %s -> %s\n", topic, ac.state)

			token := ac.mqttClient.Publish(topic, 0, false, ac.state)
			if token.Wait() && token.Error() != nil {
				log.Printf("[ALLOFF] 정기 상태 발행 실패: %v\n", token.Error())
			}

			log.Println("[ALLOFF] 정기 상태 발행 완료!")
		}
	}
}

func openSerialElevator(portName string) (*serial.Port, error) {
	// 시뮬레이션 모드 체크
	if len(portName) >= 4 && portName[:4] == "SIM_" {
		log.Printf("[ELEVATOR] 시뮬레이션 모드: %s 포트 사용\n", portName)
		return &serial.Port{}, nil // 가상 포트 반환
	}

	config := &serial.Config{
		Name:        portName,
		Baud:        9600,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: 100 * time.Millisecond,
	}
	return serial.OpenPort(config)
}
