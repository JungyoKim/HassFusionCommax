# 문제 해결 가이드

## MQTT 연결이 자꾸 끊어져요 (EOF)

### 원인
1. **Home Assistant MQTT 브로커 설정**
   - 기본적으로 HA는 익명 연결을 허용하지 않을 수 있습니다
   - 최대 클라이언트 수 제한
   - Keep-alive 타임아웃 설정

2. **네트워크 문제**
   - 방화벽/라우터가 idle 연결을 끊음
   - WiFi 절전 모드

### 해결 방법

#### 1. Home Assistant MQTT 사용자 생성
```yaml
# configuration.yaml
mqtt:
  broker: core-mosquitto
  username: hasstcp
  password: your_password_here
```

그리고 `config.yaml` 수정:
```yaml
mqtt:
  broker: "tcp://192.168.0.15:1883"
  client_id: "hasstcp"
  username: "hasstcp"      # 주석 제거
  password: "your_password" # 주석 제거
  topic: "hasstcp/parking"
```

#### 2. Mosquitto 설정 확인 (HA Add-on)
Home Assistant → 설정 → 추가 기능 → Mosquitto broker → 구성

```yaml
logins:
  - username: hasstcp
    password: your_password_here
anonymous: false
customize:
  active: false
  folder: mosquitto
certfile: fullchain.pem
keyfile: privkey.pem
require_certificate: false
```

#### 3. 이미 개선된 설정
최신 빌드에는 다음 설정이 포함됨:
- ✅ Keep-alive: 60초
- ✅ Ping timeout: 10초
- ✅ 자동 재연결: 3초 간격
- ✅ Clean session: true
- ✅ Resume subscriptions: true

### 정상 작동 확인
연결이 안정화되면:
```
2025/10/09 15:20:00 MQTT connected to tcp://192.168.0.15:1883
2025/10/09 15:20:03 packets seen=10
2025/10/09 15:20:06 packets seen=12
...
(connection lost 메시지가 사라짐)
```

### 여전히 문제가 있다면
1. **MQTT 브로커 로그 확인**
   - Home Assistant → 설정 → 추가 기능 → Mosquitto broker → 로그

2. **네트워크 테스트**
   ```powershell
   # Windows에서 MQTT 브로커 포트 확인
   Test-NetConnection -ComputerName 192.168.0.15 -Port 1883
   ```

3. **임시로 MQTT 비활성화**
   `config.yaml`에서 broker를 비워두면 MQTT 없이 콘솔에만 출력:
   ```yaml
   mqtt:
     broker: ""  # 비워두면 MQTT 비활성화
   ```

## 차량이 입차해도 감지되지 않아요

1. **올바른 네트워크 인터페이스 확인**
   ```bash
   ssh root@192.168.0.60 "ip a"
   # 또는
   ssh root@192.168.0.60 "tcpdump -D"
   ```

2. **패킷이 캡처되는지 확인**
   ```
   2025/10/09 15:20:03 packets seen=10  # 숫자가 증가해야 함
   ```

3. **직접 테스트**
   ```bash
   ssh root@192.168.0.60
   tcpdump -nn -i eth0.2 -s 0 'port 80'
   # 브라우저로 해당 주차 시스템에 접속
   ```

## SSH 연결 실패

```
config.yaml 확인:
- host: "192.168.0.60:22"
- user: "root"
- password: "올바른 비밀번호"
```

수동 테스트:
```powershell
ssh root@192.168.0.60
```

