// package main

// import (
// 	"log"
// 	"time"

// 	mqtt "github.com/eclipse/paho.mqtt.golang"
// 	"github.com/tarm/serial"
// )

// // 벨 울림/통화 종료/문열기 관련 패킷
// var (
// 	bellRingPacket    = []byte{0x10, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x10, 0x00, 0x00, 0x00, 0x5A, 0x03}
// 	callEndPacket     = []byte{0x02, 0x12, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x61, 0x00, 0x00, 0x05, 0xB2, 0x03}
// 	doorOpenCmdPacket = []byte{0x02, 0x11, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x05, 0x40, 0x00, 0x01, 0x77, 0x03}
// )

// // DoorbellController 구조체
// // RS485 감시, MQTT 연동, 문열기 제어

// type DoorbellController struct {
// 	port       string
// 	mqttBroker string
// 	mqttPrefix string
// 	serialPort *serial.Port
// 	mqttClient mqtt.Client
// 	state      string // "ON"(벨 울림) or "OFF"(대기)
// }

// // 생성자
// func NewDoorbellController(port, mqttBroker, mqttPrefix string) *DoorbellController {
// 	dc := &DoorbellController{
// 		port:       port,
// 		mqttBroker: mqttBroker,
// 		mqttPrefix: mqttPrefix,
// 		state:      "OFF",
// 	}

// 	// 시리얼 포트 초기화 (재연결 루프)
// 	var sp *serial.Port
// 	var err error
// 	for {
// 		sp, err = openSerialDoorbell(port)
// 		if err != nil {
// 			log.Println("[도어벨] 시리얼 포트 연결 실패, 3초 후 재시도:", err)
// 			time.Sleep(3 * time.Second)
// 			continue
// 		}
// 		log.Println("[도어벨] 시리얼 포트 연결 성공!")
// 		break
// 	}
// 	dc.serialPort = sp

// 	// MQTT 클라이언트 초기화
// 	opts := mqtt.NewClientOptions().AddBroker(mqttBroker).SetClientID("usbrs485-doorbell")
// 	client := mqtt.NewClient(opts)
// 	if token := client.Connect(); token.Wait() && token.Error() != nil {
// 		log.Println("[도어벨] MQTT 연결 실패:", token.Error())
// 		return dc
// 	}
// 	dc.mqttClient = client
// 	log.Println("[도어벨] MQTT 연결 성공!")

// 	return dc
// }

// // MQTT 버튼(문열기) 구독
// func (dc *DoorbellController) SubscribeMqtt() {
// 	topic := log.Sprintf("%s/doorbell/open/set", dc.mqttPrefix)
// 	dc.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
// 		cmd := string(msg.Payload())
// 		log.Printf("[MQTT][도어벨] 문열기 명령 수신: topic=%s, payload=%s\n", topic, cmd)
// 		if cmd == "ON" {
// 			dc.SendOpenDoorPacket()
// 		}
// 	})
// 	log.Println("[MQTT][도어벨] 문열기 명령 구독:", topic)
// }

// // 문열기 패킷 송신
// func (dc *DoorbellController) SendOpenDoorPacket() {
// 	_, err := dc.serialPort.Write(doorOpenCmdPacket)
// 	if err != nil {
// 		log.Println("[도어벨] 시리얼 포트 에러 발생, 재연결 시도:", err)
// 		dc.serialPort.Close()
// 		for {
// 			dc.serialPort, err = openSerialDoorbell(dc.port)
// 			if err != nil {
// 				log.Println("[도어벨] 시리얼 포트 재연결 실패, 3초 후 재시도:", err)
// 				time.Sleep(3 * time.Second)
// 				continue
// 			}
// 			log.Println("[도어벨] 시리얼 포트 재연결 성공!")
// 			break
// 		}
// 		return
// 	}
// 	log.Printf("[도어벨] 문열기 패킷 전송: % X (err: %v)\n", doorOpenCmdPacket, err)
// }

