package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

type LightController struct {
	port              *serial.Port
	mqttClient        mqtt.Client
	mqttPrefix        string
	lightStatus       []int
	prevStatus        []int
	statusMu          sync.Mutex
	writeMu           sync.Mutex
	pauseStatusQuery  chan bool
	resumeStatusQuery chan bool
	readBuffer        []byte

	// 패킷 정의
	statusQueryPackets [5]string
	onPackets          [5]string
	offPackets         [5]string
	statusOnPrefix     string
	statusOffPrefix    string
}

func NewLightController(portName string, mqttBroker string, mqttPrefix string) *LightController {
	lc := &LightController{
		mqttPrefix:        mqttPrefix,
		lightStatus:       make([]int, 5),
		prevStatus:        []int{-1, -1, -1, -1, -1},
		pauseStatusQuery:  make(chan bool),
		resumeStatusQuery: make(chan bool),
		readBuffer:        make([]byte, 128), // 버퍼 재사용

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

	// 시리얼 포트 연결
	if err := lc.connectSerial(portName); err != nil {
		return nil
	}

	// MQTT 연결
	if err := lc.connectMQTT(mqttBroker); err != nil {
		return nil
	}

	return lc
}

func (lc *LightController) connectSerial(portName string) error {
	var err error

	// 무한 재연결 시도
	for {
		log.Printf("[LIGHT] 시리얼 포트 %s 연결 시도 중...", portName)
		lc.port, err = openSerialLight(portName)
		if err == nil {
			log.Println("[LIGHT] 시리얼 포트 연결 성공!")

			// 연결 상태 모니터링 시작
			go lc.monitorSerialConnection(portName)
			return nil
		}
		log.Printf("[LIGHT] 시리얼 포트 연결 실패: %v. 3초 후 재시도...\n", err)
		time.Sleep(3 * time.Second)
	}
}

// reconnectSerial 시리얼 포트 재연결
func (lc *LightController) reconnectSerial(portName string) error {
	if lc.port != nil {
		lc.port.Close()
		lc.port = nil
	}

	// 무한 재연결 시도
	for {
		sp, err := openSerialLight(portName)
		if err == nil {
			lc.port = sp
			log.Println("[LIGHT] 시리얼 포트 재연결 성공!")
			return nil
		}
		log.Printf("[LIGHT] 시리얼 포트 재연결 실패: %v. 3초 후 재시도...\n", err)
		time.Sleep(3 * time.Second)
	}
}

func (lc *LightController) connectMQTT(mqttBroker string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] MQTT 연결 중 패닉 복구: %v\n", r)
		}
	}()

	// 무한 재연결 시도
	for {
		log.Printf("[LIGHT] MQTT 브로커 %s 연결 시도 중...", mqttBroker)

		opts := mqtt.NewClientOptions().
			AddBroker(mqttBroker).
			SetClientID("usbrs485-light").
			SetCleanSession(false).                            // 세션 유지로 재연결 시 구독 복원
			SetAutoReconnect(true).                            // 자동 재연결 활성화
			SetConnectRetry(true).                             // 연결 재시도 활성화
			SetConnectRetryInterval(10 * time.Second).         // 재연결 간격 증가
			SetMaxReconnectInterval(2 * time.Minute).          // 최대 재연결 간격 증가
			SetConnectionLostHandler(lc.onMQTTConnectionLost). // 연결 끊김 핸들러 추가
			SetOnConnectHandler(lc.onMQTTConnect).             // 연결 성공/복구 핸들러 추가
			SetOrderMatters(false).                            // 메시지 순서 무시로 성능 향상
			SetResumeSubs(true).                               // 재연결 시 구독 자동 복원
			SetKeepAlive(60 * time.Second).                    // Keep-Alive 간격 증가
			SetPingTimeout(20 * time.Second).                  // Ping 타임아웃 증가
			SetConnectTimeout(60 * time.Second)                // 연결 타임아웃 증가

		lc.mqttClient = mqtt.NewClient(opts)

		// 연결 시도 전에 잠시 대기
		time.Sleep(1 * time.Second)

		if token := lc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("[LIGHT] MQTT 연결 실패: %v. 15초 후 재시도...\n", token.Error())
			time.Sleep(15 * time.Second)
			continue
		}

		log.Println("[LIGHT] MQTT 연결 성공!")

		// 연결 상태 모니터링 시작
		go lc.monitorMQTTConnection()
		return nil
	}
}

