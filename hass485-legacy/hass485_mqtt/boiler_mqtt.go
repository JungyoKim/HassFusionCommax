package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// BoilerManager MQTT 버전
type BoilerManager struct {
	port        *serial.Port
	controllers []*BoilerController
	mu          sync.Mutex
}

// BoilerController MQTT 버전
type BoilerController struct {
	manager         *BoilerManager
	boilerNumber    int
	mqttClient      MQTT.Client
	lastMode        string
	lastCurrentTemp int
	lastSetTemp     int
}

// 보일러 패킷 정의
const (
	BOILER_MODE_QUERY = "AA5502000200"
	BOILER_TEMP_QUERY = "AA5502000300"
	BOILER_MODE_SET   = "AA5502000101" // 모드 설정
	BOILER_TEMP_SET   = "AA5502000102" // 온도 설정
)

// 보일러 이름
var BOILER_NAMES = []string{
	"거실 보일러",
	"안방 보일러",
	"공부방 보일러",
	"침대방 보일러",
}

// BoilerManager 생성
func NewBoilerManager(devicePath string, mqttClient MQTT.Client) *BoilerManager {
	config := &serial.Config{
		Name:        devicePath,
		Baud:        9600,
		ReadTimeout: 100 * time.Millisecond,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("보일러 버스 시리얼 포트 열기 실패: %v", err)
		return nil
	}

	bm := &BoilerManager{
		port: port,
	}

	for i := 1; i <= 4; i++ {
		bc := &BoilerController{
			manager:         bm,
			boilerNumber:    i,
			mqttClient:      mqttClient,
			lastMode:        "off",
			lastCurrentTemp: 20,
			lastSetTemp:     20,
		}
		bm.controllers = append(bm.controllers, bc)
	}

	// 통합 릴레이 폴링 시작
	go bm.relayPollingLoop()

	return bm
}

