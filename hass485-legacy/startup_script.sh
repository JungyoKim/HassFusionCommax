#!/bin/bash

# HASS485 자동 시작 스크립트
# 이 파일을 /config/scripts/ 디렉토리에 저장하고
# Home Assistant의 configuration.yaml에서 호출

HASS485_PATH="/config/hass485"
LOG_FILE="/config/hass485.log"

# 디렉토리 확인
if [ ! -d "$HASS485_PATH" ]; then
    echo "$(date): HASS485 디렉토리가 없습니다: $HASS485_PATH" >> "$LOG_FILE"
    exit 1
fi

# 실행 파일 확인
if [ ! -f "$HASS485_PATH/hass485-linux" ]; then
    echo "$(date): HASS485 실행 파일이 없습니다" >> "$LOG_FILE"
    exit 1
fi

# 실행 권한 확인
chmod +x "$HASS485_PATH/hass485-linux"

# 기존 프로세스 종료
pkill -f hass485-linux

# 새 프로세스 시작
echo "$(date): HASS485 시작 중..." >> "$LOG_FILE"
cd "$HASS485_PATH"
nohup ./hass485-linux >> "$LOG_FILE" 2>&1 &

echo "$(date): HASS485 시작 완료 (PID: $!)" >> "$LOG_FILE" 