// onMQTTConnectionLost MQTT 연결 끊김 핸들러
func (lc *LightController) onMQTTConnectionLost(client mqtt.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] MQTT 연결 끊김 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Printf("[LIGHT] MQTT 연결 끊김: %v. 자동 재연결 시도 중...\n", err)

	// 연결 상태 모니터링 시작
	go lc.monitorMQTTConnection()
}

// onMQTTConnect MQTT 연결 성공/복구 핸들러
func (lc *LightController) onMQTTConnect(client mqtt.Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] MQTT 연결 성공 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Println("[LIGHT] MQTT 연결 복구 또는 초기 연결 완료. 구독 설정 중...")

	// 연결 상태 확인
	if client.IsConnected() {
		log.Println("[LIGHT] MQTT 클라이언트 연결 상태 확인됨")
		lc.SubscribeMqtt() // 연결 복구 시 구독 재설정

		// 연결 복구 시 즉시 상태 발행 (HA 재부팅 대응)
		go func() {
			time.Sleep(2 * time.Second) // 구독 설정 완료 대기
			lc.publishInitialStates()
		}()
	} else {
		log.Println("[LIGHT] MQTT 클라이언트 연결 상태 확인 실패")
	}
}

// monitorMQTTConnection MQTT 연결 상태를 지속적으로 모니터링합니다.
func (lc *LightController) monitorMQTTConnection() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] MQTT 모니터링 패닉 복구: %v\n", r)
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if lc.mqttClient == nil {
				log.Println("[LIGHT] MQTT 클라이언트가 nil입니다. 모니터링을 중단합니다.")
				return
			}

			// 연결 상태 체크를 안전하게 수행
			if !lc.mqttClient.IsConnected() {
				log.Println("[LIGHT] MQTT 연결이 끊어졌습니다. 재연결 시도 중...")

				// 재연결 시도 전에 잠시 대기
				time.Sleep(2 * time.Second)

				// 수동 재연결 시도
				if token := lc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
					log.Printf("[LIGHT] MQTT 재연결 실패: %v\n", token.Error())
				} else {
					log.Println("[LIGHT] MQTT 재연결 성공!")
					// 재연결 후 구독 재설정
					time.Sleep(1 * time.Second)
					lc.SubscribeMqtt()
				}
			} else {
				log.Println("[LIGHT] MQTT 연결 상태 정상")
			}
		}
	}
}

// monitorSerialConnection 시리얼 포트 연결 상태를 지속적으로 모니터링합니다.
func (lc *LightController) monitorSerialConnection(portName string) {
	ticker := time.NewTicker(60 * time.Second) // 1분마다 체크
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if lc.port == nil {
				log.Println("[LIGHT] 시리얼 포트가 nil입니다. 재연결 시도...")
				if err := lc.reconnectSerial(portName); err != nil {
					log.Printf("[LIGHT] 시리얼 포트 재연결 실패: %v\n", err)
				}
			} else {
				// 간단한 연결 테스트
				testPacket := []byte{0x30, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x31}
				lc.writeMu.Lock()
				_, err := lc.port.Write(testPacket)
				lc.writeMu.Unlock()

				if err != nil {
					log.Printf("[LIGHT] 시리얼 포트 연결 테스트 실패: %v. 재연결 시도...\n", err)
					lc.reconnectSerial(portName) // 무한 재연결 시도
				} else {
					log.Println("[LIGHT] 시리얼 포트 연결 상태 정상")
				}
			}
		}
	}
}