// // 벨 울림/통화 종료 상태 MQTT publish
// func (dc *DoorbellController) PublishState(state string) {
// 	topic := log.Sprintf("%s/doorbell/state", dc.mqttPrefix)
// 	dc.mqttClient.Publish(topic, 0, false, state)
// 	log.Printf("[MQTT][도어벨] 상태 publish: %s → %s\n", topic, state)
// }

// // RS485 감시 루프
// func (dc *DoorbellController) Run() {
// 	dc.SubscribeMqtt()
// 	buf := make([]byte, 256)
// 	for {
// 		n, err := dc.serialPort.Read(buf)
// 		if err != nil {
// 			log.Println("[도어벨] 시리얼 포트 읽기 에러, 재연결 시도:", err)
// 			dc.serialPort.Close()
// 			for {
// 				dc.serialPort, err = openSerialDoorbell(dc.port)
// 				if err != nil {
// 					log.Println("[도어벨] 시리얼 포트 재연결 실패, 3초 후 재시도:", err)
// 					time.Sleep(3 * time.Second)
// 					continue
// 				}
// 				log.Println("[도어벨] 시리얼 포트 재연결 성공!")
// 				break
// 			}
// 			continue
// 		}
// 		if n >= 15 {
// 			if matchPacket(buf[:n], bellRingPacket) {
// 				if dc.state != "ON" {
// 					dc.state = "ON"
// 					dc.PublishState("ON")
// 					log.Println("[도어벨] 벨 울림 감지!")
// 				}
// 			} else if matchPacket(buf[:n], callEndPacket) {
// 				if dc.state != "OFF" {
// 					dc.state = "OFF"
// 					dc.PublishState("OFF")
// 					log.Println("[도어벨] 통화 종료 감지!")
// 				}
// 			}
// 		}
// 		time.Sleep(50 * time.Millisecond)
// 	}
// }

// // 패킷 매칭 함수
// func matchPacket(data, pattern []byte) bool {
// 	if len(data) < len(pattern) {
// 		return false
// 	}
// 	for i := 0; i <= len(data)-len(pattern); i++ {
// 		match := true
// 		for j := 0; j < len(pattern); j++ {
// 			if data[i+j] != pattern[j] {
// 				match = false
// 				break
// 			}
// 		}
// 		if match {
// 			return true
// 		}
// 	}
// 	return false
// }

// // 실행 함수 (보일러/조명과 동일 스타일)
// func RunDoorbellController(port, mqttBroker, mqttPrefix string) {
// 	dc := NewDoorbellController(port, mqttBroker, mqttPrefix)
// 	dc.Run()
// }

// func openSerialDoorbell(portName string) (*serial.Port, error) {
// 	config := &serial.Config{
// 		Name:        portName,
// 		Baud:        9600,
// 		Size:        8,
// 		Parity:      serial.ParityNone,
// 		StopBits:    serial.Stop1,
// 		ReadTimeout: 100 * time.Millisecond,
// 	}
// 	return serial.OpenPort(config)
// }

// doorbell.go
package main

