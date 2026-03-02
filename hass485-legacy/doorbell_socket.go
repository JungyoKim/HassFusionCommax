package main

import (
	"fmt"
	"log"
	"time"

	"github.com/tarm/serial"
)

// DoorbellController 구조체 (벨 울림 감지 + 문열기 명령)
type DoorbellController struct {
	portName     string
	socketClient *SocketClient
	prefix       string
	serialPort   *serial.Port
	state        string // "ON"(벨 울림) or "OFF"(대기)
	readBuffer   []byte

	// 벨 울림 감지 패킷
	bellRingPacket []byte
	// 문열기 명령 패킷
	doorOpenCmdPacket []byte
}

// NewDoorbellController 생성자
func NewDoorbellController(portName, socketPath, prefix string) *DoorbellController {
	dc := &DoorbellController{
		portName:   portName,
		prefix:     prefix,
		state:      "OFF",
		readBuffer: make([]byte, 256),

		bellRingPacket:    []byte{0x10, 0x01, 0x09, 0x12, 0x01, 0x01, 0x09, 0x12, 0x01, 0x10, 0x00, 0x00, 0x00, 0x5A, 0x03},
		doorOpenCmdPacket: []byte{0x02, 0x11, 0x02, 0x02, 0x09, 0x03, 0x02, 0x02, 0x09, 0x03, 0x05, 0x40, 0x00, 0x01, 0x77, 0x03},
	}

	// 시리얼 포트 연결
	if err := dc.connectSerial(); err != nil {
		log.Printf("[도어벨] 시리얼 포트 초기화 실패: %v\n", err)
		return nil
	}

	// 소켓 클라이언트 연결
	dc.socketClient = NewSocketClient(socketPath, prefix)
	if err := dc.socketClient.Connect(); err != nil {
		log.Printf("[도어벨] 소켓 초기화 실패: %v\n", err)
		return nil
	}

	// 명령 처리 고루틴 시작
	go dc.handleCommands()

	return dc
}

