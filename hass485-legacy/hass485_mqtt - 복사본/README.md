# HASS485 MQTT 버전

이 프로젝트는 HASS485 시스템을 MQTT를 통해 Home Assistant와 연동하는 버전입니다.

## 🏗️ **아키텍처**

```
Home Assistant MQTT 구성요소
           ↓
    MQTT 브로커 (Mosquitto)
           ↓
    HASS485 MQTT 서버 (Go)
           ↓
    RS485 하드웨어
```

## 📋 **필요 조건**

1. **MQTT 브로커** (Mosquitto 등)
2. **Go 1.21+**
3. **RS485 USB 어댑터**

## 🚀 **설치 및 실행**

### 1. MQTT 브로커 설치 (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install mosquitto mosquitto-clients
sudo systemctl enable mosquitto
sudo systemctl start mosquitto
```

### 2. Go 서버 빌드 및 실행
```bash
cd hass485_mqtt
go mod tidy
go build -o hass485_mqtt_server .
./hass485_mqtt_server
```

### 3. Home Assistant 설정

`configuration.yaml`에 다음을 추가하거나 MQTT 통합구성요소에서 직접 설정:

```yaml
# MQTT 브로커 설정
mqtt:
  broker: localhost
  port: 1883
  username: ""
  password: ""

# 조명 설정
light:
  - platform: mqtt
    name: "거실 조명 1"
    state_topic: "hass485/lights/1/state"
    command_topic: "hass485/lights/1/command"
    state_value_template: "{{ value_json.value }}"
    payload_on: '{"type": "on", "value": "ON"}'
    payload_off: '{"type": "off", "value": "OFF"}'
```

## 📡 **MQTT 토픽 구조**

### 상태 토픽 (서버 → Home Assistant)
- `hass485/lights/{번호}/state` - 조명 상태
- `hass485/boilers/{번호}/state` - 보일러 상태
- `hass485/doorbell/state` - 도어벨 상태
- `hass485/alloff/state` - 일괄소등 상태

### 명령 토픽 (Home Assistant → 서버)
- `hass485/lights/{번호}/command` - 조명 제어
- `hass485/boilers/{번호}/command` - 보일러 제어
- `hass485/door/command` - 도어 제어
- `hass485/elevator/command` - 엘리베이터 제어
- `hass485/alloff/command` - 일괄소등 제어

## 🔧 **설정**

### MQTT 브로커 설정
`mqtt_server.go`에서 다음 상수를 수정:

```go
const (
    MQTT_BROKER = "localhost:1883"  // MQTT 브로커 주소
    MQTT_CLIENT_ID = "hass485_server"
    MQTT_USERNAME = ""               // 사용자명 (필요시)
    MQTT_PASSWORD = ""               // 비밀번호 (필요시)
)
```

### RS485 디바이스 설정
각 컨트롤러에서 USB 디바이스 경로를 수정:

```go
controller := NewLightController("/dev/ttyUSB0", i, mqttClient)
```

## 📊 **상태 모니터링**

서버는 다음을 자동으로 수행합니다:

1. **주기적 상태 쿼리**: 5초마다 RS485 디바이스 상태 확인
2. **상태 변경 감지**: 상태가 변경되면 MQTT로 즉시 발행
3. **명령 처리**: Home Assistant에서 받은 명령을 RS485로 전송

## 🐛 **문제 해결**

### MQTT 연결 실패
```bash
# MQTT 브로커 상태 확인
sudo systemctl status mosquitto

# MQTT 브로커 재시작
sudo systemctl restart mosquitto
```

### RS485 연결 실패
```bash
# USB 디바이스 확인
ls -la /dev/ttyUSB*

# 권한 설정
sudo chmod 666 /dev/ttyUSB0
```

### Home Assistant에서 엔티티가 나타나지 않음
1. MQTT 통합구성요소가 활성화되어 있는지 확인
2. 토픽 이름과 페이로드 형식이 올바른지 확인
3. Home Assistant 로그에서 MQTT 관련 오류 확인

## 📝 **로그 확인**

```bash
# 서버 로그 확인
./hass485_mqtt_server

# MQTT 메시지 모니터링
mosquitto_sub -h localhost -t "hass485/#" -v
```

## 🔄 **기존 Unix Socket 버전과의 차이점**

| 기능 | Unix Socket 버전 | MQTT 버전 |
|------|------------------|-----------|
| 통신 방식 | Unix Socket | MQTT |
| Home Assistant 연동 | Custom Integration | 기본 MQTT 구성요소 |
| 설정 복잡도 | 높음 | 낮음 |
| 안정성 | 중간 | 높음 |
| 확장성 | 제한적 | 높음 |
| 표준 준수 | 비표준 | 표준 |

## 🎯 **장점**

1. **표준 프로토콜**: MQTT는 IoT 표준 프로토콜
2. **간단한 설정**: Home Assistant 기본 MQTT 구성요소 사용
3. **높은 안정성**: MQTT 브로커의 자동 재연결 기능
4. **확장성**: 다른 MQTT 클라이언트와 쉽게 연동
5. **디버깅 용이**: MQTT 브로커에서 메시지 모니터링 가능 