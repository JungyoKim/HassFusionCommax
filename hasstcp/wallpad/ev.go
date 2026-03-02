package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	soapURL    = "http://10.0.0.2:29715"
	mqttBroker = "tcp://192.168.0.15:1883"
)

const soapBody = `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ces="urn:ces">
   <soapenv:Header/>
   <soapenv:Body>
      <ces:getEvStatus/>
   </soapenv:Body>
</soapenv:Envelope>`

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    Body     `xml:"Body"`
}

type Body struct {
	GetEvStatusResponse GetEvStatusResponse `xml:"getEvStatusResponse"`
}

type GetEvStatusResponse struct {
	Return Out `xml:"return"`
}

type Out struct {
	Items []Item `xml:"item"`
}

type Item struct {
	CarFloor     string `xml:"carFloor"`
	IsBasement   string `xml:"isBasement"`
	CarDirection string `xml:"carDirection"`
	EvStatus     string `xml:"evStatus"`
	CallUp       string `xml:"callUp"`
	CallDown     string `xml:"callDown"`
}

// 엘리베이터 상태를 비교하기 위한 구조체
type ElevatorState struct {
	Floor      string
	IsBasement string
	Direction  string
	Status     string
	CallUp     string
	CallDown   string
}

// 이전 상태를 저장할 맵
var previousStates = make(map[int]ElevatorState)

func getElevatorStatus() ([]Item, error) {
	req, err := http.NewRequest("POST", soapURL, bytes.NewBuffer([]byte(soapBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "urn:ces#getEvStatus")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	fmt.Println("----- SOAP 응답 원본 -----")
	fmt.Println(string(body))
	fmt.Println("-------------------------")

	var env Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		fmt.Println("XML 파싱 실패:", err)
		return nil, err
	}

	fmt.Println("----- 파싱된 엘리베이터 정보 -----")
	for i, item := range env.Body.GetEvStatusResponse.Return.Items {
		fmt.Printf("엘리베이터 %d: %+v\n", i+1, item)
	}
	fmt.Println("-------------------------------")

	return env.Body.GetEvStatusResponse.Return.Items, nil
}

// 상태가 변경되었는지 확인하는 함수
func hasStateChanged(elevatorIndex int, currentState ElevatorState) bool {
	previousState, exists := previousStates[elevatorIndex]
	if !exists {
		// 이전 상태가 없으면 변경된 것으로 간주
		return true
	}

	return previousState.Floor != currentState.Floor ||
		previousState.IsBasement != currentState.IsBasement ||
		previousState.Direction != currentState.Direction ||
		previousState.Status != currentState.Status ||
		previousState.CallUp != currentState.CallUp ||
		previousState.CallDown != currentState.CallDown
}

// 엘리베이터가 정지 상태인지 확인하는 함수
func isElevatorStopped(direction string) bool {
	return direction == "0"
}

func main() {
	opts := MQTT.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("elevator-monitor-debug")
	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("MQTT 연결 실패:", token.Error())
		os.Exit(1)
	}
	fmt.Println("MQTT 연결 성공")

	for {
		items, err := getElevatorStatus()
		if err != nil {
			fmt.Println("엘리베이터 상태 조회 실패:", err)
			time.Sleep(3 * time.Second)
			continue
		}

		// 모든 엘리베이터가 정지 상태인지 확인
		allStopped := true
		for _, item := range items {
			if !isElevatorStopped(item.CarDirection) {
				allStopped = false
				break
			}
		}

		for idx, item := range items {
			currentState := ElevatorState{
				Floor:      item.CarFloor,
				IsBasement: item.IsBasement,
				Direction:  item.CarDirection,
				Status:     item.EvStatus,
				CallUp:     item.CallUp,
				CallDown:   item.CallDown,
			}

			// 상태가 변경되었을 때만 MQTT로 전송
			if hasStateChanged(idx, currentState) {
				status := map[string]string{
					"floor":       item.CarFloor,
					"is_basement": item.IsBasement,
					"direction":   item.CarDirection,
					"status":      item.EvStatus,
					"call_up":     item.CallUp,
					"call_down":   item.CallDown,
				}
				payload, _ := json.Marshal(status)
				topic := fmt.Sprintf("home/elevator/%d/status", idx+1)
				fmt.Printf("MQTT Publish → 토픽: %s, 페이로드: %s\n", topic, string(payload))
				token := client.Publish(topic, 0, false, payload)
				token.Wait()
				if token.Error() != nil {
					fmt.Println("MQTT Publish 에러:", token.Error())
				}

				// 현재 상태를 이전 상태로 저장
				previousStates[idx] = currentState
			}
		}

		// 엘리베이터 상태에 따라 다른 주기로 대기
		if allStopped {
			fmt.Println("모든 엘리베이터가 정지 상태 - 2초 대기")
			time.Sleep(2500 * time.Millisecond)
		} else {
			fmt.Println("엘리베이터가 움직이는 중 - 0.8초 대기")
			time.Sleep(1000 * time.Millisecond)
		}
	}
}
