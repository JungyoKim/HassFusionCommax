package mqtt

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// DoorOpenCommand represents a door open command from HA
type DoorOpenCommand struct {
	Floor string // "B4", "B3", "1F"
}

// SendDoorOpen sends SOAP request to open door for specified floor
func SendDoorOpen(floor string) error {
	// Floor to IP mapping
	var targetIP string
	switch floor {
	case "B4":
		targetIP = "10.9.1.37:29752"
	case "B3":
		targetIP = "10.9.1.27:29752"
	case "1F":
		targetIP = "10.9.1.17:29752"
	default:
		return fmt.Errorf("unknown floor: %s", floor)
	}

	// SOAP XML payload
	soapBody := `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/"><v:Header /><v:Body><n0:setOutOfBandDoorOpen id="o0" c:root="1" xmlns:n0="urn:clbs"><in i:type="d:int">15</in></n0:setOutOfBandDoorOpen></v:Body></v:Envelope>`

	url := fmt.Sprintf("http://%s/", targetIP)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(soapBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "kSOAP/2.0")
	req.Header.Set("SOAPAction", "")
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Connection", "close")
	req.Header.Set("Host", targetIP)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	log.Printf("[DOOR] %s층 문 열기 요청 전송 중... (target: %s)", floor, targetIP)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		log.Printf("[DOOR] %s층 문 열기 성공 (응답: %d bytes)", floor, len(body))
		return nil
	}

	return fmt.Errorf("door open failed: status %d", resp.StatusCode)
}