import (
	"bytes" // 바이트 슬라이스 비교를 위해 추가
	"fmt"
	"log" // log 대신 log 패키지 사용
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// DoorbellController 도어벨 컨트롤러 구조체
type DoorbellController struct {
	portName   string // 시리얼 포트 이름 (생성 시 저장)
	mqttBroker string
	mqttPrefix string
	serialPort *serial.Port
	mqttClient mqtt.Client
	readBuffer []byte // Read 함수 버퍼 재사용

	// 벨 울림/문열기 관련 패킷 (구조체 내부로 이동)
	bellRingPacket1   []byte
	bellRingPacket2   []byte
	doorOpenCmdPacket []byte

	// 패킷 버퍼링을 위한 필드
	packetBuffer []byte
	lastReadTime time.Time
}

// NewDoorbellController 생성자
func NewDoorbellController(portName, mqttBroker, mqttPrefix string) *DoorbellController {
	dc := &DoorbellController{
		portName:   portName, // 포트 이름 저장
		mqttBroker: mqttBroker,
		mqttPrefix: mqttPrefix,
		readBuffer: make([]byte, 256), // 버퍼 초기화

		bellRingPacket1:   []byte{0x02, 0x10, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x10, 0x00, 0x00, 0x00, 0x40, 0x03},
		bellRingPacket2:   []byte{0x02, 0x10, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x10, 0x00, 0x00, 0x00, 0x5A, 0x03},
		doorOpenCmdPacket: []byte{0x02, 0x11, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x05, 0x40, 0x00, 0x01, 0x77, 0x03},

		packetBuffer: make([]byte, 0, 256), // 패킷 버퍼 초기화
		lastReadTime: time.Now(),
	}

	// 시리얼 포트 연결
	if err := dc.connectSerial(); err != nil {
		log.Printf("[DOORBELL] 시리얼 포트 초기화 실패: %v\n", err)
		return nil // 초기화 실패 시 nil 반환
	}

	// MQTT 클라이언트 연결
	if err := dc.connectMQTT(); err != nil {
		log.Printf("[DOORBELL] MQTT 초기화 실패: %v\n", err)
		return nil // 초기화 실패 시 nil 반환
	}

	return dc
}

// connectSerial 시리얼 포트 연결 및 재연결 로직
func (dc *DoorbellController) connectSerial() error {
	var err error

	// 무한 재연결 시도
	for {
		log.Printf("[DOORBELL] 시리얼 포트 %s 연결 시도 중...", dc.portName)
		dc.serialPort, err = openSerialDoorbell(dc.portName)
		if err == nil {
			log.Println("[DOORBELL] 시리얼 포트 연결 성공!")

			// 연결 상태 모니터링 시작
			go dc.monitorSerialConnection()
			return nil
		}
		log.Printf("[DOORBELL] 시리얼 포트 연결 실패: %v. 3초 후 재시도...\n", err)
		time.Sleep(3 * time.Second)
	}
}

// reconnectSerial 시리얼 포트 재연결
func (dc *DoorbellController) reconnectSerial() error {
	if dc.serialPort != nil {
		dc.serialPort.Close()
		dc.serialPort = nil // 닫은 후 nil로 설정
	}

	// 무한 재연결 시도
	for {
		sp, err := openSerialDoorbell(dc.portName)
		if err == nil {
			dc.serialPort = sp
			log.Println("[DOORBELL] 시리얼 포트 재연결 성공!")
			return nil
		}
		log.Printf("[DOORBELL] 시리얼 포트 재연결 실패: %v. 3초 후 재시도...\n", err)
		time.Sleep(3 * time.Second)
	}
}

// monitorSerialConnection 시리얼 포트 연결 상태를 지속적으로 모니터링합니다.
func (dc *DoorbellController) monitorSerialConnection() {
	ticker := time.NewTicker(60 * time.Second) // 1분마다 체크
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if dc.serialPort == nil {
				log.Println("[DOORBELL] 시리얼 포트가 nil입니다. 재연결 시도...")
				if err := dc.reconnectSerial(); err != nil {
					log.Printf("[DOORBELL] 시리얼 포트 재연결 실패: %v\n", err)
				}
			} else {
				// 도어벨은 이벤트 기반이므로 연결 테스트를 간단하게만 수행
				// 실제 데이터 전송 없이 포트 상태만 확인
				log.Println("[DOORBELL] 시리얼 포트 연결 상태 정상 (이벤트 대기 중)")
			}
		}
	}
}

