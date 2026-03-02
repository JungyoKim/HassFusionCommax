package capture

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"hassfusion/config"
	"hassfusion/ws"
)

type EnergyMonitor struct {
	cfg        *config.Config
	wsServer   *ws.Server
	cesURL     string
	httpClient *http.Client

	cacheMu    sync.RWMutex
	lastValues map[string]float64
}

func NewEnergyMonitor(cfg *config.Config, wsServer *ws.Server) *EnergyMonitor {
	if cfg.Wallpad.IP == "" || cfg.Wallpad.Mac == "" {
		log.Println("[ENERGY] Wallpad IP/MAC not configured. Skipping energy monitor.")
		return nil
	}

	return &EnergyMonitor{
		cfg:      cfg,
		wsServer: wsServer,
		cesURL:   fmt.Sprintf("http://%s/ces/ces.php", cfg.Wallpad.IP),
		httpClient: &http.Client{
			Timeout: 10 * time.Second, // 타임아웃 10초 설정
		},
		lastValues: make(map[string]float64),
	}
}

func (em *EnergyMonitor) Run() {
	log.Println("[ENERGY] Monitoring started... (Fetching all time units every 10 minutes)")

	energyTypes := []string{"Electricity", "Gas", "Water"}

	for {
		now := time.Now()

		// 파이썬 로그를 통해 확인된 정확한 월패드 키 포맷들
		yyyy := now.Format("2006")             // 예: 2026
		yyyymm := now.Format("200601")         // 예: 202602
		yyyymmdd := now.Format("20060102")     // 예: 20260224
		yyyymmddhh := now.Format("2006010215") // 예: 2026022409 (15는 24시간제 시간 포맷)

		for _, eType := range energyTypes {
			// ---------------------------------------------------------
			// 1. 월별(이번 달) & 연간 총 사용량
			// 주의: proc_energy_usage가 비정상이므로 연간 데이터(annual_select)에서 추출합니다.
			// ---------------------------------------------------------
			annualData, err := em.fetchEDS(fmt.Sprintf("proc_get_energy_annual_select('%s', '%s', '%s', '%s', '%s', '%s')",
				em.cfg.Wallpad.Mac, em.cfg.Wallpad.DeviceID, em.cfg.Wallpad.Dong, em.cfg.Wallpad.Ho, eType, yyyy))

			if err == nil && len(annualData) > 0 {
				// 1-1. 이번 달 사용량 추출 (예: 키 202602)
				if usage, ok := annualData[yyyymm]; ok {
					em.broadcastSensor(eType, "monthly", usage)
				}

				// 1-2. 연간 총 사용량 계산 (조회된 모든 월의 값을 합산)
				var yearlyTotal float64
				for _, v := range annualData {
					yearlyTotal += v
				}
				em.broadcastSensor(eType, "yearly", yearlyTotal)
			} else if err != nil {
				log.Printf("[ENERGY] %s Annual Fetch Error: %v\n", eType, err)
			}

			// 서버 부하 분산을 위한 미세한 딜레이
			time.Sleep(500 * time.Millisecond)

			// ---------------------------------------------------------
			// 2. 일별(오늘) 사용량
			// ---------------------------------------------------------
			dailyData, err := em.fetchEDS(fmt.Sprintf("proc_get_daily_energy('%s', '%s', '%s', '%s', '%s', '%s')",
				em.cfg.Wallpad.Mac, em.cfg.Wallpad.DeviceID, em.cfg.Wallpad.Dong, em.cfg.Wallpad.Ho, eType, yyyymm))

			if err == nil && len(dailyData) > 0 {
				// 예: 키 20260224
				if usage, ok := dailyData[yyyymmdd]; ok {
					em.broadcastSensor(eType, "daily", usage)
				}
			}

			time.Sleep(500 * time.Millisecond)

			// ---------------------------------------------------------
			// 3. 시간별(현재 시) 사용량
			// ---------------------------------------------------------
			hourData, err := em.fetchEDS(fmt.Sprintf("proc_get_hour_energy('%s', '%s', '%s', '%s', '%s', '%s')",
				em.cfg.Wallpad.Mac, em.cfg.Wallpad.DeviceID, em.cfg.Wallpad.Dong, em.cfg.Wallpad.Ho, eType, yyyymmdd))

			if err == nil && len(hourData) > 0 {
				// 예: 키 2026022409
				if usage, ok := hourData[yyyymmddhh]; ok {
					em.broadcastSensor(eType, "hourly", usage)
				}
			}

			// 각 에너지 타입(전기/가스/수도)마다 1초씩 쉬어줌
			time.Sleep(1 * time.Second)
		}

		// 다음 수집까지 10분 대기
		time.Sleep(10 * time.Minute)
	}
}

