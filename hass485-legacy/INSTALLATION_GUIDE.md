# RS485 홈 오토메이션 시스템 설치 가이드 (최신 MQTT 방식)

## 📋 목차
1. [시스템 요구사항](#시스템-요구사항)
2. [하드웨어 연결](#하드웨어-연결)
3. [소프트웨어 설치](#소프트웨어-설치)
4. [Home Assistant 설정](#home-assistant-설정)
5. [테스트 및 검증](#테스트-및-검증)
6. [문제 해결](#문제-해결)

## 🔧 시스템 요구사항

### 하드웨어
- **USB-RS485 어댑터**: 4개 (조명, 보일러, 엘리베이터, 도어벨용)
- **RS485 기기들**:
  - 조명 컨트롤러 (5개 조명)
  - 보일러 컨트롤러 (4개 방)
  - 엘리베이터 호출 시스템
  - 도어벨 시스템
- **네트워크**: MQTT 브로커 연결 가능한 네트워크

### 소프트웨어
- **운영체제**: Linux (권장) 또는 Windows
- **Go**: 1.24.2 이상
- **Home Assistant**: 2024.1.0 이상 (최신 MQTT 방식 지원)
- **MQTT 브로커**: Mosquitto 또는 Home Assistant 내장 브로커

## 🔌 하드웨어 연결

### 1. USB-RS485 어댑터 연결
```bash
# Linux에서 포트 확인
ls /dev/ttyUSB*

# Windows에서 포트 확인
# 장치 관리자 > 포트(COM & LPT)에서 확인
```

### 2. 기기별 연결
- **조명**: `/dev/ttyUSB3` (Linux) 또는 `COM3` (Windows)
- **보일러**: `/dev/ttyUSB2` (Linux) 또는 `COM2` (Windows)
- **엘리베이터**: `/dev/ttyUSB0` (Linux) 또는 `COM1` (Windows)
- **도어벨**: `/dev/ttyUSB1` (Linux) 또는 `COM1` (Windows)

## 💻 소프트웨어 설치

### 1. Go 설치
```bash
# Linux
sudo apt update
sudo apt install golang-go

# Windows
# https://golang.org/dl/ 에서 다운로드
```

### 2. 프로젝트 빌드
```bash
# Linux용 빌드
GOOS=linux GOARCH=amd64 go build -o hass485-linux .

# Windows용 빌드
go build -o hass485.exe .

# ARM64용 빌드 (라즈베리파이 등)
GOOS=linux GOARCH=arm64 go build -o hass485-linux-arm64 .
```

### 3. 실행 권한 설정
```bash
chmod +x hass485-linux
```

## 🏠 Home Assistant 설정

### 1. MQTT 브로커 설정 (최신 방식)
Home Assistant의 `configuration.yaml`에 추가:
```yaml
mqtt:
  broker: 192.168.0.15  # MQTT 브로커 IP
  port: 1883
  username: your_username  # 선택사항
  password: your_password  # 선택사항
  discovery: true  # 자동 발견 활성화
  discovery_prefix: homeassistant  # 발견 접두사
```

### 2. RS485 기기 설정 (최신 MQTT 방식)
`home_assistant_configuration.yaml` 파일의 내용을 `configuration.yaml`에 추가하거나, packages 디렉토리에 저장:

```bash
# packages 디렉토리 생성
mkdir -p config/packages

# 설정 파일 복사
cp home_assistant_configuration.yaml config/packages/rs485_devices.yaml
```

### 3. configuration.yaml에 포함
```yaml
# configuration.yaml
homeassistant:
  packages:
    rs485_devices: !include packages/rs485_devices.yaml
```

### 4. Lovelace 대시보드 설정
1. Home Assistant 관리자 > 대시보드
2. "YAML 모드" 활성화
3. `lovelace_dashboard.yaml` 내용 복사

## 🚀 실행 및 테스트

### 1. RS485 서비스 실행
```bash
# Linux
./hass485-linux

# Windows
./hass485.exe

# 시뮬레이션 모드 (테스트용)
./hass485-linux --simulation
```

### 2. 서비스 자동 시작 설정
```bash
# systemd 서비스 파일 생성
sudo nano /etc/systemd/system/hass485.service
```

서비스 파일 내용:
```ini
[Unit]
Description=RS485 Home Automation Service
After=network.target

[Service]
Type=simple
User=homeassistant
WorkingDirectory=/home/homeassistant/hass485
ExecStart=/home/homeassistant/hass485/hass485-linux
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

서비스 활성화:
```bash
sudo systemctl daemon-reload
sudo systemctl enable hass485.service
sudo systemctl start hass485.service
```

### 3. 로그 확인
```bash
# 실시간 로그 확인
sudo journalctl -u hass485.service -f

# 서비스 상태 확인
sudo systemctl status hass485.service
```

## ✅ 테스트 및 검증

### 1. 연결 테스트
```bash
# 시리얼 포트 연결 확인
dmesg | grep ttyUSB

# MQTT 연결 테스트
mosquitto_pub -h 192.168.0.15 -t "home/lights/1/set" -m "ON"
```

### 2. Home Assistant에서 확인
1. **개발자 도구** > **상태**에서 기기 확인
2. **설정** > **기기 및 서비스**에서 MQTT 기기 확인
3. **대시보드**에서 제어 테스트

### 3. 기능별 테스트
- **조명**: 각 조명 ON/OFF 테스트
- **보일러**: 온도 설정 및 모드 변경 테스트
- **엘리베이터**: 호출 버튼 테스트
- **도어벨**: 이벤트 감지 테스트

## 🔧 문제 해결

### 1. 시리얼 포트 연결 실패
```bash
# 포트 권한 확인
ls -la /dev/ttyUSB*

# 권한 설정
sudo chmod 666 /dev/ttyUSB*

# 사용자 그룹 추가
sudo usermod -a -G dialout $USER
```

### 2. MQTT 연결 실패
```bash
# MQTT 브로커 상태 확인
sudo systemctl status mosquitto

# 브로커 재시작
sudo systemctl restart mosquitto
```

### 3. Home Assistant 기기 미등록 (최신 방식)
1. **설정** > **기기 및 서비스** > **통합구성요소 추가**
2. **MQTT** 검색 및 추가
3. 브로커 정보 입력
4. **설정** > **기기 및 서비스** > **MQTT** > **설정**에서 기기 확인

### 4. 로그 분석
```bash
# 상세 로그 확인
./hass485-linux 2>&1 | tee hass485.log

# 특정 에러 검색
grep -i "error\|fail" hass485.log
```

## 📱 모바일 앱 설정

### 1. Home Assistant 모바일 앱
1. 앱스토어에서 "Home Assistant" 다운로드
2. 서버 URL 입력: `http://your-ha-ip:8123`
3. 계정 로그인

### 2. 알림 설정 (최신 방식)
```yaml
# configuration.yaml에 추가
notify:
  - platform: mobile_app
    name: mobile_app
```

## 🔒 보안 설정

### 1. MQTT 보안
```yaml
# mosquitto.conf
allow_anonymous false
password_file /etc/mosquitto/passwd
```

### 2. 방화벽 설정
```bash
# MQTT 포트만 허용
sudo ufw allow 1883
sudo ufw allow 8883  # SSL
```

## 📊 모니터링

### 1. 시스템 모니터링
```bash
# CPU/메모리 사용량
htop

# 디스크 사용량
df -h

# 네트워크 연결
netstat -tuln
```

### 2. 로그 모니터링
```bash
# 실시간 로그
tail -f /var/log/syslog | grep hass485

# 로그 로테이션 설정
sudo nano /etc/logrotate.d/hass485
```

## 🔄 최신 MQTT 방식의 장점

### 1. 자동 발견
- 기기가 자동으로 Home Assistant에 등록됨
- 수동 설정 불필요

### 2. 통합된 설정
- 모든 MQTT 기기가 하나의 `mqtt:` 섹션에 통합
- 설정 관리 용이

### 3. 향상된 성능
- 더 빠른 기기 등록
- 효율적인 메모리 사용

## 🆘 지원

문제가 발생하면 다음 정보를 수집하여 문의하세요:
1. 운영체제 및 버전
2. Go 버전 (`go version`)
3. Home Assistant 버전
4. 하드웨어 연결 상태
5. 로그 파일 (`hass485.log`)
6. Home Assistant 로그
7. MQTT 브로커 상태

---

**🎉 설치 완료! 이제 완전한 홈 오토메이션 시스템을 즐기세요!** 