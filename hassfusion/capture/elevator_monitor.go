package capture

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"hassfusion/config"
	"hassfusion/ws"
)

const soapBody = `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ces="urn:ces">
   <soapenv:Header/>
   <soapenv:Body>
      <ces:getEvStatus/>
   </soapenv:Body>
</soapenv:Envelope>`

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    Body     `xml:"Body"`
}

type Body struct {
	GetEvStatusResponse GetEvStatusResponse `xml:"getEvStatusResponse"`
}

type GetEvStatusResponse struct {
	Return Out `xml:"return"`
}

type Out struct {
	Items []Item `xml:"item"`
}

type Item struct {
	CarFloor     string `xml:"carFloor"`
	IsBasement   string `xml:"isBasement"`
	CarDirection string `xml:"carDirection"`
	EvStatus     string `xml:"evStatus"`
	CallUp       string `xml:"callUp"`
	CallDown     string `xml:"callDown"`
}

type ElevatorState struct {
	Floor      string
	IsBasement string
	Direction  string
	Status     string
	CallUp     string
	CallDown   string
}

type ElevatorMonitor struct {
	cfg            *config.Config
	wsServer       *ws.Server
	soapURL        string
	previousStates map[int]ElevatorState
	stateMu        sync.RWMutex // previousStates 동시 접근 보호
	httpClient     *http.Client // 연결 관리용 HTTP 클라이언트
}

func NewElevatorMonitor(cfg *config.Config, wsServer *ws.Server) *ElevatorMonitor {
	if cfg.Wallpad.IP == "" {
		log.Println("[ELEVATOR] Wallpad IP not configured. Skipping elevator monitor.")
		return nil
	}

	return &ElevatorMonitor{
		cfg:            cfg,
		wsServer:       wsServer,
		soapURL:        fmt.Sprintf("http://%s:29715", cfg.Wallpad.IP),
		previousStates: make(map[int]ElevatorState),
		httpClient: &http.Client{
			Timeout: 10 * time.Second, // 타임아웃을 10초로 넉넉하게 연장
		},
	}
}

func (em *ElevatorMonitor) Run() {
	log.Println("[ELEVATOR] Monitoring started (Polite Polling Mode)...")

	errorCount := 0

	for {
		items, err := em.getElevatorStatus()
		if err != nil {
			errorCount++
			// 연속 3번 이상 실패 시에만 로그 출력 (도배 방지)
			if errorCount >= 3 {
				log.Printf("[ELEVATOR] Fetch error (Retry %d): %v\n", errorCount, err)
				// 중앙 서버가 많이 힘들어 보이면 10초간 아예 요청을 멈추고 쉬어줍니다
				time.Sleep(10 * time.Second)
			} else {
				// 가벼운 지연일 경우 3초 대기 후 재시도
				time.Sleep(3 * time.Second)
			}
			continue
		}

		// 성공 시 에러 카운터 초기화
		errorCount = 0

		allStopped := true
		for idx, item := range items {
			if item.CarDirection != "0" {
				allStopped = false
			}

			currentState := ElevatorState{
				Floor:      item.CarFloor,
				IsBasement: item.IsBasement,
				Direction:  item.CarDirection,
				Status:     item.EvStatus,
				CallUp:     item.CallUp,
				CallDown:   item.CallDown,
			}

			if em.hasStateChanged(idx, currentState) {
				em.broadcastChanges(idx+1, currentState)
				em.stateMu.Lock()
				em.previousStates[idx] = currentState
				em.stateMu.Unlock()
			}
		}

		// 아파트 서버 부하 경감을 위한 폴링 주기 조절
		if allStopped {
			time.Sleep(5 * time.Second) // 엘리베이터가 모두 멈춰있을 땐 5초 간격으로 확인
		} else {
			time.Sleep(3 * time.Second) // 움직이고 있을 땐 3초 간격으로 확인
		}
	}
}

// BroadcastAll sends the current cached state of both elevators
func (em *ElevatorMonitor) BroadcastAll() {
	em.stateMu.RLock()
	states := make(map[int]ElevatorState, len(em.previousStates))
	for idx, s := range em.previousStates {
		states[idx] = s
	}
	em.stateMu.RUnlock()

	for idx, s := range states {
		em.broadcastChanges(idx+1, s)
	}
}

func (em *ElevatorMonitor) getElevatorStatus() ([]Item, error) {
	req, err := http.NewRequest("POST", em.soapURL, bytes.NewBuffer([]byte(soapBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "urn:ces#getEvStatus")

	// 매번 새 Client를 만들지 않고 재사용 (서버가 끊더라도 로컬 리소스 절약)
	resp, err := em.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// ioutil.ReadAll 대신 최신 Go 표준인 io.ReadAll 사용
	body, _ := io.ReadAll(resp.Body)

	var env Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, err
	}

	return env.Body.GetEvStatusResponse.Return.Items, nil
}

func (em *ElevatorMonitor) hasStateChanged(elevatorIndex int, currentState ElevatorState) bool {
	em.stateMu.RLock()
	prevState, exists := em.previousStates[elevatorIndex]
	em.stateMu.RUnlock()
	if !exists {
		return true
	}
	return prevState != currentState
}

func (em *ElevatorMonitor) broadcastChanges(evNum int, s ElevatorState) {
	// 1. Floor (sensor)
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "sensor", DeviceID: fmt.Sprintf("elevator_%d_floor", evNum), State: s.Floor,
	})

	// 2. Direction (sensor)
	dir := "stop"
	if s.Direction == "1" {
		dir = "up"
	} else if s.Direction == "2" {
		dir = "down"
	}
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "sensor", DeviceID: fmt.Sprintf("elevator_%d_direction", evNum), State: dir,
	})

	// 3. Status (sensor)
	status := "error"
	if s.Status == "1" {
		status = "normal"
	}
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "sensor", DeviceID: fmt.Sprintf("elevator_%d_status", evNum), State: status,
	})

	// 4. Call Up (binary_sensor)
	callUp := "off"
	if s.CallUp == "1" {
		callUp = "on"
	}
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "binary_sensor", DeviceID: fmt.Sprintf("elevator_%d_call_up", evNum), State: callUp,
	})

	// 5. Call Down (binary_sensor)
	callDown := "off"
	if s.CallDown == "1" {
		callDown = "on"
	}
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "binary_sensor", DeviceID: fmt.Sprintf("elevator_%d_call_down", evNum), State: callDown,
	})

	// 6. Is Basement (binary_sensor)
	isBasement := "off"
	if s.IsBasement == "1" {
		isBasement = "on"
	}
	em.wsServer.Broadcast(ws.WSMsg{
		Type: "event", Domain: "binary_sensor", DeviceID: fmt.Sprintf("elevator_%d_basement", evNum), State: isBasement,
	})
}
