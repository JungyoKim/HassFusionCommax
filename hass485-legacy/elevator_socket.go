package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tarm/serial"
)

// ElevatorController: 엘리베이터 호출 관리 구조체
type ElevatorController struct {
	port         string
	socketClient *SocketClient
	prefix       string
	serialPort   *serial.Port
	state        string
	callChan     chan bool
	writeMu      sync.Mutex
	alloff       *AllOffSwitchController
}

// 엘리베이터 호출 패킷 (8바이트 고정, 새 값)
var elevatorCallPacket = []byte{0xA0, 0x01, 0x01, 0x00, 0x08, 0x15, 0x00, 0xBF}

// 엘리베이터 컨트롤러 생성자 (시리얼, 소켓 초기화)
func NewElevatorController(port string, socketPath string, prefix string) *ElevatorController {
	ec := &ElevatorController{
		port:     port,
		prefix:   prefix,
		state:    "idle",
		callChan: make(chan bool),
	}

	// 시리얼 포트 초기화 (재연결 루프)
	var sp *serial.Port
	var err error
	for {
		sp, err = openSerialElevator(port)
		if err != nil {
			fmt.Println("[엘리베이터] 시리얼 포트 연결 실패, 3초 후 재시도:", err)
			time.Sleep(3 * time.Second)
			continue
		}
		fmt.Println("[엘리베이터] 시리얼 포트 연결 성공!")
		break
	}
	ec.serialPort = sp

	// 소켓 클라이언트 초기화
	ec.socketClient = NewSocketClient(socketPath, prefix)
	if err := ec.socketClient.Connect(); err != nil {
		fmt.Println("[엘리베이터] 소켓 연결 실패:", err)
		return ec
	}
	fmt.Println("[엘리베이터] 소켓 연결 성공!")

	// 일괄소등 컨트롤러 생성 및 할당
	ec.alloff = NewAllOffSwitchController(ec.serialPort, ec.socketClient, prefix, &ec.writeMu)

	// 명령 처리 고루틴 시작
	go ec.handleCommands()

	return ec
}

// 엘리베이터 호출 패킷 송신 (실제 RS485 송신)
func (ec *ElevatorController) SendCallPacket() {
	// 일괄소등 상태조회 일시정지
	if ec.alloff != nil {
		select {
		case ec.alloff.pauseChan <- true:
			fmt.Println("[엘리베이터] 일괄소등 상태조회 일시정지 요청!")
		default:
		}
	}

	ec.writeMu.Lock()
	_, err := ec.serialPort.Write(elevatorCallPacket)
	ec.writeMu.Unlock()
	if err != nil {
		fmt.Println("[엘리베이터] 시리얼 포트 에러 발생, 재연결 시도:", err)
		ec.serialPort.Close()
		for {
			ec.serialPort, err = openSerialElevator(ec.port)
			if err != nil {
				fmt.Println("[엘리베이터] 시리얼 포트 재연결 실패, 3초 후 재시도:", err)
				time.Sleep(3 * time.Second)
				continue
			}
			fmt.Println("[엘리베이터] 시리얼 포트 재연결 성공!")
			break
		}
		return
	}
	fmt.Printf("[엘리베이터] 호출 패킷 전송: % X (err: %v)\n", elevatorCallPacket, err)

	// 2초 후 일괄소등 상태조회 재개
	if ec.alloff != nil {
		go func() {
			time.Sleep(2 * time.Second)
			select {
			case ec.alloff.resumeChan <- true:
				fmt.Println("[엘리베이터] 일괄소등 상태조회 재개 요청!")
			default:
			}
		}()
	}
}

// 소켓 명령 구독
func (ec *ElevatorController) SubscribeSocket() {
	fmt.Println("[엘리베이터] SubscribeSocket() 진입")
	path := fmt.Sprintf("/elevator/call/set")

	ec.socketClient.Subscribe(path, func(msg SocketMessage) {
		fmt.Println("[소켓] 콜백 함수 진입")
		cmd := fmt.Sprintf("%v", msg.Value)
		fmt.Printf("[소켓] 호출 명령 수신: path=%s, payload=%s\n", path, cmd)
		if cmd == "ON" || cmd == "1" {
			fmt.Println("[소켓] callChan <- true (호출 명령 이벤트)")
			ec.callChan <- true
		}
	})
	fmt.Println("[소켓] 엘리베이터 호출 명령 구독:", path)
}

// 상태 소켓 publish
func (ec *ElevatorController) PublishState() {
	path := fmt.Sprintf("/elevator/state")
	ec.socketClient.Publish(path, ec.state)
	fmt.Printf("[소켓] %s → %s\n", path, ec.state)
}

// 엘리베이터 컨트롤러 메인 루프
func (ec *ElevatorController) Run() {
	ec.SubscribeSocket()
	fmt.Println("[엘리베이터] Run() 시작, 이벤트 루프 진입")
	for {
		select {
		case <-ec.callChan:
			fmt.Println("[엘리베이터] callChan 이벤트 감지 → SendCallPacket() 실행")
			ec.SendCallPacket()
			// 필요시 호출 직후 상태 publish: (예시)
			// ec.state = "called"
			// ec.PublishState()
		}
	}
}

// 보일러/조명과 동일한 스타일의 실행 함수 추가
func RunElevatorController(port, socketPath, prefix string) {
	ec := NewElevatorController(port, socketPath, prefix)
	go ec.alloff.Run()
	ec.Run()
}

