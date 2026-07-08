package httpx

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"hassfusion/config"
)

// [개선] 매번 생성하지 않도록 전역 HTTP 클라이언트 선언
// Connection: close 헤더를 사용하더라도 클라이언트 객체 자체는 재사용하는 것이 좋습니다.
var doorHttpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// OpenDoor sends a SOAP request to open the door for a given floor
func OpenDoor(floor string, cfg *config.Config) error {
	var targetIP string
	switch floor {
	case "B4":
		targetIP = cfg.Doors.FloorB4IP
	case "B3":
		targetIP = cfg.Doors.FloorB3IP
	case "1F":
		targetIP = cfg.Doors.Floor1FIP
	default:
		return fmt.Errorf("unknown floor: %s", floor)
	}

	if targetIP == "" {
		return fmt.Errorf("ip not configured for floor %s", floor)
	}

	// SOAP XML payload matching original trace
	soapBody := `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/"><v:Header /><v:Body><n0:setOutOfBandDoorOpen id="o0" c:root="1" xmlns:n0="urn:clbs"><in i:type="d:int">15</in></n0:setOutOfBandDoorOpen></v:Body></v:Envelope>`

	url := fmt.Sprintf("http://%s/", targetIP)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(soapBody))
	if err != nil {
		return fmt.Errorf("create open request: %w", err)
	}

	req.Header.Set("User-Agent", "kSOAP/2.0")
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Connection", "close")
	req.Host = targetIP // net/http ignores a "Host" header; must set req.Host

	// 참고: 보통 SOAP 요청은 SOAPAction 헤더를 요구하는 경우가 많습니다.
	// 만약 테스트해 보시고 문이 안 열린다면 req.Header.Set("SOAPAction", "urn:clbs#setOutOfBandDoorOpen") 같은 헤더가 빠져서일 수 있습니다.

	log.Printf("[DOOR] Opening %s floor door...", floor)

	// [개선] 전역 클라이언트 재사용
	resp, err := doorHttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("door open http req err: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		io.ReadAll(resp.Body) // Drain
		log.Printf("[DOOR] %s floor open SUCCESS", floor)
		return nil
	}

	return fmt.Errorf("door open status %d", resp.StatusCode)
}
