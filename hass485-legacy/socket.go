package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// SocketMessage 소켓 메시지 구조체
type SocketMessage struct {
	Type    string      `json:"type"`    // "GET", "SET", "SUBSCRIBE"
	Path    string      `json:"path"`    // "/lights/1/state", "/boilers/2/mode"
	Value   interface{} `json:"value"`   // "ON", "OFF", "heat", etc.
	ID      string      `json:"id"`      // 요청 ID (응답 매칭용)
	Timeout int         `json:"timeout"` // 타임아웃 (초)
}

// SocketResponse 소켓 응답 구조체
type SocketResponse struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SocketClient 소켓 클라이언트 구조체
type SocketClient struct {
	socketPath string
	prefix     string
	conn       net.Conn
	mu         sync.Mutex
	callbacks  map[string]func(SocketMessage)
	callbackMu sync.RWMutex
}

// NewSocketClient 새로운 소켓 클라이언트 생성
func NewSocketClient(socketPath, prefix string) *SocketClient {
	return &SocketClient{
		socketPath: socketPath,
		prefix:     prefix,
		callbacks:  make(map[string]func(SocketMessage)),
	}
}

// Connect 소켓 서버에 연결
func (sc *SocketClient) Connect() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var err error
	for {
		sc.conn, err = net.Dial("unix", sc.socketPath)
		if err != nil {
			log.Printf("[소켓] 연결 실패: %v, 3초 후 재시도", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Printf("[소켓] 연결 성공: %s", sc.socketPath)
		break
	}

	// 응답 수신 고루틴 시작
	go sc.readResponses()

	return nil
}

// readResponses 응답 수신 루프
func (sc *SocketClient) readResponses() {
	buffer := make([]byte, 1024)
	for {
		n, err := sc.conn.Read(buffer)
		if err != nil {
			log.Printf("[소켓] 읽기 실패: %v", err)
			sc.reconnect()
			continue
		}

		message := string(buffer[:n])
		sc.handleResponse(message)
	}
}

// handleResponse 응답 처리
func (sc *SocketClient) handleResponse(message string) {
	// 여러 JSON 메시지가 연결되어 있을 수 있으므로 분리 처리
	responses := sc.parseMultipleResponses(message)

	for _, response := range responses {
		sc.callbackMu.RLock()
		callback, exists := sc.callbacks[response.ID]
		sc.callbackMu.RUnlock()

		if exists {
			// 콜백이 있으면 해당 메시지로 변환하여 호출
			msg := SocketMessage{
				Type:  "RESPONSE",
				Path:  "",
				Value: response.Data,
				ID:    response.ID,
			}
			callback(msg)
		}
	}
}

// parseMultipleResponses 여러 JSON 응답을 파싱
func (sc *SocketClient) parseMultipleResponses(message string) []SocketResponse {
	var responses []SocketResponse

	// 메시지 버퍼에서 완전한 JSON 객체들을 찾아서 처리
	start := 0
	depth := 0

	for i, char := range message {
		if char == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 {
				// 완전한 JSON 객체 추출
				jsonMessage := message[start : i+1]

				var response SocketResponse
				if err := json.Unmarshal([]byte(jsonMessage), &response); err != nil {
					log.Printf("[소켓] 응답 파싱 실패: %v", err)
					continue
				}
				responses = append(responses, response)
			}
		}
	}

	return responses
}

// reconnect 재연결
func (sc *SocketClient) reconnect() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.conn != nil {
		sc.conn.Close()
		sc.conn = nil
	}

	log.Printf("[소켓] 재연결 시도: %s", sc.socketPath)
	sc.Connect()
}

// Send 메시지 전송
func (sc *SocketClient) Send(msg SocketMessage) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.conn == nil {
		return fmt.Errorf("소켓 연결 없음")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("메시지 직렬화 실패: %v", err)
	}

	_, err = sc.conn.Write(data)
	if err != nil {
		log.Printf("[소켓] 쓰기 실패: %v", err)
		sc.reconnect()
		return err
	}

	return nil
}

// Subscribe 구독 설정
func (sc *SocketClient) Subscribe(path string, callback func(SocketMessage)) {
	sc.callbackMu.Lock()
	defer sc.callbackMu.Unlock()

	sc.callbacks[path] = callback

	msg := SocketMessage{
		Type: "SUBSCRIBE",
		Path: path,
	}

	sc.Send(msg)
}

// Publish 상태 발행
func (sc *SocketClient) Publish(path string, value interface{}) {
	msg := SocketMessage{
		Type:  "PUBLISH",
		Path:  path,
		Value: value,
	}

	sc.Send(msg)
}

// Get 상태 조회
func (sc *SocketClient) Get(path string) (interface{}, error) {
	msg := SocketMessage{
		Type: "GET",
		Path: path,
		ID:   fmt.Sprintf("get_%d", time.Now().UnixNano()),
	}

	// 응답을 받기 위한 채널 생성
	responseChan := make(chan interface{}, 1)
	sc.callbackMu.Lock()
	sc.callbacks[msg.ID] = func(response SocketMessage) {
		responseChan <- response.Value
	}
	sc.callbackMu.Unlock()

	// 메시지 전송
	if err := sc.Send(msg); err != nil {
		return nil, err
	}

	// 응답 대기 (타임아웃 5초)
	select {
	case response := <-responseChan:
		return response, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("응답 타임아웃")
	}
}

// Set 명령 전송
func (sc *SocketClient) Set(path string, value interface{}) error {
	msg := SocketMessage{
		Type:  "SET",
		Path:  path,
		Value: value,
		ID:    fmt.Sprintf("set_%d", time.Now().UnixNano()),
	}

	// 응답을 받기 위한 채널 생성
	responseChan := make(chan bool, 1)
	sc.callbackMu.Lock()
	sc.callbacks[msg.ID] = func(response SocketMessage) {
		responseChan <- true
	}
	sc.callbackMu.Unlock()

	// 메시지 전송
	if err := sc.Send(msg); err != nil {
		return err
	}

	// 응답 대기 (타임아웃 5초)
	select {
	case <-responseChan:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("응답 타임아웃")
	}
}