// connectMQTT MQTT 클라이언트 연결
func (dc *DoorbellController) connectMQTT() error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] MQTT 연결 중 패닉 복구: %v\n", r)
		}
	}()

	// 무한 재연결 시도
	for {
		log.Printf("[DOORBELL] MQTT 브로커 %s 연결 시도 중...", dc.mqttBroker)

		opts := mqtt.NewClientOptions().
			AddBroker(dc.mqttBroker).
			SetClientID("usbrs485-doorbell").
			SetCleanSession(false).                            // 세션 유지로 재연결 시 구독 복원
			SetAutoReconnect(true).                            // 자동 재연결 활성화
			SetConnectRetry(true).                             // 연결 재시도 활성화
			SetConnectRetryInterval(10 * time.Second).         // 재연결 간격 증가
			SetMaxReconnectInterval(2 * time.Minute).          // 최대 재연결 간격 증가
			SetConnectionLostHandler(dc.onMQTTConnectionLost). // 연결 끊김 핸들러 추가
			SetOnConnectHandler(dc.onMQTTConnect).             // 연결 성공/복구 핸들러 추가
			SetOrderMatters(false).                            // 메시지 순서 무시로 성능 향상
			SetResumeSubs(true).                               // 재연결 시 구독 자동 복원
			SetKeepAlive(60 * time.Second).                    // Keep-Alive 간격 증가
			SetPingTimeout(20 * time.Second).                  // Ping 타임아웃 증가
			SetConnectTimeout(60 * time.Second)                // 연결 타임아웃 증가

		dc.mqttClient = mqtt.NewClient(opts)

		// 연결 시도 전에 잠시 대기
		time.Sleep(1 * time.Second)

		if token := dc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("[DOORBELL] MQTT 연결 실패: %v. 15초 후 재시도...\n", token.Error())
			time.Sleep(15 * time.Second)
			continue
		}

		log.Println("[DOORBELL] MQTT 연결 성공!")

		// 연결 상태 모니터링 시작
		go dc.monitorMQTTConnection()
		return nil
	}
}

// onMQTTConnectionLost MQTT 연결 끊김 핸들러
func (dc *DoorbellController) onMQTTConnectionLost(client mqtt.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] MQTT 연결 끊김 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Printf("[DOORBELL] MQTT 연결 끊김: %v. 자동 재연결 시도 중...\n", err)

	// 연결 상태 모니터링 시작
	go dc.monitorMQTTConnection()
}

// onMQTTConnect MQTT 연결 성공/복구 핸들러
func (dc *DoorbellController) onMQTTConnect(client mqtt.Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] MQTT 연결 성공 핸들러 패닉 복구: %v\n", r)
		}
	}()

	log.Println("[DOORBELL] MQTT 연결 복구 또는 초기 연결 완료. 구독 설정 중...")

	// 연결 상태 확인
	if client.IsConnected() {
		log.Println("[DOORBELL] MQTT 클라이언트 연결 상태 확인됨")
		dc.SubscribeMqtt() // 연결 복구 시 구독 재설정
	} else {
		log.Println("[DOORBELL] MQTT 클라이언트 연결 상태 확인 실패")
	}
}

// monitorMQTTConnection MQTT 연결 상태를 지속적으로 모니터링합니다.
func (dc *DoorbellController) monitorMQTTConnection() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] MQTT 모니터링 패닉 복구: %v\n", r)
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// MQTT 클라이언트 nil 체크
			if dc.mqttClient == nil {
				log.Println("[DOORBELL] MQTT 클라이언트가 nil입니다. 모니터링을 중단합니다.")
				return
			}

			// 연결 상태 체크를 안전하게 수행
			if !dc.mqttClient.IsConnected() {
				log.Println("[DOORBELL] MQTT 연결이 끊어졌습니다. 재연결 시도 중...")

				// 재연결 시도 전에 잠시 대기
				time.Sleep(2 * time.Second)

				// 수동 재연결 시도
				if token := dc.mqttClient.Connect(); token.Wait() && token.Error() != nil {
					log.Printf("[DOORBELL] MQTT 재연결 실패: %v\n", token.Error())
				} else {
					log.Println("[DOORBELL] MQTT 재연결 성공!")
					// 재연결 후 구독 재설정
					time.Sleep(1 * time.Second)
					dc.SubscribeMqtt()
				}
			} else {
				log.Println("[DOORBELL] MQTT 연결 상태 정상")
			}
		}
	}
}

