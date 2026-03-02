package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// 일괄소등(현재 엘리베이터 컨트롤러 명칭 유지) MQTT 버전
type AlloffController struct {
	port       *serial.Port
	mqttClient MQTT.Client
	lastState  string
}

// 일괄소등 패킷(엘리베이터 호출 제거)
const (
	ALLOFF_STATUS_QUERY = "AA5504000300"
	ALLOFF_ON_COMMAND   = "AA5504000102"
	ALLOFF_OFF_COMMAND  = "AA5504000103"
)

// AlloffController 생성 (주 역할: 일괄소등)
func NewAlloffController(devicePath string, mqttClient MQTT.Client) *AlloffController {
	config := &serial.Config{
		Name: devicePath,
		Baud: 9600,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("일괄소등용 시리얼 포트 열기 실패: %v", err)
		return nil
	}

	controller := &AlloffController{
		port:       port,
		mqttClient: mqttClient,
		lastState:  "OFF",
	}

	// 상태 쿼리는 일괄소등 버튼에만 유지
	go controller.statusQueryLoop()

	return controller
}

// 상태 쿼리 루프
func (ac *AlloffController) statusQueryLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ac.queryAllOffStatus()
		}
	}
}

// 일괄소등 상태 쿼리
func (ac *AlloffController) queryAllOffStatus() {
	queryPacket := ALLOFF_STATUS_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("일괄소등 상태 쿼리 패킷 생성 실패: %v", err)
		return
	}

	_, err = ac.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 상태 쿼리 전송 실패: %v", err)
		return
	}

	// 응답 읽기
	response := make([]byte, 32)
	n, err := ac.port.Read(response)
	if err != nil {
		log.Printf("일괄소등 상태 응답 읽기 실패: %v", err)
		return
	}

	// 응답 파싱
	state := ac.parseStatusResponse(response[:n])

	// 상태가 변경되었으면 MQTT로 발행
	if state != ac.lastState {
		ac.publishAllOffState(state)
		ac.lastState = state
	}
}

// 응답 파싱
func (ac *AlloffController) parseStatusResponse(response []byte) string {
	// 실제 구현에서는 응답 형식에 맞게 수정
	if len(response) > 0 && response[0] == 0x01 {
		return "ON"
	}
	return "OFF"
}

// MQTT로 일괄소등 상태 발행
func (ac *AlloffController) publishAllOffState(state string) {
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("일괄소등 상태 발행 실패: %v", err)
		return
	}

	ac.mqttClient.Publish("hass485/alloff/state", 0, false, payload)
	log.Printf("일괄소등 상태 발행: %s", state)
}

// 일괄소등 제어
func (ac *AlloffController) handleAllOffCommand(commandType string, value interface{}) {
	switch commandType {
	case "on":
		ac.turnAllOffOn()
	case "off":
		ac.turnAllOffOff()
	default:
		log.Printf("일괄소등 알 수 없는 명령: %s", commandType)
	}
}

// 일괄소등 켜기
func (ac *AlloffController) turnAllOffOn() {
	packet := ALLOFF_ON_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("일괄소등 켜기 패킷 생성 실패: %v", err)
		return
	}

	_, err = ac.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 켜기 전송 실패: %v", err)
		return
	}

	log.Printf("일괄소등 켜기 명령 실행")

	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	ac.queryAllOffStatus()
}

// 일괄소등 끄기
func (ac *AlloffController) turnAllOffOff() {
	packet := ALLOFF_OFF_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("일괄소등 끄기 패킷 생성 실패: %v", err)
		return
	}

	_, err = ac.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 끄기 전송 실패: %v", err)
		return
	}

	log.Printf("일괄소등 끄기 명령 실행")

	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	ac.queryAllOffStatus()
}

// 정리
func (ac *AlloffController) Close() {
	if ac.port != nil {
		ac.port.Close()
	}
}
