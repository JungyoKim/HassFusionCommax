package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// MQTT 테스트 클라이언트
type MQTTTestClient struct {
	client MQTT.Client
}

// MQTT 메시지 구조체
type MQTTMessage struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// 테스트 클라이언트 생성
func NewMQTTTestClient() *MQTTTestClient {
	opts := MQTT.NewClientOptions()
	opts.AddBroker("localhost:1883")
	opts.SetClientID("hass485_test_client")
	opts.SetAutoReconnect(true)

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT 연결 실패: %v", token.Error())
	}

	return &MQTTTestClient{client: client}
}

// 조명 제어
func (tc *MQTTTestClient) ControlLight(lightNumber int, command string) {
	topic := fmt.Sprintf("hass485/lights/%d/command", lightNumber)
	message := MQTTMessage{
		Type:  command,
		Value: command,
	}

	payload, _ := json.Marshal(message)
	tc.client.Publish(topic, 0, false, payload)
	log.Printf("조명 %d %s 명령 전송", lightNumber, command)
}

// 보일러 제어
func (tc *MQTTTestClient) ControlBoiler(boilerNumber int, command string, value interface{}) {
	topic := fmt.Sprintf("hass485/boilers/%d/command", boilerNumber)
	message := MQTTMessage{
		Type:  command,
		Value: value,
	}

	payload, _ := json.Marshal(message)
	tc.client.Publish(topic, 0, false, payload)
	log.Printf("보일러 %d %s 명령 전송", boilerNumber, command)
}

// 도어 제어
func (tc *MQTTTestClient) ControlDoor(command string) {
	message := MQTTMessage{
		Type:  command,
		Value: command,
	}

	payload, _ := json.Marshal(message)
	tc.client.Publish("hass485/door/command", 0, false, payload)
	log.Printf("도어 %s 명령 전송", command)
}

// 엘리베이터 제어
func (tc *MQTTTestClient) ControlElevator(command string) {
	message := MQTTMessage{
		Type:  command,
		Value: command,
	}

	payload, _ := json.Marshal(message)
	tc.client.Publish("hass485/elevator/command", 0, false, payload)
	log.Printf("엘리베이터 %s 명령 전송", command)
}

// 일괄소등 제어
func (tc *MQTTTestClient) ControlAllOff(command string) {
	message := MQTTMessage{
		Type:  command,
		Value: command,
	}

	payload, _ := json.Marshal(message)
	tc.client.Publish("hass485/alloff/command", 0, false, payload)
	log.Printf("일괄소등 %s 명령 전송", command)
}

// 상태 모니터링
func (tc *MQTTTestClient) MonitorStates() {
	// 모든 상태 토픽 구독
	topics := []string{
		"hass485/lights/+/state",
		"hass485/boilers/+/state",
		"hass485/doorbell/state",
		"hass485/alloff/state",
	}

	for _, topic := range topics {
		tc.client.Subscribe(topic, 0, func(client MQTT.Client, msg MQTT.Message) {
			log.Printf("상태 수신 [%s]: %s", msg.Topic(), string(msg.Payload()))
		})
	}

	log.Println("상태 모니터링 시작...")
}

// 정리
func (tc *MQTTTestClient) Close() {
	tc.client.Disconnect(250)
}

// 메인 함수
func main() {
	if len(os.Args) < 2 {
		fmt.Println("사용법:")
		fmt.Println("  ./mqtt_test_client light-on <번호>")
		fmt.Println("  ./mqtt_test_client light-off <번호>")
		fmt.Println("  ./mqtt_test_client boiler-mode <번호> <mode>")
		fmt.Println("  ./mqtt_test_client boiler-temp <번호> <temperature>")
		fmt.Println("  ./mqtt_test_client door-open")
		fmt.Println("  ./mqtt_test_client elevator-call")
		fmt.Println("  ./mqtt_test_client alloff-on")
		fmt.Println("  ./mqtt_test_client alloff-off")
		fmt.Println("  ./mqtt_test_client monitor")
		return
	}

	client := NewMQTTTestClient()
	defer client.Close()

	command := os.Args[1]

	switch command {
	case "light-on":
		if len(os.Args) < 3 {
			fmt.Println("조명 번호를 지정하세요")
			return
		}
		lightNumber := 1 // 기본값
		fmt.Sscanf(os.Args[2], "%d", &lightNumber)
		client.ControlLight(lightNumber, "on")

	case "light-off":
		if len(os.Args) < 3 {
			fmt.Println("조명 번호를 지정하세요")
			return
		}
		lightNumber := 1
		fmt.Sscanf(os.Args[2], "%d", &lightNumber)
		client.ControlLight(lightNumber, "off")

	case "boiler-mode":
		if len(os.Args) < 4 {
			fmt.Println("보일러 번호와 모드를 지정하세요")
			return
		}
		boilerNumber := 1
		mode := os.Args[3]
		fmt.Sscanf(os.Args[2], "%d", &boilerNumber)
		client.ControlBoiler(boilerNumber, "mode", mode)

	case "boiler-temp":
		if len(os.Args) < 4 {
			fmt.Println("보일러 번호와 온도를 지정하세요")
			return
		}
		boilerNumber := 1
		temperature := 20
		fmt.Sscanf(os.Args[2], "%d", &boilerNumber)
		fmt.Sscanf(os.Args[3], "%d", &temperature)
		client.ControlBoiler(boilerNumber, "set_temp", temperature)

	case "door-open":
		client.ControlDoor("open")

	case "elevator-call":
		client.ControlElevator("call")

	case "alloff-on":
		client.ControlAllOff("on")

	case "alloff-off":
		client.ControlAllOff("off")

	case "monitor":
		client.MonitorStates()
		// 무한 대기
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

	default:
		fmt.Printf("알 수 없는 명령: %s\n", command)
	}
}