// connectSerial 시리얼 포트 연결 및 재연결 로직
func (dc *DoorbellController) connectSerial() error {
	var err error
	for {
		log.Printf("[도어벨] 시리얼 포트 %s 연결 시도 중...\n", dc.portName)
		dc.serialPort, err = openSerialDoorbell(dc.portName)
		if err != nil {
			log.Printf("[도어벨] 시리얼 포트 연결 실패: %v. 3초 후 재시도.\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("[도어벨] 시리얼 포트 연결 성공!")
		return nil
	}
}

// reconnectSerial 시리얼 포트 재연결
func (dc *DoorbellController) reconnectSerial() error {
	if dc.serialPort != nil {
		dc.serialPort.Close()
		dc.serialPort = nil
	}
	return dc.connectSerial()
}

// SubscribeSocket 소켓 명령 구독 (문열기 명령)
func (dc *DoorbellController) SubscribeSocket() {
	path := fmt.Sprintf("/door/open/set")

	dc.socketClient.Subscribe(path, func(msg SocketMessage) {
		cmd := fmt.Sprintf("%v", msg.Value)
		log.Printf("[소켓][도어벨] 문열기 명령 수신: path=%s, payload=%s\n", path, cmd)
		if cmd == "ON" {
			dc.SendOpenDoorPacket()
		}
	})
	log.Printf("[소켓][도어벨] 문열기 명령 구독 성공: %s\n", path)
}

// SendOpenDoorPacket 문열기 패킷 송신
func (dc *DoorbellController) SendOpenDoorPacket() {
	log.Printf("[도어벨] 문열기 패킷 전송 시도: % X\n", dc.doorOpenCmdPacket)

	if dc.serialPort == nil {
		log.Println("[도어벨] 시리얼 포트가 연결되어 있지 않습니다. 재연결 시도...")
		if err := dc.reconnectSerial(); err != nil {
			log.Printf("[도어벨] 시리얼 포트 재연결 실패: %v. 문열기 명령 실패.\n", err)
			return
		}
	}

	_, err := dc.serialPort.Write(dc.doorOpenCmdPacket)
	if err != nil {
		log.Printf("[도어벨] 시리얼 포트 쓰기 에러: %v. 재연결 시도.\n", err)
		if reconnectErr := dc.reconnectSerial(); reconnectErr != nil {
			log.Printf("[도어벨] 시리얼 포트 재연결도 실패: %v. 문열기 명령 실패.\n", reconnectErr)
			return
		}
	}
	log.Printf("[도어벨] 문열기 패킷 전송 완료: % X\n", dc.doorOpenCmdPacket)
}

// PublishState 벨 울림 상태를 소켓에 발행합니다.
func (dc *DoorbellController) PublishState(state string) {
	path := fmt.Sprintf("/doorbell/state")
	dc.socketClient.Publish(path, state)
	log.Printf("[소켓][도어벨] 상태 publish: %s → %s\n", path, state)
}

// Run RS485 감시 루프를 시작합니다.
func (dc *DoorbellController) Run() {
	dc.SubscribeSocket()

	// 상태 모니터링 고루틴 시작
	go dc.monitorDoorbell()

	// 메인 루프는 대기
	select {}
}

// monitorDoorbell 벨 울림 상태를 모니터링합니다.
func (dc *DoorbellController) monitorDoorbell() {
	reconnectCount := 0
	maxReconnectAttempts := 5

	for {
		// 시리얼 포트가 연결되어 있는지 확인
		if dc.serialPort == nil {
			log.Printf("[도어벨] 시리얼 포트가 연결되지 않음. 재연결 시도 %d/%d...\n", reconnectCount+1, maxReconnectAttempts)
			if err := dc.reconnectSerial(); err != nil {
				reconnectCount++
				if reconnectCount >= maxReconnectAttempts {
					log.Printf("[도어벨] 최대 재연결 시도 횟수 초과. 도어벨 모니터링 중단.\n")
					return
				}
				time.Sleep(3 * time.Second)
				continue
			}
			reconnectCount = 0
			log.Printf("[도어벨] 시리얼 포트 재연결 성공. 모니터링 재개.\n")
		}

		// 도어벨은 이벤트 기반이므로 타임아웃 기반 읽기
		n, err := dc.serialPort.Read(dc.readBuffer)
		if err != nil {
			// EOF는 도어벨의 정상적인 상태 (데이터가 없음)
			if err.Error() == "EOF" {
				// 도어벨은 평상시에 데이터가 없으므로 정상
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Printf("[도어벨] 시리얼 포트 읽기 에러: %v. 재연결 시도.\n", err)
			if err := dc.reconnectSerial(); err != nil {
				reconnectCount++
				if reconnectCount >= maxReconnectAttempts {
					log.Printf("[도어벨] 최대 재연결 시도 횟수 초과. 도어벨 모니터링 중단.\n")
					return
				}
				time.Sleep(3 * time.Second)
				continue
			}
			reconnectCount = 0
			continue
		}

		if n > 0 {
			// 데이터 수신 시 처리
			data := dc.readBuffer[:n]
			log.Printf("[도어벨] RS485 데이터 수신 (길이: %d): % X\n", n, data)

			// 벨 울림 패킷 패턴 확인
			if dc.isBellRingPacket(data) {
				log.Printf("[도어벨] 🔔 벨 울림 감지! 패킷 매칭됨")

				// 벨이 울렸을 때 상태 변경
				dc.state = "ON"
				dc.PublishState("ON")

				// 잠시 후 상태 복원 (벨이 멈췄을 것으로 가정)
				time.Sleep(2 * time.Second)
				dc.state = "OFF"
				dc.PublishState("OFF")
			} else {
				log.Printf("[도어벨] 벨 울림이 아닌 데이터 수신 (무시)")
			}
		}
	}
}

// handleCommands 명령 처리 고루틴
func (dc *DoorbellController) handleCommands() {
	for {
		select {
		case cmd := <-doorbellCommands:
			log.Printf("[도어벨] 도어벨 명령 수신: %+v", cmd)
			if cmd.Action == "open" {
				log.Printf("[도어벨] 문열기 명령 실행: %s", cmd.Value)
				dc.SendOpenDoorPacket()
			}
		}
	}
}

// isBellRingPacket 벨 울림 패킷인지 확인
func (dc *DoorbellController) isBellRingPacket(data []byte) bool {
	// 정의된 벨 패킷과 정확히 일치하는지 확인
	if len(data) == len(dc.bellRingPacket) {
		for i, b := range data {
			if b != dc.bellRingPacket[i] {
				return false
			}
		}
		return true
	}
	return false
}

// openSerialDoorbell 시리얼 포트 설정 및 열기
func openSerialDoorbell(portName string) (*serial.Port, error) {
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

// RunDoorbellController는 main.go에서 호출될 외부 함수
func RunDoorbellController(port, socketPath, prefix string) {
	dc := NewDoorbellController(port, socketPath, prefix)
	if dc == nil {
		log.Printf("[도어벨] 컨트롤러 초기화 실패")
		return
	}
	dc.Run()
}
