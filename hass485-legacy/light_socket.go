package main

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

type LightController struct {
	port              *serial.Port
	socketClient      *SocketClient
	prefix            string
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

func NewLightController(portName string, socketPath string, prefix string) *LightController {
	lc := &LightController{
		prefix:            prefix,
		lightStatus:       make([]int, 5),
		prevStatus:        []int{0, 0, 0, 0, 0}, // -1에서 0으로 변경
		pauseStatusQuery:  make(chan bool),
		resumeStatusQuery: make(chan bool),
		readBuffer:        make([]byte, 128),

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

	// 소켓 클라이언트 연결
	lc.socketClient = NewSocketClient(socketPath, prefix)
	if err := lc.socketClient.Connect(); err != nil {
		return nil
	}

	return lc
}

func (lc *LightController) connectSerial(portName string) error {
	var err error
	for {
		lc.port, err = openSerialLight(portName)
		if err != nil {
			fmt.Println("[조명] 시리얼 포트 연결 실패, 3초 후 재시도:", err)
			time.Sleep(3 * time.Second)
			continue
		}
		fmt.Println("[조명] 시리얼 포트 연결 성공!")
		return nil
	}
}

func (lc *LightController) reconnectSerial(portName string) error {
	lc.port.Close()
	return lc.connectSerial(portName)
}

func (lc *LightController) subscribeToCommands() {
	for i := 1; i <= 5; i++ {
		path := fmt.Sprintf("/lights/%d/set", i)
		idx := i - 1

		fmt.Printf("[조명] 소켓 구독 시도: %s\n", path)

		lc.socketClient.Subscribe(path, func(msg SocketMessage) {
			cmd := strings.ToUpper(fmt.Sprintf("%v", msg.Value))
			fmt.Printf("[조명] 소켓 명령 수신 - 경로: %s, 명령: %s, 조명%d 현재상태: %d\n", msg.Path, cmd, idx+1, lc.lightStatus[idx])

			var pkt string

			lc.statusMu.Lock()
			cur := lc.lightStatus[idx]
			lc.statusMu.Unlock()

			if cmd == "ON" && cur == 0 {
				pkt = lc.onPackets[idx]
				fmt.Printf("[조명] 조명%d ON 명령 전송: %s\n", idx+1, pkt)
			} else if cmd == "OFF" && cur == 1 {
				pkt = lc.offPackets[idx]
				fmt.Printf("[조명] 조명%d OFF 명령 전송: %s\n", idx+1, pkt)
			} else {
				fmt.Printf("[조명] 조명%d 명령 무시 - 현재상태: %d, 요청명령: %s\n", idx+1, cur, cmd)
				return
			}

			// 상태 조회 일시 중단
			lc.pauseStatusQuery <- true

			lc.writeMu.Lock()
			_, err := lc.port.Write(hexStringToBytes(pkt))
			lc.writeMu.Unlock()

			if err != nil {
				fmt.Printf("[조명] 시리얼 포트 쓰기 에러: %v\n", err)
			}

			time.Sleep(75 * time.Millisecond)

			// 상태 조회 재개
			lc.resumeStatusQuery <- true
		})
	}
}

func (lc *LightController) publishStateIfChanged(idx int, newStatus int) {
	lc.statusMu.Lock()
	defer lc.statusMu.Unlock()

	// 상태가 변경된 경우에만 소켓 발행
	if lc.prevStatus[idx] != newStatus {
		path := fmt.Sprintf("/lights/%d/state", idx+1)
		statusStr := onOff(newStatus)
		fmt.Printf("[소켓] 조명 %d 상태 발행: %s -> %s\n", idx+1, path, statusStr)

		lc.socketClient.Publish(path, statusStr)

		// 메인 서버의 상태 저장소도 업데이트
		UpdateLightState(idx+1, statusStr)

		lc.prevStatus[idx] = newStatus
	} else {
		fmt.Printf("[소켓] 조명 %d 상태 변경 없음 (현재: %d, 이전: %d)\n", idx+1, newStatus, lc.prevStatus[idx])
	}
}

func (lc *LightController) processStatusResponse(resp string) {
	fmt.Printf("[RS485] 상태 응답 처리 시작: %s\n", resp)
	fmt.Printf("[RS485] 응답 길이: %d, ON prefix: %s, OFF prefix: %s\n", len(resp), lc.statusOnPrefix, lc.statusOffPrefix)

	if len(resp) >= 6 {
		numStr := resp[4:6]
		fmt.Printf("[RS485] 조명 번호 추출: %s\n", numStr)

		idx, err := strconv.ParseInt(numStr, 16, 0)
		if err != nil {
			fmt.Printf("[RS485] 조명 번호 파싱 에러: %v\n", err)
			return
		}

		if idx >= 1 && idx <= 5 {
			arrayIdx := int(idx) - 1
			fmt.Printf("[RS485] 조명 %d 상태 확인 - 배열 인덱스: %d\n", idx, arrayIdx)

			if strings.HasPrefix(resp, lc.statusOnPrefix) {
				fmt.Printf("[RS485] 조명 %d ON 상태 확인됨 (응답: %s, prefix: %s)\n", idx, resp, lc.statusOnPrefix)
				lc.statusMu.Lock()
				oldStatus := lc.lightStatus[arrayIdx]
				lc.lightStatus[arrayIdx] = 1
				newStatus := lc.lightStatus[arrayIdx]
				lc.statusMu.Unlock()

				fmt.Printf("[RS485] 조명 %d 상태 업데이트: %d -> %d\n", idx, oldStatus, newStatus)
				lc.publishStateIfChanged(arrayIdx, newStatus)
			} else if strings.HasPrefix(resp, lc.statusOffPrefix) {
				fmt.Printf("[RS485] 조명 %d OFF 상태 확인됨 (응답: %s, prefix: %s)\n", idx, resp, lc.statusOffPrefix)
				lc.statusMu.Lock()
				oldStatus := lc.lightStatus[arrayIdx]
				lc.lightStatus[arrayIdx] = 0
				newStatus := lc.lightStatus[arrayIdx]
				lc.statusMu.Unlock()

				fmt.Printf("[RS485] 조명 %d 상태 업데이트: %d -> %d\n", idx, oldStatus, newStatus)
				lc.publishStateIfChanged(arrayIdx, newStatus)
			} else {
				fmt.Printf("[RS485] 조명 %d 알 수 없는 상태 응답 (응답: %s, ON prefix: %s, OFF prefix: %s)\n", idx, resp, lc.statusOnPrefix, lc.statusOffPrefix)
			}
		} else {
			fmt.Printf("[RS485] 조명 번호 범위 오류: %d (1-5 범위 밖)\n", idx)
		}
	} else {
		fmt.Printf("[RS485] 응답 길이 부족: %d (최소 6자리 필요)\n", len(resp))
	}
}

func (lc *LightController) statusQueryLoop(portName string) {
	paused := false
	for {
		select {
		case <-lc.pauseStatusQuery:
			paused = true
			<-lc.resumeStatusQuery
			paused = false
		default:
			if paused {
				time.Sleep(75 * time.Millisecond)
				continue
			}

			for _, pkt := range lc.statusQueryPackets {
				fmt.Printf("[RS485] 상태 조회 패킷 전송: %s\n", pkt)

				lc.writeMu.Lock()
				_, err := lc.port.Write(hexStringToBytes(pkt))
				lc.writeMu.Unlock()

				if err != nil {
					fmt.Printf("[RS485] 시리얼 포트 쓰기 에러: %v\n", err)
					fmt.Println("[조명] 시리얼 포트 에러 발생, 재연결 시도:", err)
					if reconnectErr := lc.reconnectSerial(portName); reconnectErr != nil {
						continue
					}
					continue
				}

				n, err := lc.port.Read(lc.readBuffer)
				if err != nil {
					fmt.Printf("[RS485] 시리얼 포트 읽기 에러: %v\n", err)
					fmt.Println("[조명] 시리얼 포트 읽기 에러, 재연결 시도:", err)
					if reconnectErr := lc.reconnectSerial(portName); reconnectErr != nil {
						continue
					}
					continue
				}

				if n > 0 {
					resp := strings.ToUpper(hex.EncodeToString(lc.readBuffer[:n]))
					fmt.Printf("[RS485] 응답 수신 (길이: %d): %s\n", n, resp)

					if n >= 8 {
						lc.processStatusResponse(resp)
					} else {
						fmt.Printf("[RS485] 응답 길이 부족 (최소 8바이트 필요, 수신: %d바이트)\n", n)
					}
				} else {
					fmt.Printf("[RS485] 응답 없음 (0바이트)\n")
				}

				time.Sleep(75 * time.Millisecond)
			}
			time.Sleep(75 * time.Millisecond)
		}
	}
}

func (lc *LightController) Run(portName string) {
	lc.subscribeToCommands()

	// 상태 조회 및 소켓 상태 발행 고루틴
	go lc.statusQueryLoop(portName)

	select {} // 고루틴 대기
}

// 명령 처리 메서드
func (lc *LightController) handleCommands() {
	for {
		select {
		case cmd := <-lightCommands:
			lc.processLightCommand(cmd)
		}
	}
}

// 조명 명령 처리
func (lc *LightController) processLightCommand(cmd LightCommand) {
	lightNum, err := strconv.Atoi(cmd.LightNum)
	if err != nil {
		fmt.Printf("[조명] 잘못된 조명 번호: %s\n", cmd.LightNum)
		return
	}

	if lightNum < 1 || lightNum > 5 {
		fmt.Printf("[조명] 지원하지 않는 조명 번호: %d\n", lightNum)
		return
	}

	idx := lightNum - 1 // 배열 인덱스로 변환

	switch cmd.Value {
	case "ON":
		fmt.Printf("[조명] 조명 %d ON 명령 실행\n", lightNum)
		lc.sendLightCommand(idx, true)
	case "OFF":
		fmt.Printf("[조명] 조명 %d OFF 명령 실행\n", lightNum)
		lc.sendLightCommand(idx, false)
	default:
		fmt.Printf("[조명] 알 수 없는 명령: %s\n", cmd.Value)
	}
}

// 조명 명령 전송
func (lc *LightController) sendLightCommand(idx int, turnOn bool) {
	var packet string
	if turnOn {
		packet = lc.onPackets[idx]
	} else {
		packet = lc.offPackets[idx]
	}

	fmt.Printf("[RS485] 조명 명령 패킷 전송: %s\n", packet)

	// 상태 조회 일시 중지
	lc.pauseStatusQuery <- true

	lc.writeMu.Lock()
	_, err := lc.port.Write(hexStringToBytes(packet))
	lc.writeMu.Unlock()

	if err != nil {
		fmt.Printf("[RS485] 조명 명령 전송 실패: %v\n", err)
	} else {
		fmt.Printf("[RS485] 조명 명령 전송 성공\n")
	}

	// 상태 조회 재개
	time.Sleep(100 * time.Millisecond)
	lc.resumeStatusQuery <- true
}

func onOff(val int) string {
	if val == 1 {
		return "ON"
	}
	return "OFF"
}

func openSerialLight(portName string) (*serial.Port, error) {
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

func RunLightController(portName string, socketPath string, prefix string) {
	controller := NewLightController(portName, socketPath, prefix)
	if controller == nil {
		fmt.Println("[조명] 컨트롤러 초기화 실패")
		return
	}

	// 명령 처리 고루틴 시작
	go controller.handleCommands()

	controller.Run(portName)
}
