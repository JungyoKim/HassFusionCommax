# 월패드 제어 시스템

월패드 제어를 위한 다양한 스크립트와 Home Assistant 연동 설정을 포함한 프로젝트입니다.

## 주요 기능

### 1. 엘리베이터 모니터링 (ev.go)
- SOAP API를 통해 엘리베이터 상태 실시간 모니터링
- MQTT를 통해 Home Assistant로 상태 전송
- 2대의 엘리베이터 동시 모니터링

**실행 방법:**
```bash
# Windows에서 실행
.\run_ev.bat

# 또는 직접 실행
$env:GOOS="windows"; $env:GOARCH="amd64"; go run ev.go
```

**MQTT 메시지 구조:**
```json
{
  "floor": "15",
  "is_basement": "0",
  "direction": "1",
  "status": "1",
  "call_up": "0",
  "call_down": "0"
}
```

### 2. Home Assistant 연동

#### MQTT 설정 (configuration.yaml)
```yaml
mqtt:
  broker: 192.168.0.15
  port: 1883
  discovery: true
  discovery_prefix: homeassistant
```

#### 엘리베이터 센서 설정
`homeassistant_mqtt_config.yaml` 파일에 다음 센서들이 포함됩니다:

**센서 (sensor):**
- 엘리베이터 1/2 층수
- 엘리베이터 1/2 방향 (정지/상행/하행)
- 엘리베이터 1/2 상태 (정상/오류)

**바이너리 센서 (binary_sensor):**
- 엘리베이터 1/2 상행 호출
- 엘리베이터 1/2 하행 호출
- 엘리베이터 1/2 지하층 여부

#### 자동화 설정
- 엘리베이터 층수 변화 시 알림
- 호출 버튼 감지 시 알림
- 모바일 앱 푸시 알림

#### 대시보드 설정
- 엘리베이터 상태 실시간 모니터링
- 층수, 방향, 호출 상태 표시
- 상태 요약 버튼

### 3. 사용법

#### 1단계: MQTT 브로커 설정
Home Assistant의 `configuration.yaml`에 MQTT 브로커 설정을 추가합니다.

#### 2단계: 엘리베이터 모니터링 시작
```bash
.\run_ev.bat
```

#### 3단계: Home Assistant 설정 적용
`homeassistant_mqtt_config.yaml`의 내용을 Home Assistant 설정에 추가합니다.

#### 4단계: 대시보드 확인
Home Assistant에서 "엘리베이터 모니터링" 탭을 확인합니다.

### 4. 엘리베이터 상태 값 설명

#### 방향 (direction)
- `0`: 정지
- `1`: 상행
- `2`: 하행

#### 상태 (status)
- `1`: 정상
- `0`: 오류

#### 호출 버튼 (call_up/call_down)
- `1`: 호출됨
- `0`: 호출되지 않음

#### 지하층 여부 (is_basement)
- `1`: 지하층
- `0`: 지상층

### 5. 알림 기능

#### 자동 알림
- 엘리베이터 층수 변화 시
- 호출 버튼이 눌렸을 때
- 엘리베이터 상태 변화 시

#### 수동 알림
- "상태 요약" 버튼으로 현재 상태 확인
- 스크립트를 통한 상태 요약 알림

### 6. 문제 해결

#### MQTT 연결 문제
1. MQTT 브로커 주소 확인 (`192.168.0.15:1883`)
2. 네트워크 연결 상태 확인
3. 방화벽 설정 확인

#### 엘리베이터 상태 수신 안됨
1. `ev.go` 프로그램 실행 상태 확인
2. SOAP API 연결 상태 확인
3. MQTT 토픽 구독 상태 확인

#### Home Assistant에서 센서가 보이지 않음
1. MQTT Discovery 설정 확인
2. 센서 설정 파일 적용 확인
3. Home Assistant 재시작

### 7. 파일 구조

```
wallpad/
├── ev.go                    # 엘리베이터 모니터링 스크립트
├── run_ev.bat              # Windows 실행 배치 파일
├── homeassistant_mqtt_config.yaml  # Home Assistant 설정
├── go.mod                   # Go 모듈 설정
├── go.sum                   # Go 의존성 체크섬
└── README.md               # 프로젝트 설명서
```

### 8. 개발 환경

- **Go**: 1.24.5
- **MQTT**: Eclipse Paho MQTT Client
- **Home Assistant**: 최신 버전
- **OS**: Windows 10/11

### 9. 라이선스

이 프로젝트는 개인 사용 목적으로 개발되었습니다.

---

## 추가 기능

### Python 스크립트들
- `call.py`: 엘리베이터 호출
- `open.py`: 문 열기
- `state.py`: 상태 확인
- `switch.py`: 스위치 제어
- `ems.py`: 에너지 모니터링

### Go 스크립트들
- `main.go`: 메인 제어 프로그램
- `sensor.go`: 센서 데이터 처리
- `publicdooropen.go`: 공용문 열기

각 스크립트의 자세한 사용법은 해당 파일의 주석을 참조하세요.

