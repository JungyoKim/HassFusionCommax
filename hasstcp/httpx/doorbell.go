package httpx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"hasstcp/mqtt"
)

// detectDoorbell checks if the HTTP payload contains SOAP pushVList (doorbell) and publishes to MQTT
func detectDoorbell(httpData []byte, dir, src, dst string, isRequest bool) {
	// Extract body after HTTP headers
	idx := bytes.Index(httpData, []byte("\r\n\r\n"))
	if idx == -1 {
		return
	}
	body := httpData[idx+4:]
	if len(body) == 0 {
		return
	}

	// Check if it's pushVList SOAP
	if !bytes.Contains(body, []byte("pushVList")) {
		return
	}

	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return
	}

	if isRequest && env.Body.PushVList != nil {
		// 층 구별: Host 헤더 또는 dst에서 IP 추출
		floor := getFloorFromDst(dst)
		if floor == "" {
			return
		}

		// 중복 방지: 5초 이내 같은 층 이벤트 무시
		now := time.Now().Unix()
		if lastTime, exists := lastDoorbellEvents[floor]; exists && (now-lastTime) < 5 {
			fmt.Printf("[중복 무시] %s 벨 이벤트 (%.1f초 전)\n", floor, float64(now-lastTime))
			return
		}

		// 새로운 이벤트로 기록
		lastDoorbellEvents[floor] = now

		fmt.Printf("\n🔔 [벨 울림 감지] 층: %s | %s -> %s (%s)\n\n", floor, src, dst, dir)

		// Publish to MQTT if client is configured
		if mqttClient != nil {
			event := mqtt.ParkEvent{
				EventType:   "doorbell",
				CarNo:       floor, // floor를 CarNo 필드에 재사용
				Timestamp:   time.Now(),
				Direction:   dir,
				Source:      src,
				Destination: dst,
			}
			if err := mqttClient.PublishParkEvent(event); err != nil {
				fmt.Printf("MQTT publish error: %v\n", err)
			}
		}
	}
}

// getFloorFromDst extracts floor name from destination address
func getFloorFromDst(dst string) string {
	// dst format: "10.9.1.37:29720"
	parts := strings.Split(dst, ":")
	if len(parts) != 2 {
		return ""
	}
	ip := parts[0]

	switch ip {
	case "10.9.1.37":
		return "B4"
	case "10.9.1.27":
		return "B3"
	case "10.9.1.17":
		return "1F"
	default:
		return ""
	}
}
