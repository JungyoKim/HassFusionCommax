package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client        mqtt.Client
	topic         string
	lastPublished map[string]time.Time // 차량별 마지막 발행 시간
	minInterval   time.Duration        // 최소 발행 간격 (중복 방지)
}

type ParkEvent struct {
	EventType   string    `json:"event_type"` // "parkIn" or "parkOut"
	CarNo       string    `json:"car_no"`
	Timestamp   time.Time `json:"timestamp"`
	Direction   string    `json:"direction"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
}

// NewClient creates a new MQTT client
func NewClient(broker, clientID, username, password, topic string) (*Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)

	// Client ID가 비어있으면 자동으로 랜덤 ID 생성 (충돌 방지)
	if clientID == "" {
		clientID = fmt.Sprintf("hasstcp_%d", time.Now().UnixNano())
	}
	opts.SetClientID(clientID)

	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	// 연결 안정성 설정 (이벤트 발행 전용)
	opts.SetCleanSession(true)                     // 세션 정리 (HA 재시작 시 중복 방지)
	opts.SetAutoReconnect(true)                    // 자동 재연결 활성화
	opts.SetConnectRetry(true)                     // 연결 재시도 활성화
	opts.SetConnectRetryInterval(10 * time.Second) // 재연결 간격
	opts.SetMaxReconnectInterval(2 * time.Minute)  // 최대 재연결 간격
	opts.SetOrderMatters(false)                    // 메시지 순서 무시로 성능 향상
	opts.SetKeepAlive(60 * time.Second)            // Keep-Alive 간격
	opts.SetPingTimeout(20 * time.Second)          // Ping 타임아웃
	opts.SetConnectTimeout(60 * time.Second)       // 연결 타임아웃

	mqttClientInstance := &Client{
		client:        nil, // 아래에서 설정
		topic:         topic,
		lastPublished: make(map[string]time.Time),
		minInterval:   5 * time.Second,
	}

	opts.OnConnect = func(c mqtt.Client) {
		log.Printf("MQTT connected to %s", broker)
		// 버튼 명령 구독
		mqttClientInstance.setupButtonSubscriptions()
	}
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v (auto-reconnecting...)", err)
	}
	opts.OnReconnecting = func(c mqtt.Client, opts *mqtt.ClientOptions) {
		log.Printf("MQTT reconnecting...")
	}

	client := mqtt.NewClient(opts)

	// 연결 시도 전 대기
	time.Sleep(1 * time.Second)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("mqtt connect: %w", token.Error())
	}

	mqttClientInstance.client = client
	return mqttClientInstance, nil
}

// PublishParkEvent publishes a parking event to MQTT
func (c *Client) PublishParkEvent(event ParkEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Publish to main topic
	token := c.client.Publish(c.topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("mqtt publish: %w", token.Error())
	}

	// Publish to car-specific event topic (only for parkIn events)
	if event.EventType == "parkIn" {
		// 중복 방지: 최근 발행 시간 확인
		lastTime, exists := c.lastPublished[event.CarNo]
		now := time.Now()

		if exists && now.Sub(lastTime) < c.minInterval {
			log.Printf("MQTT Event 중복 방지: %s (마지막 발행 %.1f초 전)",
				event.CarNo, now.Sub(lastTime).Seconds())
			return nil
		}

		carTopic := fmt.Sprintf("%s/%s", c.topic, event.CarNo)
		// MQTT Event 형식: JSON with event_type
		eventPayload := map[string]interface{}{
			"event_type": "parkIn",
			"car_no":     event.CarNo,
			"timestamp":  event.Timestamp.Format("2006-01-02T15:04:05-07:00"),
		}
		eventJSON, _ := json.Marshal(eventPayload)
		token = c.client.Publish(carTopic, 0, false, string(eventJSON))
		token.Wait()

		// 마지막 발행 시간 기록
		c.lastPublished[event.CarNo] = now

		log.Printf("MQTT Event published: parkIn -> %s (topic: %s)", event.CarNo, carTopic)
	}

	return nil
}

// setupButtonSubscriptions subscribes to door open button commands from HA
func (c *Client) setupButtonSubscriptions() {
	floors := []string{"B4", "B3", "1F"}

	for _, floor := range floors {
		floorCopy := floor // 클로저 캡처용
		topic := fmt.Sprintf("%s/door/%s/set", c.topic, floor)
		token := c.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			c.handleDoorOpenCommand(floorCopy, msg.Payload())
		})
		token.Wait()
		if token.Error() != nil {
			log.Printf("MQTT subscribe failed (%s): %v", topic, token.Error())
		} else {
			log.Printf("MQTT subscribed: %s", topic)
		}
	}
}

// handleDoorOpenCommand handles door open button press from HA
func (c *Client) handleDoorOpenCommand(floor string, payload []byte) {
	log.Printf("[DOOR] %s층 문 열기 명령 수신: %s", floor, string(payload))

	// 버튼은 "PRESS" 페이로드를 보냄
	if string(payload) == "PRESS" {
		if err := SendDoorOpen(floor); err != nil {
			log.Printf("[DOOR] %s층 문 열기 실패: %v", floor, err)
		}
	}
}

// Close disconnects the MQTT client
func (c *Client) Close() {
	c.client.Disconnect(250)
}