// SubscribeMqtt MQTT 명령 구독
func (lc *LightController) SubscribeMqtt() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] MQTT 구독 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if lc.mqttClient == nil {
		log.Println("[LIGHT] MQTT 클라이언트가 nil입니다. 구독을 건너뜁니다.")
		return
	}

	for i := 1; i <= 5; i++ {
		topic := fmt.Sprintf("%s/lights/%d/set", lc.mqttPrefix, i)
		idx := i - 1

		log.Printf("[LIGHT] MQTT 구독 시도: %s\n", topic)

		token := lc.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			cmd := strings.ToUpper(string(msg.Payload()))
			log.Printf("[LIGHT] MQTT 명령 수신 - 토픽: %s, 명령: %s, 조명%d 현재상태: %d\n", msg.Topic(), cmd, idx+1, lc.lightStatus[idx])

			var pkt string

			lc.statusMu.Lock()
			cur := lc.lightStatus[idx]
			lc.statusMu.Unlock()

			if cmd == "ON" && cur == 0 {
				pkt = lc.onPackets[idx]
				log.Printf("[LIGHT] 조명%d ON 명령 전송: %s\n", idx+1, pkt)
			} else if cmd == "OFF" && cur == 1 {
				pkt = lc.offPackets[idx]
				log.Printf("[LIGHT] 조명%d OFF 명령 전송: %s\n", idx+1, pkt)
			} else {
				log.Printf("[LIGHT] 조명%d 명령 무시 - 현재상태: %d, 요청명령: %s\n", idx+1, cur, cmd)
				return
			}

			// 상태 조회 일시 중단 (의도된 동작)
			lc.pauseStatusQuery <- true

			lc.writeMu.Lock()
			_, err := lc.port.Write(hexStringToBytes(pkt))
			lc.writeMu.Unlock()

			if err != nil {
				log.Printf("[LIGHT] 시리얼 포트 쓰기 에러: %v\n", err)
			}

			time.Sleep(75 * time.Millisecond)

			// 상태 조회 재개
			lc.resumeStatusQuery <- true
		})

		if token.Wait() && token.Error() != nil {
			log.Printf("[LIGHT] MQTT 구독 실패: %s, 에러: %v\n", topic, token.Error())
		} else {
			log.Printf("[LIGHT] MQTT 구독 성공: %s\n", topic)
		}
	}
}

func (lc *LightController) publishStateIfChanged(idx int, newStatus int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] 상태 발행 패닉 복구: %v\n", r)
		}
	}()

	lc.statusMu.Lock()
	defer lc.statusMu.Unlock()

	// MQTT 클라이언트 nil 체크
	if lc.mqttClient == nil {
		log.Println("[LIGHT] MQTT 클라이언트가 nil입니다. 상태 발행을 건너뜁니다.")
		return
	}

	// 상태가 변경된 경우에만 MQTT 발행 (최적화)
	if lc.prevStatus[idx] != newStatus {
		topic := fmt.Sprintf("%s/lights/%d/state", lc.mqttPrefix, idx+1)
		statusStr := onOff(newStatus)
		log.Printf("[LIGHT] 조명 %d 상태 발행: %s -> %s\n", idx+1, topic, statusStr)

		token := lc.mqttClient.Publish(topic, 0, false, statusStr)
		if token.Wait() && token.Error() != nil {
			log.Printf("[LIGHT] 조명 %d 상태 발행 실패: %v\n", idx+1, token.Error())
		}

		lc.prevStatus[idx] = newStatus
	} else {
		log.Printf("[LIGHT] 조명 %d 상태 변경 없음 (현재: %d, 이전: %d)\n", idx+1, newStatus, lc.prevStatus[idx])
	}
}

