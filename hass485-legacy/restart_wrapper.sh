#!/bin/bash

# hass485 자동 재시작 래퍼 스크립트
# 프로그램이 종료되면 자동으로 재시작합니다

PROGRAM="./hass485-linux"
LOG_FILE="./hass485_restart.log"
RESTART_COUNT=0

# 로그 함수
log_message() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# 시그널 핸들러
cleanup() {
    log_message "재시작 래퍼가 종료됩니다. (시그널: $1)"
    exit 0
}

# 시그널 트랩 설정
trap cleanup SIGINT SIGTERM

log_message "hass485 자동 재시작 래퍼 시작"

while true; do
    log_message "프로그램 시작 (재시작 횟수: $RESTART_COUNT)"
    
    # 프로그램 실행
    $PROGRAM
    
    EXIT_CODE=$?
    RESTART_COUNT=$((RESTART_COUNT + 1))
    
    log_message "프로그램 종료 (종료 코드: $EXIT_CODE, 재시작 횟수: $RESTART_COUNT)"
    
    # 정상 종료인 경우 (시그널로 인한 종료)
    if [ $EXIT_CODE -eq 0 ] || [ $EXIT_CODE -eq 130 ] || [ $EXIT_CODE -eq 143 ]; then
        log_message "정상 종료로 인식되어 재시작을 중단합니다."
        break
    fi

done

log_message "자동 재시작 래퍼 종료"

