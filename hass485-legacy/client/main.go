package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// SocketMessage 소켓 메시지 구조체
type SocketMessage struct {
	Type    string      `json:"type"`
	Path    string      `json:"path"`
	Value   interface{} `json:"value"`
	ID      string      `json:"id"`
	Timeout int         `json:"timeout"`
}

// 테스트 클라이언트
func main() {
	if len(os.Args) < 2 {
		fmt.Println("사용법: go run main.go <명령>")
		fmt.Println("명령 예시:")
		fmt.Println("  light-on 1     - 조명 1번 ON")
		fmt.Println("  light-off 1    - 조명 1번 OFF")
		fmt.Println("  boiler-heat 2  - 보일러 2번 방 히팅 ON")
		fmt.Println("  boiler-off 2   - 보일러 2번 방 OFF")
		fmt.Println("  boiler-temp 2 25 - 보일러 2번 방 온도 25도 설정")
		fmt.Println("  elevator-call   - 엘리베이터 호출")
		fmt.Println("  alloff-on       - 일괄소등 ON")
		fmt.Println("  alloff-off      - 일괄소등 OFF")
		fmt.Println("  door-open       - 도어 열기")
		return
	}

	socketPath := "/config/hass485.sock"
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		log.Fatalf("소켓 연결 실패: %v", err)
	}
	defer conn.Close()

	command := os.Args[1]
	var msg SocketMessage

	switch command {
	case "light-on":
		if len(os.Args) < 3 {
			fmt.Println("조명 번호를 지정하세요. 예: light-on 1")
			return
		}
		lightNum := os.Args[2]
		msg = SocketMessage{
			Type:  "SET",
			Path:  fmt.Sprintf("/lights/%s/set", lightNum),
			Value: "ON",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "light-off":
		if len(os.Args) < 3 {
			fmt.Println("조명 번호를 지정하세요. 예: light-off 1")
			return
		}
		lightNum := os.Args[2]
		msg = SocketMessage{
			Type:  "SET",
			Path:  fmt.Sprintf("/lights/%s/set", lightNum),
			Value: "OFF",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "boiler-heat":
		if len(os.Args) < 3 {
			fmt.Println("방 번호를 지정하세요. 예: boiler-heat 2")
			return
		}
		roomNum := os.Args[2]
		msg = SocketMessage{
			Type:  "SET",
			Path:  fmt.Sprintf("/boilers/%s/mode/set", roomNum),
			Value: "heat",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "boiler-off":
		if len(os.Args) < 3 {
			fmt.Println("방 번호를 지정하세요. 예: boiler-off 2")
			return
		}
		roomNum := os.Args[2]
		msg = SocketMessage{
			Type:  "SET",
			Path:  fmt.Sprintf("/boilers/%s/mode/set", roomNum),
			Value: "off",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "boiler-temp":
		if len(os.Args) < 4 {
			fmt.Println("방 번호와 온도를 지정하세요. 예: boiler-temp 2 25")
			return
		}
		roomNum := os.Args[2]
		temp := os.Args[3]
		msg = SocketMessage{
			Type:  "SET",
			Path:  fmt.Sprintf("/boilers/%s/temperature/set", roomNum),
			Value: temp,
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "elevator-call":
		msg = SocketMessage{
			Type:  "SET",
			Path:  "/elevator/call/set",
			Value: "ON",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "alloff-on":
		msg = SocketMessage{
			Type:  "SET",
			Path:  "/alloff/set",
			Value: "ON",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "alloff-off":
		msg = SocketMessage{
			Type:  "SET",
			Path:  "/alloff/set",
			Value: "OFF",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	case "door-open":
		msg = SocketMessage{
			Type:  "SET",
			Path:  "/doorbell/open/set",
			Value: "ON",
			ID:    fmt.Sprintf("test_%d", time.Now().UnixNano()),
		}

	default:
		fmt.Printf("알 수 없는 명령: %s\n", command)
		return
	}

	// 메시지 전송
	data, err := json.Marshal(msg)
	if err != nil {
		log.Fatalf("메시지 직렬화 실패: %v", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Fatalf("메시지 전송 실패: %v", err)
	}

	fmt.Printf("명령 전송 완료: %s\n", command)

	// 응답 대기
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("응답 읽기 실패: %v", err)
		return
	}

	response := string(buffer[:n])
	fmt.Printf("서버 응답: %s\n", response)
}