// SubscribeMqtt MQTT 버튼(문열기) 구독
func (dc *DoorbellController) SubscribeMqtt() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] MQTT 구독 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if dc.mqttClient == nil {
		log.Println("[DOORBELL] MQTT 클라이언트가 nil입니다. 구독을 건너뜁니다.")
		return
	}

	topic := fmt.Sprintf("%s/doorbell/open/set", dc.mqttPrefix)
	// 구독은 OnConnect 핸들러에서 이루어지므로, 이미 구독되어 있다면 재구독하지 않도록 설정
	// Paho MQTT는 기본적으로 CleanSession=false일 때 재연결 시 구독을 복원하려고 시도합니다.
	// 따라서 이 함수는 onMQTTConnect에서만 호출되어야 중복 구독이 발생하지 않습니다.
	if token := dc.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		cmd := string(msg.Payload())
		log.Printf("[DOORBELL] 문열기 명령 수신: topic=%s, payload=%s\n", topic, cmd)
		if cmd == "ON" {
			dc.SendOpenDoorPacket()
		}
	}); token.Wait() && token.Error() != nil {
		log.Printf("[DOORBELL] 문열기 명령 구독 실패: %v\n", token.Error())
	} else {
		log.Printf("[DOORBELL] 문열기 명령 구독 성공: %s\n", topic)
	}
}

// SendOpenDoorPacket 문열기 패킷 송신
func (dc *DoorbellController) SendOpenDoorPacket() {
	log.Printf("[DOORBELL] 문열기 패킷 전송 시도: % X\n", dc.doorOpenCmdPacket) // % X로 16진수 출력

	if dc.serialPort == nil {
		log.Println("[DOORBELL] 시리얼 포트가 연결되어 있지 않습니다. 재연결 시도...")
		if err := dc.reconnectSerial(); err != nil {
			log.Printf("[DOORBELL] 시리얼 포트 재연결 실패: %v. 문열기 명령 실패.\n", err)
			return
		}
	}

	_, err := dc.serialPort.Write(dc.doorOpenCmdPacket)
	if err != nil {
		log.Printf("[DOORBELL] 시리얼 포트 쓰기 에러: %v. 재연결 시도.\n", err)
		if reconnectErr := dc.reconnectSerial(); reconnectErr != nil {
			log.Printf("[DOORBELL] 시리얼 포트 재연결도 실패: %v. 문열기 명령 실패.\n", reconnectErr)
			return
		}
	}
	log.Printf("[DOORBELL] 문열기 패킷 전송 완료: % X\n", dc.doorOpenCmdPacket)
}

// PublishEvent 도어벨 이벤트를 MQTT에 발행합니다.
func (dc *DoorbellController) PublishEvent(eventType string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DOORBELL] 이벤트 발행 패닉 복구: %v\n", r)
		}
	}()

	// MQTT 클라이언트 nil 체크
	if dc.mqttClient == nil {
		log.Println("[DOORBELL] MQTT 클라이언트가 nil입니다. 이벤트 발행을 건너뜁니다.")
		return
	}

	// MQTT Event는 JSON 형식으로 발행
	topic := fmt.Sprintf("%s/doorbell/event", dc.mqttPrefix)
	payload := fmt.Sprintf(`{"event_type":"%s"}`, eventType)

	token := dc.mqttClient.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Printf("[DOORBELL] 이벤트 publish 실패: %s → %s, 에러: %v\n", topic, payload, token.Error())
	} else {
		log.Printf("[DOORBELL] 이벤트 publish: %s → %s\n", topic, payload)
	}
}

