package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	b4URL      = "http://10.9.1.37:29752/"
	b3URL      = "http://10.9.1.27:29752/"
	f1URL      = "http://10.9.1.17:29752/"
	mqttBroker = "tcp://192.168.0.15:1883"
	topicB4    = "wallpad/door/b4open"
	topicB3    = "wallpad/door/b3open"
	topicF1    = "wallpad/door/f1open"
)

var soapBody = `
<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/"><v:Header /><v:Body><n0:setOutOfBandDoorOpen id="o0" c:root="1" xmlns:n0="urn:clbs"><in i:type="d:int">15</in></n0:setOutOfBandDoorOpen></v:Body></v:Envelope>
`

func openDoor(soapURL string) {
	req, err := http.NewRequest("POST", soapURL, bytes.NewBuffer([]byte(soapBody)))
	if err != nil {
		fmt.Println("Request 생성 오류:", err)
		return
	}
	req.Header.Set("User-Agent", "kSOAP/2.0")
	req.Header.Set("SOAPAction", "")
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Connection", "close")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(soapBody)))
	req.Header.Set("Accept", "*, */*")
	req.Header.Set("Host", soapURL[7:]) // Host 헤더 자동 세팅

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("요청 오류:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("응답:", string(body))
}

func main() {
	opts := MQTT.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("wallpad-door-opener")
	opts.AutoReconnect = true
	opts.ConnectRetry = true
	opts.ConnectRetryInterval = 3 * time.Second
	opts.OnConnectionLost = func(client MQTT.Client, err error) {
		fmt.Println("MQTT 연결 끊김:", err)
	}
	opts.OnConnect = func(client MQTT.Client) {
		fmt.Println("MQTT 재연결 성공")
		// 재연결 시 반드시 다시 subscribe!
		if token := client.Subscribe(topicB4, 0, func(client MQTT.Client, msg MQTT.Message) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("콜백 패닉 발생:", r)
				}
			}()
			fmt.Println("지하4층 문 열기 명령 수신!")
			openDoor(b4URL)
		}); token.Wait() && token.Error() != nil {
			fmt.Println("B4 구독 실패:", token.Error())
		}
		if token := client.Subscribe(topicB3, 0, func(client MQTT.Client, msg MQTT.Message) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("콜백 패닉 발생:", r)
				}
			}()
			fmt.Println("지하3층 문 열기 명령 수신!")
			openDoor(b3URL)
		}); token.Wait() && token.Error() != nil {
			fmt.Println("B3 구독 실패:", token.Error())
		}
		if token := client.Subscribe(topicF1, 0, func(client MQTT.Client, msg MQTT.Message) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println("콜백 패닉 발생:", r)
				}
			}()
			fmt.Println("1층 문 열기 명령 수신!")
			openDoor(f1URL)
		}); token.Wait() && token.Error() != nil {
			fmt.Println("F1 구독 실패:", token.Error())
		}
	}

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println("MQTT 연결 실패:", token.Error())
		os.Exit(1)
	}
	fmt.Println("MQTT 연결 성공")

	select {} // 프로그램이 종료되지 않게 대기
}
