package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tarm/serial"
)

// BoilerController MQTT 버전
type BoilerController struct {
	port            *serial.Port
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

// BoilerController 생성
func NewBoilerController(devicePath string, boilerNumber int, mqttClient MQTT.Client) *BoilerController {
	config := &serial.Config{
		Name: devicePath,
		Baud: 9600,
	}

	port, err := serial.OpenPort(config)
	if err != nil {
		log.Printf("보일러 %d 시리얼 포트 열기 실패: %v", boilerNumber, err)
		return nil
	}

	controller := &BoilerController{
		port:            port,
		boilerNumber:    boilerNumber,
		mqttClient:      mqttClient,
		lastMode:        "off",
		lastCurrentTemp: 20,
		lastSetTemp:     20,
	}

	// 상태 모니터링 시작
	go controller.statusQueryLoop()

	return controller
}

// 상태 쿼리 루프
func (bc *BoilerController) statusQueryLoop() {
	ticker := time.NewTicker(10 * time.Second) // 보일러는 10초마다 쿼리
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bc.queryMode()
			bc.queryTemperature()
		}
	}
}

// 모드 쿼리
func (bc *BoilerController) queryMode() {
	queryPacket := BOILER_MODE_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("보일러 %d 모드 쿼리 패킷 생성 실패: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 모드 쿼리 전송 실패: %v", bc.boilerNumber, err)
		return
	}

	// 응답 읽기
	response := make([]byte, 32)
	n, err := bc.port.Read(response)
	if err != nil {
		log.Printf("보일러 %d 모드 응답 읽기 실패: %v", bc.boilerNumber, err)
		return
	}

	// 응답 파싱
	mode := bc.parseModeResponse(response[:n])

	// 모드가 변경되었으면 MQTT로 발행
	if mode != bc.lastMode {
		bc.publishState()
		bc.lastMode = mode
	}
}

// 온도 쿼리
func (bc *BoilerController) queryTemperature() {
	queryPacket := BOILER_TEMP_QUERY
	packetBytes, err := hex.DecodeString(queryPacket)
	if err != nil {
		log.Printf("보일러 %d 온도 쿼리 패킷 생성 실패: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 온도 쿼리 전송 실패: %v", bc.boilerNumber, err)
		return
	}

	// 응답 읽기
	response := make([]byte, 32)
	n, err := bc.port.Read(response)
	if err != nil {
		log.Printf("보일러 %d 온도 응답 읽기 실패: %v", bc.boilerNumber, err)
		return
	}

	// 응답 파싱
	currentTemp, setTemp := bc.parseTemperatureResponse(response[:n])

	// 온도가 변경되었으면 MQTT로 발행
	if currentTemp != bc.lastCurrentTemp || setTemp != bc.lastSetTemp {
		bc.lastCurrentTemp = currentTemp
		bc.lastSetTemp = setTemp
		bc.publishState()
	}
}

// 모드 응답 파싱
func (bc *BoilerController) parseModeResponse(response []byte) string {
	// 실제 구현에서는 응답 형식에 맞게 수정
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

// 온도 응답 파싱
func (bc *BoilerController) parseTemperatureResponse(response []byte) (currentTemp, setTemp int) {
	// 실제 구현에서는 응답 형식에 맞게 수정
	if len(response) >= 2 {
		currentTemp = int(response[0])
		setTemp = int(response[1])
	} else {
		currentTemp = 20
		setTemp = 20
	}
	return
}

// MQTT로 상태 발행
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
		log.Printf("보일러 %d 상태 발행 실패: %v", bc.boilerNumber, err)
		return
	}

	bc.mqttClient.Publish(topic, 0, false, payload)
	log.Printf("보일러 %d 상태 발행: mode=%s, current=%d, set=%d",
		bc.boilerNumber, bc.lastMode, bc.lastCurrentTemp, bc.lastSetTemp)
}

// 보일러 제어
func (bc *BoilerController) handleCommand(commandType string, value interface{}) {
	switch commandType {
	case "mode":
		bc.setMode(fmt.Sprintf("%v", value))
	case "set_temp":
		if temp, ok := value.(float64); ok {
			bc.setTemperature(int(temp))
		} else if temp, ok := value.(int); ok {
			bc.setTemperature(temp)
		} else if tempStr, ok := value.(string); ok {
			if temp, err := strconv.Atoi(tempStr); err == nil {
				bc.setTemperature(temp)
			}
		}
	default:
		log.Printf("보일러 %d 알 수 없는 명령: %s", bc.boilerNumber, commandType)
	}
}

// 모드 설정
func (bc *BoilerController) setMode(mode string) {
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

	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("보일러 %d 모드 설정 패킷 생성 실패: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 모드 설정 전송 실패: %v", bc.boilerNumber, err)
		return
	}

	log.Printf("보일러 %d 모드 설정: %s", bc.boilerNumber, mode)

	// 설정 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	bc.queryMode()
}

// 온도 설정
func (bc *BoilerController) setTemperature(temp int) {
	// 온도를 16진수로 변환
	tempHex := fmt.Sprintf("%02X", temp)
	packet := BOILER_TEMP_SET + tempHex

	packetBytes, err := hex.DecodeString(packet)
	if err != nil {
		log.Printf("보일러 %d 온도 설정 패킷 생성 실패: %v", bc.boilerNumber, err)
		return
	}

	_, err = bc.port.Write(packetBytes)
	if err != nil {
		log.Printf("보일러 %d 온도 설정 전송 실패: %v", bc.boilerNumber, err)
		return
	}

	log.Printf("보일러 %d 온도 설정: %d", bc.boilerNumber, temp)

	// 설정 후 즉시 상태 업데이트
	time.Sleep(100 * time.Millisecond)
	bc.queryTemperature()
}

// 정리
func (bc *BoilerController) Close() {
	if bc.port != nil {
		bc.port.Close()
	}
}
