# HASS485 - Unix Socket 버전

RS485 통신을 통한 홈 오토메이션 시스템 (Unix Socket 기반)

## 📁 파일 구조

```
hass485/
├── main.go              # 메인 서버 (Unix Socket 서버)
├── light_socket.go      # 조명 제어
├── boiler_socket.go     # 보일러 제어
├── elevator_socket.go   # 엘리베이터 제어
├── doorbell_socket.go   # 도어벨 제어 (벨 감지 + 문열기)
├── socket.go            # Unix Socket 클라이언트
├── utils.go             # 유틸리티 함수
├── client/
│   └── main.go         # 테스트 클라이언트
└── README.md           # 이 파일
```

## 🚀 빌드 및 실행

### 서버 빌드
```bash
# 리눅스용 빌드
GOOS=linux GOARCH=amd64 go build -o hass485_linux .

# 윈도우용 빌드
go build -o hass485.exe .
```

### 클라이언트 빌드
```bash
# 리눅스용 빌드
GOOS=linux GOARCH=amd64 go build -o test_client_linux ./client

# 윈도우용 빌드
go build -o test_client.exe ./client
```

### 실행
```bash
# 서버 실행
./hass485_linux

# 클라이언트 테스트
./test_client_linux light-on 1
./test_client_linux boiler-heat 2
./test_client_linux elevator-call
./test_client_linux alloff-on
./test_client_linux alloff-off
./test_client_linux door-open
./test_client_linux doorbell-ring
```

## 🔧 설정

### 시리얼 포트 설정
```go
lightPort := "/dev/ttyUSB3"    // 조명
boilerPort := "/dev/ttyUSB2"   // 보일러
elevatorPort := "/dev/ttyUSB0" // 엘리베이터
doorbellPort := "/dev/ttyUSB1" // 도어벨 (벨 감지 + 문열기)
```

### 소켓 경로
```go
socketPath := "/config/hass485.sock"
```

## 📡 API

### 조명 제어
- **소켓 경로**: `/lights/{번호}/set`
- **값**: `"ON"`, `"OFF"`
- **예시**: `{"type":"SET","path":"/lights/1/set","value":"ON"}`

### 보일러 제어
- **모드 설정**: `/boilers/{번호}/mode/set`
- **온도 설정**: `/boilers/{번호}/temperature/set`
- **값**: `"heat"`, `"off"`, `"25"` (온도)
- **예시**: `{"type":"SET","path":"/boilers/2/mode/set","value":"heat"}`

### 엘리베이터 제어
- **소켓 경로**: `/elevator/call/set`
- **값**: `"ON"`
- **예시**: `{"type":"SET","path":"/elevator/call/set","value":"ON"}`

### 일괄소등 제어
- **소켓 경로**: `/alloff/set`
- **값**: `"ON"` 또는 `"OFF"`
- **예시**: `{"type":"SET","path":"/alloff/set","value":"ON"}` 또는 `{"type":"SET","path":"/alloff/set","value":"OFF"}`

### 도어 제어 (문열기)
- **소켓 경로**: `/door/open/set`
- **값**: `"ON"`
- **예시**: `{"type":"SET","path":"/door/open/set","value":"ON"}`

## 📊 상태 모니터링

### 상태 발행
각 컨트롤러는 상태 변경 시 자동으로 소켓에 발행합니다:
- `/lights/{번호}/state` → `"ON"` / `"OFF"`
- `/boilers/{번호}/mode` → `"heat"` / `"off"`
- `/boilers/{번호}/current_temp` → `"25"`
- `/boilers/{번호}/set_temp` → `"20"`
- `/alloff/state` → `"ON"` / `"OFF"`
- `/doorbell/state` → `"ON"` / `"OFF"` (벨 울림 감지)

## 🔄 명령 처리 흐름

1. **클라이언트** → Unix Socket → **서버**
2. **서버** → 채널 → **컨트롤러**
3. **컨트롤러** → RS485 → **하드웨어**
4. **하드웨어** → RS485 → **컨트롤러**
5. **컨트롤러** → 소켓 → **클라이언트**

## 🛠️ 지원 명령

### 조명 (5개)
- `light-on 1` ~ `light-on 5`
- `light-off 1` ~ `light-off 5`

### 보일러 (4개 방)
- `boiler-heat 1` ~ `boiler-heat 4`
- `boiler-off 1` ~ `boiler-off 4`
- `boiler-temp 1 25` ~ `boiler-temp 4 30`

### 엘리베이터
- `elevator-call`

### 일괄소등
- `alloff-on`
- `alloff-off`

### 도어 (문열기)
- `door-open`

## 📝 로그

각 컨트롤러는 상세한 로그를 출력합니다:
- `[조명]` - 조명 관련 로그
- `[보일러]` - 보일러 관련 로그
- `[엘리베이터]` - 엘리베이터 관련 로그
- `[일괄소등]` - 일괄소등 관련 로그
- `[도어벨]` - 도어벨 관련 로그 (벨 감지 + 문열기)
- `[소켓]` - 소켓 통신 로그
- `[RS485]` - RS485 통신 로그 