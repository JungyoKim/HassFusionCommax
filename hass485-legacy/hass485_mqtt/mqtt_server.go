package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// MQTT 설정
const (
	MQTT_BROKER    = "192.168.10.15:1883"
	MQTT_CLIENT_ID = "hass485_server"
	MQTT_USERNAME  = ""
	MQTT_PASSWORD  = ""
)

// MQTT 토픽 정의
const (
	// 상태 발행 토픽
	TOPIC_LIGHT_STATE    = "hass485/lights/+/state"
	TOPIC_BOILER_STATE   = "hass485/boilers/+/state"
	TOPIC_DOORBELL_STATE = "hass485/doorbell/state"
	TOPIC_ALL_OFF_STATE  = "hass485/alloff/state"

	// 제어 명령 토픽
	TOPIC_LIGHT_COMMAND   = "hass485/lights/+/command"
	TOPIC_BOILER_COMMAND  = "hass485/boilers/+/command"
	TOPIC_DOOR_COMMAND    = "hass485/door/command"
	TOPIC_ALL_OFF_COMMAND = "hass485/alloff/command"
)

// MQTT 메시지 구조체
type MQTTMessage struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// MQTT 클라이언트
var mqttClient MQTT.Client

// 메인 함수
func main() {
	// MQTT 클라이언트 설정
	opts := MQTT.NewClientOptions()
	opts.AddBroker(MQTT_BROKER)
	opts.SetClientID(MQTT_CLIENT_ID)
	opts.SetUsername(MQTT_USERNAME)
	opts.SetPassword(MQTT_PASSWORD)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	// 연결 콜백
	opts.SetOnConnectHandler(func(client MQTT.Client) {
		log.Println("MQTT 브로커에 연결되었습니다")

		// 구독 설정
		subscribeToTopics()
	})

	// 연결 끊김 콜백
	opts.SetConnectionLostHandler(func(client MQTT.Client, err error) {
		log.Printf("MQTT 연결이 끊어졌습니다: %v", err)
	})

	// MQTT 클라이언트 생성 및 연결
	mqttClient = MQTT.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT 연결 실패: %v", token.Error())
	}

	// 디바이스 컨트롤러 시작
	startDeviceControllers()

	// 시그널 핸들링
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("서버를 종료합니다...")
	mqttClient.Disconnect(250)
}

// 토픽 구독
func subscribeToTopics() {
	// 조명 제어 명령 구독
	mqttClient.Subscribe("hass485/lights/+/command", 0, handleLightCommand)

	// 보일러 제어 명령 구독
	mqttClient.Subscribe("hass485/boilers/+/command", 0, handleBoilerCommand)

	// 도어 제어 명령 구독
	mqttClient.Subscribe("hass485/door/command", 0, handleDoorCommand)

	// 일괄소등 제어 명령 구독
	mqttClient.Subscribe("hass485/alloff/command", 0, handleAllOffCommand)

	log.Println("모든 토픽을 구독했습니다")
}

// 조명 제어 명령 처리
func handleLightCommand(client MQTT.Client, msg MQTT.Message) {
	var command MQTTMessage
	if err := json.Unmarshal(msg.Payload(), &command); err != nil {
		log.Printf("조명 명령 파싱 오류: %v", err)
		return
	}

	// 토픽에서 조명 번호 추출
	lightNumber := extractNumberFromTopic(msg.Topic())

	log.Printf("조명 %d 명령 수신: %s", lightNumber, command.Type)

	// 실제 조명 제어 로직 (기존 RS485 코드 사용)
	handleLightControl(lightNumber, command.Type, command.Value)
}

// 보일러 제어 명령 처리
func handleBoilerCommand(client MQTT.Client, msg MQTT.Message) {
	var command MQTTMessage
	if err := json.Unmarshal(msg.Payload(), &command); err != nil {
		log.Printf("보일러 명령 파싱 오류: %v", err)
		return
	}

	boilerNumber := extractNumberFromTopic(msg.Topic())
	log.Printf("보일러 %d 명령 수신: %s", boilerNumber, command.Type)

	handleBoilerControl(boilerNumber, command.Type, command.Value)
}

// 도어 제어 명령 처리
func handleDoorCommand(client MQTT.Client, msg MQTT.Message) {
	var command MQTTMessage
	if err := json.Unmarshal(msg.Payload(), &command); err != nil {
		log.Printf("도어 명령 파싱 오류: %v", err)
		return
	}

	log.Printf("도어 명령 수신: %s", command.Type)
	handleDoorControl(command.Type)
}