// 일괄소등 스위치 컨트롤러 구조체
type AllOffSwitchController struct {
	serialPort   *serial.Port
	socketClient *SocketClient
	prefix       string
	state        string // "ON" or "OFF"
	writeMu      *sync.Mutex
	pauseChan    chan bool
	resumeChan   chan bool
}

// 일괄소등 관련 패킷
var (
	alloffStatusReqPacket = []byte{0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21}
	alloffOnCmdPacket     = []byte{0x22, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x25}
	alloffOffCmdPacket    = []byte{0x22, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x24}
)

// 상태 패킷 파싱 (8바이트)
func parseAlloffStatusPacket(pkt []byte) (string, bool) {
	// 긴 패킷에서 8바이트 단위로 검색
	for i := 0; i <= len(pkt)-8; i++ {
		chunk := pkt[i : i+8]
		if chunk[0] == 0xA0 {
			if chunk[1] == 0x01 && chunk[2] == 0x01 {
				return "ON", true
			}
			if chunk[1] == 0x00 && chunk[2] == 0x01 {
				return "OFF", true
			}
		}
	}
	return "", false
}

// 컨트롤러 생성자 (포트/클라이언트/프리픽스 공유)
func NewAllOffSwitchController(serialPort *serial.Port, socketClient *SocketClient, prefix string, writeMu *sync.Mutex) *AllOffSwitchController {
	return &AllOffSwitchController{
		serialPort:   serialPort,
		socketClient: socketClient,
		prefix:       prefix,
		state:        "OFF",
		writeMu:      writeMu,
		pauseChan:    make(chan bool, 1),
		resumeChan:   make(chan bool, 1),
	}
}

// 소켓 명령 구독
func (ac *AllOffSwitchController) SubscribeSocket() {
	path := fmt.Sprintf("/alloff/set")
	ac.socketClient.Subscribe(path, func(msg SocketMessage) {
		cmd := fmt.Sprintf("%v", msg.Value)
		fmt.Printf("[소켓][일괄소등] 명령 수신: path=%s, payload=%s\n", path, cmd)
		if cmd == "ON" {
			ac.SendCommand(true)
		} else if cmd == "OFF" {
			ac.SendCommand(false)
		}
	})
	fmt.Println("[소켓][일괄소등] 명령 구독:", path)
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
	fmt.Printf("[일괄소등] 제어 패킷 전송: % X (bytes: %d, err: %v)\n", pkt, n, err)
}

// 상태 소켓 publish
func (ac *AllOffSwitchController) PublishState() {
	path := fmt.Sprintf("/alloff/state")
	ac.socketClient.Publish(path, ac.state)
	fmt.Printf("[소켓][일괄소등] 상태 publish: %s → %s\n", path, ac.state)
}

// 상태 주기적 조회 및 패킷 수신 루프
func (ac *AllOffSwitchController) Run() {
	ac.SubscribeSocket()
	buf := make([]byte, 128)
	paused := false
	for {
		select {
		case <-ac.pauseChan:
			paused = true
			fmt.Println("[일괄소등] 상태조회 일시정지!")
			<-ac.resumeChan
			paused = false
			fmt.Println("[일괄소등] 상태조회 재개!")
		default:
			if paused {
				time.Sleep(75 * time.Millisecond)
				continue
			}
			// 상태 요청 패킷 송신
			ac.writeMu.Lock()
			ac.serialPort.Write(alloffStatusReqPacket)
			ac.writeMu.Unlock()
			fmt.Printf("[일괄소등] 상태 요청 패킷 전송: % X\n", alloffStatusReqPacket)

			time.Sleep(100 * time.Millisecond)
			n, err := ac.serialPort.Read(buf)
			if err != nil {
				fmt.Printf("[일괄소등] 상태 응답 읽기 에러: %v\n", err)
			} else if n > 0 {
				fmt.Printf("[일괄소등] 상태 응답 수신 (길이: %d): % X\n", n, buf[:n])
				if n >= 8 {
					state, ok := parseAlloffStatusPacket(buf[:n])
					if ok {
						fmt.Printf("[일괄소등] 상태 파싱 성공: %s\n", state)
						if state != ac.state {
							fmt.Printf("[일괄소등] 상태 변경: %s → %s\n", ac.state, state)
							ac.state = state
							ac.PublishState()
						} else {
							fmt.Printf("[일괄소등] 상태 변경 없음 (현재: %s)\n", ac.state)
						}
					} else {
						fmt.Printf("[일괄소등] 상태 패킷 파싱 실패: % X\n", buf[:n])
					}
				} else {
					fmt.Printf("[일괄소등] 불완전한 상태 응답 (길이 %d): % X\n", n, buf[:n])
				}
			} else {
				fmt.Printf("[일괄소등] 상태 응답 없음 (n=%d)\n", n)
			}
			time.Sleep(75 * time.Millisecond)
		}
	}
}

func openSerialElevator(portName string) (*serial.Port, error) {
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

// handleCommands 명령 처리 고루틴
func (ec *ElevatorController) handleCommands() {
	for {
		select {
		case cmd := <-elevatorCommands:
			fmt.Printf("[엘리베이터] 명령 수신: %+v\n", cmd)
			if cmd.Action == "call" {
				fmt.Printf("[엘리베이터] 호출 명령 실행\n")
				ec.SendCallPacket()
			}
		case cmd := <-allOffCommands:
			fmt.Printf("[엘리베이터] 일괄소등 명령 수신: %+v\n", cmd)
			if cmd.Action == "set" {
				fmt.Printf("[엘리베이터] 일괄소등 명령 실행: %s\n", cmd.Value)
				on := cmd.Value == "ON"
				ec.alloff.SendCommand(on)
			}
		}
	}
}
