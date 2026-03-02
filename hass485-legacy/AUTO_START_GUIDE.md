# 🚀 HASS485 자동 시작 설정 가이드

## 방법 1: Systemd 서비스 (권장)

### 1. 파일 업로드
```bash
# Linux 서버에 파일 업로드
scp hass485-linux user@your-server:/home/homeassistant/hass485/
scp hass485.service user@your-server:/home/homeassistant/hass485/
scp install_service.sh user@your-server:/home/homeassistant/hass485/
```

### 2. 서비스 설치
```bash
# 서버에 접속
ssh user@your-server

# 설치 스크립트 실행
cd /home/homeassistant/hass485
chmod +x install_service.sh
./install_service.sh
```

### 3. 상태 확인
```bash
# 서비스 상태 확인
sudo systemctl status hass485.service

# 로그 확인
sudo journalctl -u hass485.service -f
```

## 방법 2: Home Assistant Add-on

### 1. Add-on 디렉토리 생성
```bash
# Home Assistant의 addons 디렉토리에 추가
mkdir -p /config/addons/hass485
```

### 2. 파일 복사
```bash
# 필요한 파일들을 복사
cp config.yaml /config/addons/hass485/
cp Dockerfile /config/addons/hass485/
cp hass485-linux /config/addons/hass485/
```

### 3. Add-on 활성화
- Home Assistant 웹 인터페이스에서 **설정 > 애드온**으로 이동
- **로컬 애드온** 탭에서 **HASS485** 찾기
- **설치** 후 **시작** 버튼 클릭

## 방법 3: Home Assistant Configuration.yaml

### 1. 스크립트 파일 업로드
```bash
# 스크립트 파일을 Home Assistant config 디렉토리에 업로드
cp startup_script.sh /config/scripts/
chmod +x /config/scripts/startup_script.sh
```

### 2. Configuration.yaml 수정 (최신 형식)
```yaml
# configuration.yaml에 다음 내용 추가

# 셸 명령어 정의
shell_command:
  start_hass485: "bash /config/scripts/startup_script.sh"

# 자동화 (최신 형식)
automation:
  - id: 'hass485_auto_start'
    alias: "HASS485 자동 시작"
    description: "Home Assistant 시작 시 HASS485 프로그램 자동 실행"
    trigger:
      - platform: homeassistant
        event: start
    condition: []
    action:
      - service: shell_command.start_hass485
    mode: single
```

### 3. 자동화 파일 분리 (권장)
```yaml
# configuration.yaml에 추가
automation: !include hass485_automations.yaml
```

### 4. Home Assistant 재시작
- **설정 > 시스템 > 재시작** 클릭

## 🔧 유용한 명령어

### Systemd 서비스 관리
```bash
# 서비스 시작
sudo systemctl start hass485.service

# 서비스 중지
sudo systemctl stop hass485.service

# 서비스 재시작
sudo systemctl restart hass485.service

# 서비스 상태 확인
sudo systemctl status hass485.service

# 로그 실시간 확인
sudo journalctl -u hass485.service -f
```

### 프로세스 관리
```bash
# 프로세스 확인
ps aux | grep hass485

# 프로세스 종료
pkill -f hass485-linux

# 로그 확인
tail -f /config/hass485.log
```

## 🎯 권장 방법

**Systemd 서비스 (방법 1)**를 권장합니다:
- ✅ 안정적인 서비스 관리
- ✅ 자동 재시작 기능
- ✅ 시스템 로그 통합
- ✅ 부팅 시 자동 시작

## 📋 자동화 예시

### 기본 자동화 (HASS485 시작)
```yaml
automation:
  - id: 'hass485_auto_start'
    alias: "HASS485 자동 시작"
    description: "Home Assistant 시작 시 HASS485 프로그램 자동 실행"
    trigger:
      - platform: homeassistant
        event: start
    condition: []
    action:
      - service: shell_command.start_hass485
    mode: single
```

### 도어벨 알림 자동화
```yaml
automation:
  - id: 'hass485_doorbell_notification'
    alias: "도어벨 벨 울림 알림"
    description: "도어벨이 울리면 모바일로 알림 전송"
    trigger:
      - platform: state
        entity_id: binary_sensor.doorbell
        to: "on"
    condition: []
    action:
      - service: notify.mobile_app
        data:
          title: "🔔 도어벨"
          message: "현관문에 누군가 왔습니다!"
    mode: single
```

## ⚠️ 주의사항

1. **시리얼 포트 권한**: USB 장치에 대한 접근 권한 확인
2. **MQTT 브로커**: Home Assistant의 MQTT 브로커가 실행 중인지 확인
3. **포트 설정**: 시리얼 포트 경로가 올바른지 확인
4. **로그 모니터링**: 정기적으로 로그를 확인하여 오류 감지
5. **자동화 ID**: 각 자동화에 고유한 ID를 부여해야 함 