// HA로 데이터를 전송하는 헬퍼 함수
func (em *EnergyMonitor) broadcastSensor(energyType, timeUnit string, value float64) {
	deviceID := fmt.Sprintf("energy_%s_%s", strings.ToLower(energyType), strings.ToLower(timeUnit))

	em.cacheMu.Lock()
	em.lastValues[deviceID] = value
	em.cacheMu.Unlock()

	em.wsServer.Broadcast(ws.WSMsg{
		Type:     "event",
		Domain:   "sensor",
		DeviceID: deviceID,
		State:    fmt.Sprintf("%.3f", value),
	})
	log.Printf("[ENERGY] Broadcasted: %s = %.3f", deviceID, value)
}

// 초기 접속 시 캐싱된 모든 값을 한 번에 전송
func (em *EnergyMonitor) BroadcastAll() {
	em.cacheMu.RLock()
	defer em.cacheMu.RUnlock()

	for deviceID, value := range em.lastValues {
		em.wsServer.Broadcast(ws.WSMsg{
			Type:     "event",
			Domain:   "sensor",
			DeviceID: deviceID,
			State:    fmt.Sprintf("%.3f", value),
		})
	}
}

// 월패드에 SOAP 요청을 보내고 <result> 태그 안의 내용을 파싱하는 함수
func (em *EnergyMonitor) fetchEDS(funcCall string) (map[string]float64, error) {
	soapBody := fmt.Sprintf(`
	<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
	  <v:Header />
	  <v:Body>
	    <n0:callEDS id="o0" c:root="1" xmlns:n0="urn:ces">
	      <in i:type="d:string">call %s</in>
	    </n0:callEDS>
	  </v:Body>
	</v:Envelope>`, funcCall)

	req, err := http.NewRequest("POST", em.cesURL, bytes.NewBuffer([]byte(strings.TrimSpace(soapBody))))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("User-Agent", "kSOAP/2.0")
	req.Header.Set("SOAPAction", "")
	req.Header.Set("Connection", "close") // 타임아웃 방지용 연결 끊기 선언

	resp, err := em.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// 정규표현식을 사용하여 네임스페이스와 속성에 상관없이 <result> 내부의 값만 확실하게 추출
	re := regexp.MustCompile(`(?s)<(?:\w+:)?result[^>]*>(.*?)</(?:\w+:)?result>`)
	matches := re.FindStringSubmatch(bodyStr)

	if len(matches) < 2 {
		// 파싱에 실패하면 디버깅을 위해 원본 응답을 로그로 출력
		log.Printf("[ENERGY DUMP] 원본 응답: %s\n", bodyStr)
		return nil, fmt.Errorf("result tag not found in response")
	}

	rawResult := matches[1]
	if rawResult == "" {
		// 값이 없는 경우는 에러가 아니라 아직 사용량이 집계 안 된 정상 상태일 수 있음
		return make(map[string]float64), nil
	}

	resultMap := make(map[string]float64)

	// format: "20250701#123.4$20250702#200.5" -> "date#usage"
	entries := strings.Split(rawResult, "$")
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "#")
		if len(parts) >= 2 {
			val, err := strconv.ParseFloat(parts[1], 64)
			if err == nil {
				resultMap[parts[0]] = val
			}
		}
	}

	return resultMap, nil
}
