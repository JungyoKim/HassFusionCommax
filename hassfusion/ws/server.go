package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second    // max time to flush one message to a client
	pongWait   = 60 * time.Second    // drop a client that goes silent this long
	pingPeriod = (pongWait * 9) / 10 // ping cadence, must be < pongWait
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
	mu   sync.Mutex    // 이 커넥션에 대한 동시 쓰기를 방지하는 락
	done chan struct{} // closed when the read loop exits, to stop pingLoop
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
	client := &Client{conn: conn, done: make(chan struct{})}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	// Keepalive: without a read deadline a half-open peer lingers forever in the
	// client map; without a write deadline a stalled peer blocks every Broadcast.
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go s.pingLoop(client)

	defer func() {
		close(client.done)
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

		// Any inbound frame proves the peer is alive — extend its read deadline.
		conn.SetReadDeadline(time.Now().Add(pongWait))

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
		err := c.writeMessage(websocket.TextMessage, p)

		if err != nil {
			log.Printf("WS Write Error: %v", err)
			c.conn.Close()
			s.RemoveClient(c) // 에러 발생 시 목록에서 제거
		}
	}
}

// writeMessage serializes one write with a deadline so a stalled (half-open)
// client can never block the broadcast loop indefinitely.
func (c *Client) writeMessage(mt int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(mt, data)
}

// pingLoop keeps the connection alive and detects dead peers: a failed ping
// closes the socket, which unblocks the read loop and reaps the client.
func (s *Server) pingLoop(c *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.writeMessage(websocket.PingMessage, nil); err != nil {
				c.conn.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}
