#!/bin/bash

# hass485 & hasstcp 자동 재시작 래퍼 스크립트
# 프로그램들이 종료되면 자동으로 재시작합니다

PROGRAM1="./hass485-linux"
PROGRAM2="./hasstcp"
LOG_FILE="./restart_wrapper.log"
MAX_RESTARTS=100
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

log_message "hass485 & hasstcp 자동 재시작 래퍼 시작"

# 프로그램1 (hass485) 실행 함수
run_program1() {
    log_message "hass485 시작 (재시작 횟수: $RESTART_COUNT)"
    $PROGRAM1
    local exit_code=$?
    log_message "hass485 종료 (종료 코드: $exit_code)"
    return $exit_code
}

# 프로그램2 (hasstcp) 실행 함수
run_program2() {
    log_message "hasstcp 시작 (재시작 횟수: $RESTART_COUNT)"
    $PROGRAM2
    local exit_code=$?
    log_message "hasstcp 종료 (종료 코드: $exit_code)"
    return $exit_code
}

# 병렬 실행 함수
run_parallel() {
    # 백그라운드에서 두 프로그램을 동시 실행
    run_program1 &
    PID1=$!
    run_program2 &
    PID2=$!
    
    log_message "프로그램들이 병렬로 실행 중 (PID1: $PID1, PID2: $PID2)"
    
    # 두 프로세스 모두 종료될 때까지 대기
    wait $PID1
    EXIT_CODE1=$?
    wait $PID2
    EXIT_CODE2=$?
    
    log_message "모든 프로그램 종료 (hass485: $EXIT_CODE1, hasstcp: $EXIT_CODE2)"
    
    # 하나라도 정상 종료가 아닌 경우 재시작
    if [ $EXIT_CODE1 -ne 0 ] && [ $EXIT_CODE1 -ne 130 ] && [ $EXIT_CODE1 -ne 143 ]; then
        return 1
    fi
    if [ $EXIT_CODE2 -ne 0 ] && [ $EXIT_CODE2 -ne 130 ] && [ $EXIT_CODE2 -ne 143 ]; then
        return 1
    fi
    
    return 0
}

while [ $RESTART_COUNT -lt $MAX_RESTARTS ]; do
    log_message "프로그램들 시작 (재시작 횟수: $RESTART_COUNT)"
    
    # 병렬 실행
    run_parallel
    OVERALL_EXIT_CODE=$?
    RESTART_COUNT=$((RESTART_COUNT + 1))
    
    # 정상 종료인 경우
    if [ $OVERALL_EXIT_CODE -eq 0 ]; then
        log_message "정상 종료로 인식되어 재시작을 중단합니다."
        break
    fi
    
    # 최대 재시작 횟수 도달
    if [ $RESTART_COUNT -ge $MAX_RESTARTS ]; then
        log_message "최대 재시작 횟수($MAX_RESTARTS)에 도달했습니다. 재시작을 중단합니다."
        break
    fi
    
    log_message "5초 후 재시작합니다..."
    sleep 5
done

log_message "자동 재시작 래퍼 종료"
