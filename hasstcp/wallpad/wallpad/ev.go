// package main

// import (
// 	"bytes"
// 	"encoding/json"
// 	"encoding/xml"
// 	"fmt"
// 	"io/ioutil"
// 	"net/http"
// 	"os"
// 	"time"

// 	MQTT "github.com/eclipse/paho.mqtt.golang"
// )

// const (
// 	soapURL    = "http://10.0.0.2:29715"
// 	mqttBroker = "tcp://192.168.0.15:1883"
// )

// var (
// 	soapBody = `
// <SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns="urn:ces">
// <SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
// <ns:getEvStatus><in>1</in></ns:getEvStatus>
// </SOAP-ENV:Body>
// </SOAP-ENV:Envelope>
// `
// )

// type Envelope struct {
// 	XMLName xml.Name `xml:"Envelope"`
// 	Body    Body     `xml:"Body"`
// }

// type Body struct {
// 	GetEvStatusResponse GetEvStatusResponse `xml:"getEvStatusResponse"`
// }

// type GetEvStatusResponse struct {
// 	Return Out `xml:"return"`
// }

// type Out struct {
// 	Items []Item `xml:"item"`
// }

// type Item struct {
// 	CarFloor     string `xml:"carFloor"`
// 	IsBasement   string `xml:"isBasement"`
// 	CarDirection string `xml:"carDirection"`
// 	EvStatus     string `xml:"evStatus"`
// 	CallUp       string `xml:"callUp"`
// 	CallDown     string `xml:"callDown"`
// }

// func getElevatorStatus() ([]Item, error) {
// 	req, err := http.NewRequest("POST", soapURL, bytes.NewBuffer([]byte(soapBody)))
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
// 	req.Header.Set("SOAPAction", "urn:ces#getEvStatus")

// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()
// 	body, _ := ioutil.ReadAll(resp.Body)

// 	fmt.Println("----- SOAP 응답 원본 -----")
// 	fmt.Println(string(body))
// 	fmt.Println("-------------------------")

// 	var env Envelope
// 	if err := xml.Unmarshal(body, &env); err != nil {
// 		fmt.Println("XML 파싱 실패:", err)
// 		return nil, err
// 	}

// 	fmt.Println("----- 파싱된 엘리베이터 정보 -----")
// 	for i, item := range env.Body.GetEvStatusResponse.Return.Items {
// 		fmt.Printf("엘리베이터 %d: %+v\n", i+1, item)
// 	}
// 	fmt.Println("-------------------------------")

// 	return env.Body.GetEvStatusResponse.Return.Items, nil
// }

// func main() {
// 	opts := MQTT.NewClientOptions().AddBroker(mqttBroker)
// 	opts.SetClientID("elevator-monitor-debug")
// 	client := MQTT.NewClient(opts)
// 	if token := client.Connect(); token.Wait() && token.Error() != nil {
// 		fmt.Println("MQTT 연결 실패:", token.Error())
// 		os.Exit(1)
// 	}
// 	fmt.Println("MQTT 연결 성공")

// 	for {
// 		items, err := getElevatorStatus()
// 		if err != nil {
// 			fmt.Println("엘리베이터 상태 조회 실패:", err)
// 			time.Sleep(3 * time.Second)
// 			continue
// 		}
// 		for idx, item := range items {
// 			status := map[string]string{
// 				"floor":       item.CarFloor,
// 				"is_basement": item.IsBasement,
// 				"direction":   item.CarDirection,
// 				"status":      item.EvStatus,
// 				"call_up":     item.CallUp,
// 				"call_down":   item.CallDown,
// 			}
// 			payload, _ := json.Marshal(status)
// 			topic := fmt.Sprintf("home/elevator/%d/status", idx+1)
// 			fmt.Printf("MQTT Publish → 토픽: %s, 페이로드: %s\n", topic, string(payload))
// 			token := client.Publish(topic, 0, false, payload)
// 			token.Wait()
// 			if token.Error() != nil {
// 				fmt.Println("MQTT Publish 에러:", token.Error())
// 			}
// 		}
// 		time.Sleep(2 * time.Second)
// 	}
// }
