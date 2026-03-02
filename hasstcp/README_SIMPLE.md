# hasstcp - 간단 사용 가이드

## 1. 설정 파일 수정

`config.yaml` 파일을 열어서 필요한 정보만 수정하세요:

```yaml
ssh:
  host: "192.168.0.60:22"        # SSH 서버 주소
  user: "root"                    # SSH 사용자
  password: "1!@Honami"           # SSH 비밀번호
  command: "tcpdump -nn -i eth0.2 -U -s 0 -w - not port 22"

mqtt:
  broker: "tcp://192.168.0.15:1883"  # MQTT 브로커 (Home Assistant)
  topic: "hasstcp/parking"
```

## 2. 실행

### 방법 1: 배치 파일 더블클릭
```
start.bat 더블클릭
```

### 방법 2: exe 파일 직접 실행
```
hasstcp.exe 더블클릭
```

### 방법 3: 명령 프롬프트
```cmd
hasstcp.exe
```

끝! 이제 차량 입출차가 감지되면 자동으로 Home Assistant로 전송됩니다.

## 출력 예시

```
2025/10/09 00:30:00 SSH: root@192.168.0.60:22
2025/10/09 00:30:00 MQTT enabled: tcp://192.168.0.15:1883 -> hasstcp/parking
tcpdump: listening on eth0.2, link-type EN10MB (Ethernet), snapshot length 262144 bytes
2025/10/09 00:30:05 packets seen=12

🚗 [차량 입차 패킷 감지] 번호판: 2814 | 시간: 2025-10-09T00:30:10+09:00 | 10.9.12.11:38598 -> 10.0.0.2:29709 (inbound)
2025/10/09 00:30:10 MQTT published: parkIn -> 2814
```

## Home Assistant 설정

1. Home Assistant `configuration.yaml`에 `ha_config_example.yaml` 내용 복사
2. Home Assistant 재시작
3. 센서 확인:
   - `sensor.parking_latest_entry` - 최근 입차 차량
   - `sensor.parking_latest_exit` - 최근 출차 차량
   - `binary_sensor.parking_entry_detected` - 입차 감지

## 문제 해결

- **SSH 연결 실패**: `config.yaml`의 host, user, password 확인
- **MQTT 연결 실패**: Home Assistant IP 주소(`192.168.0.15`) 확인
- **패킷이 안 잡힘**: 원격 서버의 네트워크 인터페이스명(`eth0.2`) 확인

## 서비스로 등록 (선택사항)

Windows에서 자동 시작하려면 작업 스케줄러에 등록:
1. 작업 스케줄러 실행
2. 기본 작업 만들기
3. 프로그램 시작: `D:\Users\G433m\Documents\hasstcp\hasstcp.exe`
4. 시작 위치: `D:\Users\G433m\Documents\hasstcp`
5. 트리거: 시스템 시작 시