func (lc *LightController) processStatusResponse(resp string) {
	log.Printf("[LIGHT] 상태 응답 처리 시작: %s\n", resp)

	if len(resp) >= 6 {
		numStr := resp[4:6]
		log.Printf("[LIGHT] 조명 번호 추출: %s\n", numStr)

		idx, err := strconv.ParseInt(numStr, 16, 0)
		if err != nil {
			log.Printf("[LIGHT] 조명 번호 파싱 에러: %v\n", err)
			return
		}

		if idx >= 1 && idx <= 5 {
			arrayIdx := int(idx) - 1
			log.Printf("[LIGHT] 조명 %d 상태 확인 - 배열 인덱스: %d\n", idx, arrayIdx)

			lc.statusMu.Lock()
			oldStatus := lc.lightStatus[arrayIdx]
			lc.statusMu.Unlock()

			var newStatus int

			if strings.HasPrefix(resp, lc.statusOnPrefix) {
				log.Printf("[LIGHT] 조명 %d ON 상태 확인됨\n", idx)
				newStatus = 1
			} else if strings.HasPrefix(resp, lc.statusOffPrefix) {
				log.Printf("[LIGHT] 조명 %d OFF 상태 확인됨\n", idx)
				newStatus = 0
			} else {
				log.Printf("[LIGHT] 조명 %d 알 수 없는 상태 응답 (ON prefix: %s, OFF prefix: %s)\n", idx, lc.statusOnPrefix, lc.statusOffPrefix)
				return
			}

			if oldStatus != newStatus {
				log.Printf("[LIGHT] 조명 %d 상태 업데이트: %d -> %d\n", idx, oldStatus, newStatus)
				lc.statusMu.Lock()
				lc.lightStatus[arrayIdx] = newStatus
				lc.statusMu.Unlock()
				lc.publishStateIfChanged(arrayIdx, newStatus)
			} else {
				log.Printf("[LIGHT] 조명 %d 상태 업데이트: %d -> %d\n", idx, oldStatus, newStatus)
			}
		} else {
			log.Printf("[LIGHT] 조명 번호 범위 오류: %d (1-5 범위 밖)\n", idx)
		}
	} else {
		log.Printf("[LIGHT] 응답 길이 부족: %d (최소 6자리 필요)\n", len(resp))
	}
}

func (lc *LightController) statusQueryLoop(portName string) {
	paused := false
	for {
		select {
		case <-lc.pauseStatusQuery:
			paused = true
			<-lc.resumeStatusQuery // resume 신호까지 대기 (의도된 동작)
			paused = false
		default:
			if paused {
				time.Sleep(75 * time.Millisecond)
				continue
			}

			for _, pkt := range lc.statusQueryPackets {
				log.Printf("[LIGHT] 상태 조회 패킷 전송: %s\n", pkt)

				lc.writeMu.Lock()
				_, err := lc.port.Write(hexStringToBytes(pkt))
				lc.writeMu.Unlock()

				if err != nil {
					log.Printf("[LIGHT] 시리얼 포트 쓰기 에러: %v\n", err)
					log.Printf("[LIGHT] 시리얼 포트 에러 발생, 재연결 시도: %v\n", err)
					if reconnectErr := lc.reconnectSerial(portName); reconnectErr != nil {
						continue
					}
					continue
				}

				n, err := lc.port.Read(lc.readBuffer) // 버퍼 재사용
				if err != nil {
					log.Printf("[LIGHT] 시리얼 포트 읽기 에러: %v\n", err)
					log.Printf("[LIGHT] 시리얼 포트 읽기 에러, 재연결 시도: %v\n", err)
					if reconnectErr := lc.reconnectSerial(portName); reconnectErr != nil {
						continue
					}
					continue
				}

				if n > 0 {
					// 받은 데이터를 hex로 변환
					resp := strings.ToUpper(hex.EncodeToString(lc.readBuffer[:n]))
					log.Printf("[LIGHT] 응답 수신 (길이: %d): %s\n", n, resp)

					if n >= 8 {
						lc.processStatusResponse(resp)
					} else {
						log.Printf("[LIGHT] 응답 길이 부족 (최소 8바이트 필요, 수신: %d바이트)\n", n)
					}
				} else {
					log.Printf("[LIGHT] 응답 없음 (0바이트)\n")
				}

				time.Sleep(75 * time.Millisecond)
			}
			time.Sleep(75 * time.Millisecond)
		}
	}
}

