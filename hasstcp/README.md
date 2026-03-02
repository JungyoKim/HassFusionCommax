# hasstcp

Go 기반 원격/로컬 TCP 패킷 캡처 및 HTTP/SOAP 이벤트 감지 도구입니다.

## 주요 기능

- **원격 SSH tcpdump 스트림 수신**: SSH로 원격 서버에 연결해 실시간 패킷 캡처
- **HTTP 요청/응답 감지**: TCP 재조립을 통한 HTTP 트래픽 분석
- **인바운드/아웃바운드 방향 판정**: CIDR 기반 내부망 설정
- **SOAP parkService 감지**: 차량 입출차 패킷 자동 파싱 (번호판, 시간, 입/출차 구분)
- **페이로드 출력**: HTTP 본문 미리보기 (텍스트/hex)
- **MQTT 연동**: Home Assistant 센서로 차량 입출차 이벤트 자동 전송

## 빌드

```powershell
# Windows/amd64 대상
$env:GOOS="windows"
$env:GOARCH="amd64"
go build -o hasstcp.exe ./cmd/hasstcp
```

## 사용법

### 1. SSH 원격 캡처 (기본)

```powershell
.\hasstcp.exe `
  --mode ssh `
  --host 192.168.0.60:22 `
  --user root `
  --pass "비밀번호" `
  --cmd "tcpdump -nn -i eth0.2 -U -s 0 -w - not port 22"
```

### 2. SSH 키 인증

```powershell
.\hasstcp.exe `
  --mode ssh `
  --host 192.168.0.60:22 `
  --user root `
  --key "C:\Users\사용자\.ssh\id_rsa" `
  --cmd "tcpdump -nn -i eth0.2 -U -s 0 -w - not port 22"
```

### 3. 내부망 CIDR 지정 (인바운드/아웃바운드 판정)

```powershell
.\hasstcp.exe `
  --mode ssh `
  --host 192.168.0.60:22 `
  --user root `
  --pass "비밀번호" `
  --cidrs "10.0.0.0/8,192.168.0.0/16" `
  --cmd "tcpdump -nn -i eth0.2 -U -s 0 -w - not port 22"
```

### 4. MQTT 연동 (Home Assistant)

```powershell
.\hasstcp.exe `
  --mode ssh `
  --host 192.168.0.60:22 `
  --user root `
  --pass "비밀번호" `
  --mqtt-broker "tcp://192.168.0.100:1883" `
  --mqtt-user "homeassistant" `
  --mqtt-pass "mqtt비번" `
  --mqtt-topic "hasstcp/parking" `
  --cmd "tcpdump -nn -i eth0.2 -U -s 0 -w - not port 22"
```

### 5. 로컬 tcpdump 파이프 입력

```bash
tcpdump -nn -i eth0 -U -s 0 -w - | hasstcp.exe --mode stdin
```

## 출력 예시

### 일반 HTTP 요청/응답

```
2025-10-08T19:20:48+09:00 inbound http request 10.9.12.11:38598 -> 10.0.0.2:29709 POST /
payload: <SOAP-ENV:Envelope ...>
2025-10-08T19:20:48+09:00 outbound http response 10.0.0.2:29709 -> 10.9.12.11:38598 200
payload: <?xml version="1.0" ...>
```

### 차량 입출차 감지

```
🚗 [차량 입차 패킷 감지] 번호판: 2814 | 시간: 2025-10-09T04:10:43+09:00 | 10.9.12.11:38598 -> 10.0.0.2:29709 (inbound)
```

## CLI 플래그

| 플래그 | 기본값 | 설명 |
|--------|--------|------|
| `--mode` | `ssh` | 캡처 모드: `ssh` 또는 `stdin` |
| `--host` | `192.168.0.60:22` | SSH 서버 주소 (포트 포함) |
| `--user` | `root` | SSH 사용자명 |
| `--pass` | `` | SSH 비밀번호 (옵션) |
| `--key` | `` | SSH 개인키 경로 (옵션) |
| `--cmd` | `tcpdump -nn -i eth0.2 ...` | 원격에서 실행할 tcpdump 명령 |
| `--cidrs` | `` | 내부망 CIDR (쉼표 구분, 예: `10.0.0.0/8,192.168.0.0/16`) |
| `--mqtt-broker` | `` | MQTT 브로커 URL (예: `tcp://192.168.0.100:1883`) |
| `--mqtt-client-id` | `hasstcp` | MQTT 클라이언트 ID |
| `--mqtt-user` | `` | MQTT 사용자명 |
| `--mqtt-pass` | `` | MQTT 비밀번호 |
| `--mqtt-topic` | `hasstcp/parking` | 차량 이벤트 발행 토픽 |

## Home Assistant 연동

MQTT를 통해 Home Assistant에서 센서로 사용할 수 있습니다.

1. `ha_config_example.yaml` 파일의 내용을 Home Assistant `configuration.yaml`에 추가하세요.
2. Home Assistant를 재시작하세요.
3. hasstcp를 MQTT 옵션과 함께 실행하세요.

### MQTT 메시지 형식

```json
{
  "event_type": "parkIn",
  "car_no": "2814",
  "timestamp": "2025-10-09T04:10:43+09:00",
  "direction": "inbound",
  "source": "10.9.12.11:38598",
  "destination": "10.0.0.2:29709"
}
```

- `event_type`: `parkIn` (입차) 또는 `parkOut` (출차)
- `car_no`: 차량 번호판
- `timestamp`: ISO8601 형식 시간
- `direction`: `inbound`, `outbound`, `unknown`

## 패킷 카운터

- 3초마다 수신된 패킷 수를 로그로 출력합니다.
- 원격 tcpdump의 stderr는 콘솔에 연결되어 있습니다.

## 의존성

- `github.com/google/gopacket` v1.1.19
- `golang.org/x/crypto/ssh` v0.42.0
- `github.com/eclipse/paho.mqtt.golang` v1.5.1

## 라이선스

MIT

## 참고

- 민감한 비밀번호는 환경 변수나 키 파일 사용을 권장합니다.
- 패스프레이즈가 있는 개인키는 현재 미지원입니다.
- 압축/청크 인코딩 HTTP 본문은 일부만 표시될 수 있습니다.


