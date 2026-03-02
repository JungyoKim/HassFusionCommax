package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// LightManager MQTT 버전
type LightManager struct {
	port        *serial.Port
	controllers []*LightController
	mu          sync.Mutex
}

// LightController MQTT 버전
type LightController struct {
	manager     *LightManager
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

// LightManager 생성
func NewLightManager(devicePath string, mqttClient MQTT.Client) *LightManager {
	config := &serial.Config{
		Name:        devicePath,
		Baud:        9600,
		ReadTimeout: 100 * time.Millisecond,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("조명 버스 시리얼 포트 열기 실패: %v", err)
		return nil
	}

	lm := &LightManager{
		port: port,
	}

	// 1~5번 조명 컨트롤러 등록
	for i := 1; i <= 5; i++ {
		lc := &LightController{
			manager:     lm,
			lightNumber: i,
			mqttClient:  mqttClient,
			lastState:   "OFF", // 초기값 부여
		}
		lm.controllers = append(lm.controllers, lc)
	}

	// 통합 릴레이 폴링 시작
	go lm.relayPollingLoop()

	return lm
}

// 릴레이 쿼리 루프 (단일 루프)
func (lm *LightManager) relayPollingLoop() {
	for {
		for _, lc := range lm.controllers {
			lc.queryStatus()
			time.Sleep(50 * time.Millisecond) // 숨고르기
		}
	}
}

// 명령 라우터
func (lm *LightManager) HandleCommand(lightNumber int, commandType string, value interface{}) {
	if lightNumber > 0 && lightNumber <= len(lm.controllers) {
		lm.controllers[lightNumber-1].handleCommand(commandType, value)
	} else {
		log.Printf("조명 %d 컨트롤러를 찾을 수 없습니다", lightNumber)
	}
}

// 정리
func (lm *LightManager) Close() {
	if lm.port != nil {
		lm.port.Close()
	}
}

// 상태 쿼리 (Mutex 잠금 적용)
func (lc *LightController) queryStatus() {
	lc.manager.mu.Lock()
	defer lc.manager.mu.Unlock()
	lc.doQueryStatus()
}

// 내부 동작 상태 쿼리
func (lc *LightController) doQueryStatus() {
	dummy := make([]byte, 256)
	lc.manager.port.Read(dummy)

	queryPacket := LIGHT_STATUS_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("조명 %d 상태 쿼리 패킷 생성 실패: %v", lc.lightNumber, err)
		return
	}

	_, err = lc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("조명 %d 상태 쿼리 전송 실패: %v", lc.lightNumber, err)
		return
	}

	response := make([]byte, 32)
	n, err := lc.manager.port.Read(response)
	if err != nil && err.Error() != "EOF" {
		log.Printf("조명 %d 상태 응답 읽기 무시 가능 오류: %v", lc.lightNumber, err)
		return
	}

	if n == 0 {
		return
	}

	state := lc.parseStatusResponse(response[:n])
	if state != lc.lastState {
		lc.publishState(state)
		lc.lastState = state
	}
}

// 응답 파싱
func (lc *LightController) parseStatusResponse(response []byte) string {
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
	log.Printf("조명 %d 상태 발행 확인: %s", lc.lightNumber, state)
}

// 조명 제어
func (lc *LightController) handleCommand(commandType string, value interface{}) {
	lc.manager.mu.Lock()
	defer lc.manager.mu.Unlock()

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

	dummy := make([]byte, 256)
	lc.manager.port.Read(dummy)

	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("조명 %d 명령 패킷 생성 실패: %v", lc.lightNumber, err)
		return
	}

	_, err = lc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("조명 %d 명령 전송 실패: %v", lc.lightNumber, err)
		return
	}

	log.Printf("조명 %d 명령 정상 실행 됨: %s", lc.lightNumber, commandType)

	time.Sleep(100 * time.Millisecond)
	lc.doQueryStatus()
}