func (lc *LightController) Run(portName string) {
	// 초기 구독은 onMQTTConnect 핸들러에서 자동으로 설정됩니다.
	// lc.SubscribeMqtt() // 이 줄을 제거

	// 시리얼 포트가 nil인지 확인
	if lc.port == nil {
		log.Println("[LIGHT] 시리얼 포트가 nil입니다. 조명 컨트롤러 실행을 중단합니다.")
		return
	}

	// 초기 상태 발행
	lc.publishInitialStates()

	// 상태 조회 및 MQTT 상태 발행 고루틴
	go lc.statusQueryLoop(portName)

	// 정기적인 상태 발행 (HA 재부팅 대응)
	go lc.periodicStatePublish()

	select {} // 고루틴 대기
}

// publishInitialStates 초기 상태를 MQTT에 발행합니다.
func (lc *LightController) publishInitialStates() {
	log.Println("[LIGHT] 초기 상태 발행 시작...")

	for i := 1; i <= 5; i++ {
		topic := fmt.Sprintf("%s/lights/%d/state", lc.mqttPrefix, i)
		statusStr := "OFF" // 기본값은 OFF

		log.Printf("[LIGHT] 조명 %d 초기 상태 발행: %s -> %s\n", i, topic, statusStr)

		token := lc.mqttClient.Publish(topic, 0, false, statusStr)
		if token.Wait() && token.Error() != nil {
			log.Printf("[LIGHT] 조명 %d 초기 상태 발행 실패: %v\n", i, token.Error())
		}
	}

	log.Println("[LIGHT] 초기 상태 발행 완료!")
}

// periodicStatePublish 정기적으로 상태를 발행하여 HA가 장치를 인식할 수 있도록 합니다.
func (lc *LightController) periodicStatePublish() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LIGHT] 정기 상태 발행 패닉 복구: %v\n", r)
		}
	}()

	// 5분마다 상태 발행
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if lc.mqttClient == nil {
				log.Println("[LIGHT] MQTT 클라이언트가 nil입니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			// 연결 상태 확인
			if !lc.mqttClient.IsConnected() {
				log.Println("[LIGHT] MQTT 연결이 끊어져 있습니다. 정기 상태 발행을 건너뜁니다.")
				continue
			}

			log.Println("[LIGHT] 정기 상태 발행 시작...")

			// 현재 상태를 모두 발행
			lc.statusMu.Lock()
			for i := 0; i < 5; i++ {
				topic := fmt.Sprintf("%s/lights/%d/state", lc.mqttPrefix, i+1)
				statusStr := onOff(lc.lightStatus[i])

				log.Printf("[LIGHT] 조명 %d 정기 상태 발행: %s -> %s\n", i+1, topic, statusStr)

				token := lc.mqttClient.Publish(topic, 0, false, statusStr)
				if token.Wait() && token.Error() != nil {
					log.Printf("[LIGHT] 조명 %d 정기 상태 발행 실패: %v\n", i+1, token.Error())
				}
			}
			lc.statusMu.Unlock()

			log.Println("[LIGHT] 정기 상태 발행 완료!")
		}
	}
}

func onOff(val int) string {
	if val == 1 {
		return "ON"
	}
	return "OFF"
}

// openSerialLight 조명용 시리얼 포트를 엽니다.
func openSerialLight(portName string) (*serial.Port, error) {
	// 시뮬레이션 모드 체크
	if len(portName) >= 4 && portName[:4] == "SIM_" {
		log.Printf("[LIGHT] 시뮬레이션 모드: %s 포트 사용\n", portName)
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

func RunLightController(portName string, mqttBroker string, mqttPrefix string) {
	controller := NewLightController(portName, mqttBroker, mqttPrefix)
	if controller == nil {
		log.Println("[LIGHT] 컨트롤러 초기화 실패")
		return
	}
	controller.Run(portName)
}
