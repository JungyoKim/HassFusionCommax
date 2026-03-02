package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// ElevatorController MQTT 버전
type ElevatorController struct {
	port        *serial.Port
	mqttClient  MQTT.Client
	lastState   string
}

// 엘리베이터 패킷 정의
const (
	ELEVATOR_STATUS_QUERY = "AA5504000200"
	ELEVATOR_CALL_COMMAND = "AA5504000101"
	ALLOFF_STATUS_QUERY   = "AA5504000300"
	ALLOFF_ON_COMMAND     = "AA5504000102"
	ALLOFF_OFF_COMMAND    = "AA5504000103"
)

// ElevatorController 생성
func NewElevatorController(devicePath string, mqttClient MQTT.Client) *ElevatorController {
	config := &serial.Config{
		Name: devicePath,
		Baud: 9600,
	}
	
	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("엘리베이터 시리얼 포트 열기 실패: %v", err)
		return nil
	}
	
	controller := &ElevatorController{
		port:       port,
		mqttClient: mqttClient,
		lastState:  "OFF",
	}
	
	// 상태 모니터링 시작
	go controller.statusQueryLoop()
	
	return controller
}

// 상태 쿼리 루프
func (ec *ElevatorController) statusQueryLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ec.queryAllOffStatus()
		}
	}
}

// 일괄소등 상태 쿼리
func (ec *ElevatorController) queryAllOffStatus() {
	queryPacket := ALLOFF_STATUS_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("일괄소등 상태 쿼리 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = ec.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 상태 쿼리 전송 실패: %v", err)
		return
	}
	
	// 응답 읽기
	response := make([]byte, 32)
	n, err := ec.port.Read(response)
	if err != nil {
		log.Printf("일괄소등 상태 응답 읽기 실패: %v", err)
		return
	}
	
	// 응답 파싱
	state := ec.parseStatusResponse(response[:n])
	
	// 상태가 변경되었으면 MQTT로 발행
	if state != ec.lastState {
		ec.publishAllOffState(state)
		ec.lastState = state
	}
}

// 응답 파싱
func (ec *ElevatorController) parseStatusResponse(response []byte) string {
	// 실제 구현에서는 응답 형식에 맞게 수정
	if len(response) > 0 && response[0] == 0x01 {
		return "ON"
	}
	return "OFF"
}

// MQTT로 일괄소등 상태 발행
func (ec *ElevatorController) publishAllOffState(state string) {
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}
	
	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("일괄소등 상태 발행 실패: %v", err)
		return
	}
	
	ec.mqttClient.Publish("hass485/alloff/state", 0, false, payload)
	log.Printf("일괄소등 상태 발행: %s", state)
}

// 엘리베이터 제어
func (ec *ElevatorController) handleElevatorCommand(commandType string, value interface{}) {
	switch commandType {
	case "call":
		ec.callElevator()
	default:
		log.Printf("엘리베이터 알 수 없는 명령: %s", commandType)
	}
}

// 일괄소등 제어
func (ec *ElevatorController) handleAllOffCommand(commandType string, value interface{}) {
	switch commandType {
	case "on":
		ec.turnAllOffOn()
	case "off":
		ec.turnAllOffOff()
	default:
		log.Printf("일괄소등 알 수 없는 명령: %s", commandType)
	}
}

// 엘리베이터 호출
func (ec *ElevatorController) callElevator() {
	packet := ELEVATOR_CALL_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("엘리베이터 호출 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = ec.port.Write(packetBytes)
	if err != nil {
		log.Printf("엘리베이터 호출 전송 실패: %v", err)
		return
	}
	
	log.Printf("엘리베이터 호출 명령 실행")
	
	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	ec.queryAllOffStatus()
}

// 일괄소등 켜기
func (ec *ElevatorController) turnAllOffOn() {
	packet := ALLOFF_ON_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("일괄소등 켜기 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = ec.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 켜기 전송 실패: %v", err)
		return
	}
	
	log.Printf("일괄소등 켜기 명령 실행")
	
	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	ec.queryAllOffStatus()
}

// 일괄소등 끄기
func (ec *ElevatorController) turnAllOffOff() {
	packet := ALLOFF_OFF_COMMAND
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("일괄소등 끄기 패킷 생성 실패: %v", err)
		return
	}
	
	_, err = ec.port.Write(packetBytes)
	if err != nil {
		log.Printf("일괄소등 끄기 전송 실패: %v", err)
		return
	}
	
	log.Printf("일괄소등 끄기 명령 실행")
	
	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	ec.queryAllOffStatus()
}

// 정리
func (ec *ElevatorController) Close() {
	if ec.port != nil {
		ec.port.Close()
	}
} 