// Run RS485 감시 루프를 시작하고 MQTT 구독을 설정합니다.
func (dc *DoorbellController) Run() {
	// 초기 구독은 onMQTTConnect 핸들러에서 자동으로 설정됩니다.
	// dc.SubscribeMqtt() // 이 줄을 제거

	// 시리얼 포트가 nil인지 확인
	if dc.serialPort == nil {
		log.Println("[DOORBELL] 시리얼 포트가 nil입니다. 도어벨 컨트롤러 실행을 중단합니다.")
		return
	}

	// MQTT 구독은 connectMQTT 내부의 onMQTTConnect에서 처리됩니다.
	// 초기 연결 시 구독이 되므로 여기서는 별도로 호출할 필요 없습니다.

	log.Println("[DOORBELL] 도어벨 컨트롤러 시작 - 이벤트 대기 중...")

	for {
		n, err := dc.serialPort.Read(dc.readBuffer) // 버퍼 재사용
		if err != nil {
			// EOF는 정상적인 대기 상태 (도어벨이 울리지 않음)
			if err.Error() == "EOF" {
				// 1초 대기 후 다시 읽기 시도
				time.Sleep(1 * time.Second)
				continue
			}

			// 다른 에러의 경우 재연결 시도
			log.Printf("[DOORBELL] 시리얼 포트 읽기 에러: %v. 재연결 시도.\n", err)
			dc.reconnectSerial() // 무한 재연결 시도
			continue             // 다음 루프에서 재연결된 포트로 다시 시도
		}

		if n > 0 { // 데이터가 수신된 경우만 처리
			currentTime := time.Now()

			// 마지막 읽기로부터 100ms 이상 지났으면 버퍼 초기화 (새로운 패킷 시작)
			if currentTime.Sub(dc.lastReadTime) > 100*time.Millisecond {
				dc.packetBuffer = dc.packetBuffer[:0]
			}
			dc.lastReadTime = currentTime

			// 수신된 데이터를 패킷 버퍼에 추가
			dc.packetBuffer = append(dc.packetBuffer, dc.readBuffer[:n]...)

			log.Printf("[DOORBELL] 데이터 수신 (길이: %d, 누적: %d): % X\n", n, len(dc.packetBuffer), dc.packetBuffer)

			// 패킷이 완전히 수신되었는지 확인 (패킷은 0x02로 시작, 0x03으로 끝남)
			if len(dc.packetBuffer) > 0 && dc.packetBuffer[0] == 0x02 && dc.packetBuffer[len(dc.packetBuffer)-1] == 0x03 {
				// 완전한 패킷 수신됨
				log.Printf("[DOORBELL] 완전한 패킷 수신 (길이: %d): % X\n", len(dc.packetBuffer), dc.packetBuffer)

				// 벨 울림 패킷 감지 - 두 가지 패킷 모두 확인
				if bytes.Equal(dc.packetBuffer, dc.bellRingPacket1) || bytes.Equal(dc.packetBuffer, dc.bellRingPacket2) {
					log.Println("[DOORBELL] 🔔 벨 울림 감지!")
					dc.PublishEvent("ring")
				} else {
					// 기타 패킷 (디버깅용)
					log.Printf("[DOORBELL] 알 수 없는 패킷: % X\n", dc.packetBuffer)
				}

				// 패킷 처리 후 버퍼 초기화
				dc.packetBuffer = dc.packetBuffer[:0]
			} else if len(dc.packetBuffer) > 256 {
				// 버퍼가 너무 크면 초기화 (잘못된 데이터)
				log.Printf("[DOORBELL] 버퍼 오버플로우, 초기화: % X\n", dc.packetBuffer)
				dc.packetBuffer = dc.packetBuffer[:0]
			}
		}
	}
}

// openSerialDoorbell 도어벨용 시리얼 포트를 엽니다.
func openSerialDoorbell(portName string) (*serial.Port, error) {
	// 시뮬레이션 모드 체크
	if len(portName) >= 4 && portName[:4] == "SIM_" {
		log.Printf("[DOORBELL] 시뮬레이션 모드: %s 포트 사용\n", portName)
		return &serial.Port{}, nil // 가상 포트 반환
	}

	config := &serial.Config{
		Name:        portName,
		Baud:        9600,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
		ReadTimeout: 1 * time.Second, // 타임아웃을 1초로 늘림
	}
	return serial.OpenPort(config)
}

// RunDoorbellController는 main.go에서 호출될 외부 함수
func RunDoorbellController(port, mqttBroker, mqttPrefix string) {
	dc := NewDoorbellController(port, mqttBroker, mqttPrefix)
	if dc == nil {
		log.Fatalf("[DOORBELL] 컨트롤러 초기화 실패로 프로그램 종료.") // 치명적 에러 시 프로그램 종료
	}
	dc.Run()
}
