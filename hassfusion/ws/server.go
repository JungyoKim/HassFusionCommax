package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WSMsg represents the standard JSON message format over WebSocket
type WSMsg struct {
	Type       string                 `json:"type"`                 // "event" or "command"
	Domain     string                 `json:"domain"`               // e.g., "light", "climate", "sensor", "button"
	DeviceID   string                 `json:"device_id"`            // Unique ID like "light_1"
	Action     string                 `json:"action,omitempty"`     // Used in "command" (e.g., "turn_on")
	State      string                 `json:"state,omitempty"`      // Used in "event" (e.g., "on")
	Attributes map[string]interface{} `json:"attributes,omitempty"` // Extra data
	Value      interface{}            `json:"value,omitempty"`      // Command argument
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the local network HA connection
	},
}

// [수정1] 클라이언트 커넥션과 해당 커넥션 전용 쓰기 락(Mutex)을 묶어주는 구조체 생성
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex // 이 커넥션에 대한 동시 쓰기를 방지하는 락
}

// Server handles WebSocket connections to Home Assistant
type Server struct {
	clients map[*Client]bool // [수정2] *websocket.Conn 대신 *Client를 사용하도록 변경
	mu      sync.Mutex       // clients 맵과 handlers 맵을 보호하는 메인 락

	// Message router (Domain -> Handler)
	handlers map[string]func(WSMsg)
}

func NewServer() *Server {
	return &Server{
		clients:  make(map[*Client]bool),
		handlers: make(map[string]func(WSMsg)),
	}
}

// RegisterHandler allows device modules to receive commands
func (s *Server) RegisterHandler(domain string, handler func(WSMsg)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[domain] = handler
}

// HandleWS upgrades HTTP to WebSocket and manages the connection
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS Upgrade Error:", err)
		return
	}

	log.Printf("New client connected: %s", conn.RemoteAddr().String())

	// 새로운 클라이언트 객체 생성
	client := &Client{conn: conn}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	defer func() {
		s.RemoveClient(client)
		conn.Close()
		log.Printf("Client disconnected: %s", conn.RemoteAddr().String())
	}()

	// Read loop
	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS Read Error: %v", err)
			}
			break
		}

		var msg WSMsg
		if err := json.Unmarshal(p, &msg); err != nil {
			log.Printf("WS Invalid JSON payload: %v | Raw: %s", err, string(p))
			continue
		}

		// Route message to appropriate handler if it is a command
		if msg.Type == "command" {
			s.mu.Lock()
			handler, exists := s.handlers[msg.Domain]
			s.mu.Unlock()

			if exists {
				// Execute handler asynchronously so WS read isn't blocked
				go handler(msg)
			} else {
				log.Printf("WS Unsupported command domain: %s", msg.Domain)
			}
		}
	}
}

// [수정3] 클라이언트를 안전하게 제거하는 헬퍼 함수
func (s *Server) RemoveClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, c)
}

// Broadcast sends an event message to all connected HA clients
func (s *Server) Broadcast(msg WSMsg) {
	// Must only send events
	msg.Type = "event"

	p, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WS Marshal Error: %v", err)
		return
	}

	// [수정4] 메인 락을 짧게 쥐고 현재 접속 중인 클라이언트 목록만 복사해 옴
	s.mu.Lock()
	activeClients := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		activeClients = append(activeClients, c)
	}
	s.mu.Unlock() // 네트워크 전송 전에 메인 락을 즉시 해제!

	// 복사해 온 목록을 순회하며 데이터 전송
	for _, c := range activeClients {
		// 해당 커넥션의 쓰기 충돌만 방지하도록 개별 락 사용
		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, p)
		c.mu.Unlock()

		if err != nil {
			log.Printf("WS Write Error: %v", err)
			c.conn.Close()
			s.RemoveClient(c) // 에러 발생 시 목록에서 제거
		}
	}
}