// 단일 릴레이 루프
func (bm *BoilerManager) relayPollingLoop() {
	for {
		for _, bc := range bm.controllers {
			bc.queryStatus()
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// 명령 라우터
func (bm *BoilerManager) HandleCommand(boilerNumber int, commandType string, value interface{}) {
	if boilerNumber > 0 && boilerNumber <= len(bm.controllers) {
		bm.controllers[boilerNumber-1].handleCommand(commandType, value)
	} else {
		log.Printf("보일러 %d 컨트롤러를 찾을 수 없습니다", boilerNumber)
	}
}

// 포트 정리
func (bm *BoilerManager) Close() {
	if bm.port != nil {
		bm.port.Close()
	}
}

// 상태 종합 쿼리
func (bc *BoilerController) queryStatus() {
	bc.manager.mu.Lock()
	defer bc.manager.mu.Unlock()

	bc.doQueryMode()
	time.Sleep(20 * time.Millisecond)
	bc.doQueryTemperature()
}

func (bc *BoilerController) doQueryMode() {
	dummy := make([]byte, 256)
	bc.manager.port.Read(dummy)

	packetBytes, err := hex.DecodeString(BOILER_MODE_QUERY)
	if err != nil {
		log.Printf("보일러 %d 모드 쿼리 패킷 오류: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 모드 쿼리 전송 실패: %v", bc.boilerNumber, err)
		return
	}

	response := make([]byte, 32)
	n, err := bc.manager.port.Read(response)
	if err != nil && err.Error() != "EOF" {
		log.Printf("보일러 %d 모드 응답 에러 (무시됨): %v", bc.boilerNumber, err)
		return
	}
	if n == 0 {
		return
	}

	mode := bc.parseModeResponse(response[:n])
	if mode != bc.lastMode {
		bc.lastMode = mode
		bc.publishState()
	}
}

func (bc *BoilerController) doQueryTemperature() {
	dummy := make([]byte, 256)
	bc.manager.port.Read(dummy)

	packetBytes, err := hex.DecodeString(BOILER_TEMP_QUERY)
	if err != nil {
		log.Printf("보일러 %d 온도 쿼리 패킷 오류: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 온도 쿼리 실패: %v", bc.boilerNumber, err)
		return
	}

	response := make([]byte, 32)
	n, err := bc.manager.port.Read(response)
	if err != nil && err.Error() != "EOF" {
		log.Printf("보일러 %d 온도 에러 (무시됨): %v", bc.boilerNumber, err)
		return
	}
	if n == 0 {
		return
	}

	currentTemp, setTemp := bc.parseTemperatureResponse(response[:n])
	if currentTemp != bc.lastCurrentTemp || setTemp != bc.lastSetTemp {
		bc.lastCurrentTemp = currentTemp
		bc.lastSetTemp = setTemp
		bc.publishState()
	}
}

func (bc *BoilerController) parseModeResponse(response []byte) string {
	if len(response) > 0 {
		switch response[0] {
		case 0x00:
			return "off"
		case 0x01:
			return "heat"
		case 0x02:
			return "cool"
		}
	}
	return "off"
}

func (bc *BoilerController) parseTemperatureResponse(response []byte) (currentTemp, setTemp int) {
	if len(response) >= 2 {
		currentTemp = int(response[0])
		setTemp = int(response[1])
	} else {
		currentTemp = 20
		setTemp = 20
	}
	return
}

func (bc *BoilerController) publishState() {
	topic := fmt.Sprintf("hass485/boilers/%d/state", bc.boilerNumber)
	message := MQTTMessage{
		Type: "state",
		Value: map[string]interface{}{
			"mode":         bc.lastMode,
			"current_temp": bc.lastCurrentTemp,
			"set_temp":     bc.lastSetTemp,
		},
	}

	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("보일러 %d 상태 발행 오류: %v", bc.boilerNumber, err)
		return
	}

	bc.mqttClient.Publish(topic, 0, false, payload)
	log.Printf("보일러 %d 발행: 모드=%s, 현재=%d, 설정=%d",
		bc.boilerNumber, bc.lastMode, bc.lastCurrentTemp, bc.lastSetTemp)
}

func (bc *BoilerController) handleCommand(commandType string, value interface{}) {
	bc.manager.mu.Lock()
	defer bc.manager.mu.Unlock()

	switch commandType {
	case "mode":
		bc.doSetMode(fmt.Sprintf("%v", value))
	case "set_temp":
		if temp, ok := value.(float64); ok {
			bc.doSetTemperature(int(temp))
		} else if temp, ok := value.(int); ok {
			bc.doSetTemperature(temp)
		} else if tempStr, ok := value.(string); ok {
			if temp, err := strconv.Atoi(tempStr); err == nil {
				bc.doSetTemperature(temp)
			}
		}
	default:
		log.Printf("보일러 %d 알 수 없는 명령: %s", bc.boilerNumber, commandType)
	}
}

func (bc *BoilerController) doSetMode(mode string) {
	var packet string
	switch mode {
	case "off":
		packet = BOILER_MODE_SET + "00"
	case "heat":
		packet = BOILER_MODE_SET + "01"
	case "cool":
		packet = BOILER_MODE_SET + "02"
	default:
		log.Printf("보일러 %d 알 수 없는 모드: %s", bc.boilerNumber, mode)
		return
	}

	dummy := make([]byte, 256)
	bc.manager.port.Read(dummy)

	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("보일러 %d 모드 설정 실패: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 모드 설정 전송 안됨: %v", bc.boilerNumber, err)
		return
	}

	log.Printf("보일러 %d 모드 정상 설정: %s", bc.boilerNumber, mode)
	time.Sleep(100 * time.Millisecond)
	bc.doQueryMode()
}

func (bc *BoilerController) doSetTemperature(temp int) {
	tempHex := fmt.Sprintf("%02X", temp)
	packet := BOILER_TEMP_SET + tempHex

	dummy := make([]byte, 256)
	bc.manager.port.Read(dummy)

	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("보일러 %d 온도 설정 전송 불가: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.manager.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 온도 명령 실패: %v", bc.boilerNumber, err)
		return
	}

	log.Printf("보일러 %d 온도 정상 설정: %d", bc.boilerNumber, temp)
	time.Sleep(100 * time.Millisecond)
	bc.doQueryTemperature()
}
