package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// LightController MQTT 버전
type LightController struct {
	port        *serial.Port
	lightNumber int
	mqttClient  MQTT.Client
	lastState   string
}

// 조명 패킷 정의
const (
	LIGHT_ON_PACKET    = "AA5501000101"
	LIGHT_OFF_PACKET   = "AA5501000100"
	LIGHT_STATUS_QUERY = "AA5501000200"
)

// 조명 이름
var LIGHT_NAMES = []string{
	"거실 조명 1",
	"거실 조명 2",
	"거실 조명 3",
	"거실 조명 4",
	"복도 조명",
}

// LightController 생성
func NewLightController(devicePath string, lightNumber int, mqttClient MQTT.Client) *LightController {
	config := &serial.Config{
		Name: devicePath,
		Baud: 9600,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("조명 %d 시리얼 포트 열기 실패: %v", lightNumber, err)
		return nil
	}

	controller := &LightController{
		port:        port,
		lightNumber: lightNumber,
		mqttClient:  mqttClient,
		lastState:   "OFF",
	}

	// 상태 모니터링 시작
	go controller.statusQueryLoop()

	return controller
}

// 상태 쿼리 루프
func (lc *LightController) statusQueryLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lc.queryStatus()
		}
	}
}

// 상태 쿼리
func (lc *LightController) queryStatus() {
	// RS485로 상태 쿼리
	queryPacket := LIGHT_STATUS_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("조명 %d 상태 쿼리 패킷 생성 실패: %v", lc.lightNumber, err)
		return
	}

	_, err = lc.port.Write(packetBytes)
	if err != nil {
		log.Printf("조명 %d 상태 쿼리 전송 실패: %v", lc.lightNumber, err)
		return
	}

	// 응답 읽기
	response := make([]byte, 32)
	n, err := lc.port.Read(response)
	if err != nil {
		log.Printf("조명 %d 상태 응답 읽기 실패: %v", lc.lightNumber, err)
		return
	}

	// 응답 파싱 (실제 구현에서는 응답 형식에 맞게 수정)
	state := lc.parseStatusResponse(response[:n])

	// 상태가 변경되었으면 MQTT로 발행
	if state != lc.lastState {
		lc.publishState(state)
		lc.lastState = state
	}
}

// 응답 파싱 (예시)
func (lc *LightController) parseStatusResponse(response []byte) string {
	// 실제 구현에서는 응답 형식에 맞게 수정
	// 여기서는 간단히 ON/OFF로 반환
	if len(response) > 0 && response[0] == 0x01 {
		return "ON"
	}
	return "OFF"
}

// MQTT로 상태 발행
func (lc *LightController) publishState(state string) {
	topic := fmt.Sprintf("hass485/lights/%d/state", lc.lightNumber)
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("조명 %d 상태 발행 실패: %v", lc.lightNumber, err)
		return
	}

	lc.mqttClient.Publish(topic, 0, false, payload)
	log.Printf("조명 %d 상태 발행: %s", lc.lightNumber, state)
}

// 조명 제어
func (lc *LightController) handleCommand(commandType string, value interface{}) {
	var packet string

	switch commandType {
	case "on":
		packet = LIGHT_ON_PACKET
	case "off":
		packet = LIGHT_OFF_PACKET
	default:
		log.Printf("조명 %d 알 수 없는 명령: %s", lc.lightNumber, commandType)
		return
	}

	// RS485로 명령 전송
	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("조명 %d 명령 패킷 생성 실패: %v", lc.lightNumber, err)
		return
	}

	_, err = lc.port.Write(packetBytes)
	if err != nil {
		log.Printf("조명 %d 명령 전송 실패: %v", lc.lightNumber, err)
		return
	}

	log.Printf("조명 %d 명령 실행: %s", lc.lightNumber, commandType)

	// 명령 실행 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	lc.queryStatus()
}

// 정리
func (lc *LightController) Close() {
	if lc.port != nil {
		lc.port.Close()
	}
}