// 일괄소등 제어 명령 처리
func handleAllOffCommand(client MQTT.Client, msg MQTT.Message) {
	var command MQTTMessage
	if err := json.Unmarshal(msg.Payload(), &command); err != nil {
		log.Printf("일괄소등 명령 파싱 오류: %v", err)
		return
	}

	log.Printf("일괄소등 명령 수신: %s", command.Type)
	handleAllOffControl(command.Type)
}

// 토픽에서 번호 추출
func extractNumberFromTopic(topic string) int {
	// "hass485/lights/1/command" -> 1
	parts := strings.Split(topic, "/")
	if len(parts) >= 3 {
		if num, err := strconv.Atoi(parts[2]); err == nil {
			return num
		}
	}
	return 1 // 기본값
}

// 상태 발행 함수들
func publishLightState(lightNumber int, state string) {
	topic := fmt.Sprintf("hass485/lights/%d/state", lightNumber)
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}

	payload, _ := json.Marshal(message)
	mqttClient.Publish(topic, 0, false, payload)
}

func publishBoilerState(boilerNumber int, mode string, currentTemp, setTemp int) {
	topic := fmt.Sprintf("hass485/boilers/%d/state", boilerNumber)
	message := MQTTMessage{
		Type: "state",
		Value: map[string]interface{}{
			"mode":         mode,
			"current_temp": currentTemp,
			"set_temp":     setTemp,
		},
	}

	payload, _ := json.Marshal(message)
	mqttClient.Publish(topic, 0, false, payload)
}

func publishDoorbellState(state string) {
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}

	payload, _ := json.Marshal(message)
	mqttClient.Publish("hass485/doorbell/state", 0, false, payload)
}

func publishAllOffState(state string) {
	message := MQTTMessage{
		Type:  "state",
		Value: state,
	}

	payload, _ := json.Marshal(message)
	mqttClient.Publish("hass485/alloff/state", 0, false, payload)
}

// 디바이스 매니저 및 컨트롤러들
var lightManager *LightManager
var boilerManager *BoilerManager
var doorbellController *DoorbellController
var alloffController *AlloffController

// 디바이스 컨트롤러 시작 (기존 코드와 통합)
func startDeviceControllers() {
	log.Println("디바이스 컨트롤러를 시작합니다...")

	// 조명 매니저 시작
	lightManager = NewLightManager("/dev/ttyUSB0", mqttClient)
	if lightManager != nil {
		log.Printf("조명 매니저 시작됨")
	}

	// 보일러 매니저 시작
	boilerManager = NewBoilerManager("/dev/ttyUSB1", mqttClient)
	if boilerManager != nil {
		log.Printf("보일러 매니저 시작됨")
	}

	// 도어벨 컨트롤러 시작
	doorbellController = NewDoorbellController("/dev/ttyUSB2", mqttClient)
	if doorbellController != nil {
		log.Printf("도어벨 컨트롤러 시작됨")
	}

	// 일괄소등 컨트롤러 시작
	alloffController = NewAlloffController("/dev/ttyUSB3", mqttClient)
	if alloffController != nil {
		log.Printf("일괄소등 컨트롤러 시작됨")
	}
}

// 기존 RS485 제어 함수들 (기존 코드에서 가져와서 수정)
func handleLightControl(lightNumber int, commandType string, value interface{}) {
	if lightManager != nil {
		lightManager.HandleCommand(lightNumber, commandType, value)
	} else {
		log.Printf("조명 매니저가 초기화되지 않았습니다")
	}
}

func handleBoilerControl(boilerNumber int, commandType string, value interface{}) {
	if boilerManager != nil {
		boilerManager.HandleCommand(boilerNumber, commandType, value)
	} else {
		log.Printf("보일러 매니저가 초기화되지 않았습니다")
	}
}

func handleDoorControl(commandType string) {
	if doorbellController != nil {
		doorbellController.handleCommand(commandType, nil)
	} else {
		log.Printf("도어벨 컨트롤러를 찾을 수 없습니다")
	}
}

func handleAllOffControl(commandType string) {
	if alloffController != nil {
		alloffController.handleAllOffCommand(commandType, nil)
	} else {
		log.Printf("일괄소등 컨트롤러를 찾을 수 없습니다")
	}
}
