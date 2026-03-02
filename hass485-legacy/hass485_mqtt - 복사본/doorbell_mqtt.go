package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// DoorbellController MQTT 버전
type DoorbellController struct {
	port        *serial.Port
	mqttClient  MQTT.Client
	lastState   string
}

// 도어벨 패킷 정의
const (
	DOORBELL_STATUS_QUERY = "AA5503000200"
	DOOR_OPEN_COMMAND     = "AA5503000101"
)

// DoorbellController 생성
func NewDoorbellController(devicePath string, mqttClient MQTT.Client) *DoorbellController {
	config := &serial.Config{
		Name: devicePath,
		Baud: 9600,
	}
	
	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("도어벨 시리얼 포트 열기 실패: %v", err)
		return nil
	}
	
	controller := &DoorbellController{
		port:       port,
		mqttClient: mqttClient,
		lastState:  "OFF",
	}
	
	// 상태 모니터링 시작
	go controller.statusQueryLoop()
	
	return controller
}

// 상태 쿼리 루프
func (dc *DoorbellController) statusQueryLoop() {
	ticker := time.NewTicker(2 * time.Second) // 도어벨은 2초마다 쿼리 (빠른 응답 필요)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			dc.queryStatus()
		}
	}
}

// 상태 쿼리
func (dc *DoorbellController) queryStatus() {
	queryPacket := DOORBELL_STATUS_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("도어벨 상태 쿼리 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = dc.port.Write(packetBytes)
	if err != nil {
		log.Printf("도어벨 상태 쿼리 전송 실패: %v", err)
		return
	}
	
	// 응답 읽기
	response := make([]byte, 32)
	n, err := dc.port.Read(response)
	if err != nil {
		log.Printf("도어벨 상태 응답 읽기 실패: %v", err)
		return
	}
	
	// 응답 파싱
	state := dc.parseStatusResponse(response[:n])
	
	// 상태가 변경되었으면 MQTT로 발행
	if state != dc.lastState {
		dc.publishState(state)
		dc.lastState = state
	}
}

// 응답 파싱
func (dc *DoorbellController) parseStatusResponse(response []byte) string {
	// 실제 구현에서는 응답 형식에 맞게 수정
	if len(response) > 0 && response[0] == 0x01 {
		return "ON"
	}
	return "OFF"
}

// MQTT로 상태 발행
func (dc *DoorbellController) publishState(state string) {
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}
	
	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("도어벨 상태 발행 실패: %v", err)
		return
	}
	
	dc.mqttClient.Publish("hass485/doorbell/state", 0, false, payload)
	log.Printf("도어벨 상태 발행: %s", state)
}

// 도어 제어
func (dc *DoorbellController) handleCommand(commandType string, value interface{}) {
	switch commandType {
	case "open":
		dc.openDoor()
	default:
		log.Printf("도어벨 알 수 없는 명령: %s", commandType)
	}
}

// 도어 열기
func (dc *DoorbellController) openDoor() {
	packet := DOOR_OPEN_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("도어 열기 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = dc.port.Write(packetBytes)
	if err != nil {
		log.Printf("도어 열기 전송 실패: %v", err)
		return
	}
	
	log.Printf("도어 열기 명령 실행")
	
	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	dc.queryStatus()
}

// 정리
func (dc *DoorbellController) Close() {
	if dc.port != nil {
		dc.port.Close()
	}
} 