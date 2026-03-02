#!/bin/bash

# 모든 프로세스 재시작 스크립트

echo "=== 프로세스 정지 중 ==="

# 1. restart_wrapper 정지 (자식 프로세스도 함께 종료)
if pgrep -f restart_wrapper > /dev/null; then
    echo "restart_wrapper 정지 중..."
    pkill -TERM -f restart_wrapper
    sleep 2
fi

# 2. 개별 프로세스 정지 (혹시 남아있을 경우)
if pgrep -f hass485-linux > /dev/null; then
    echo "hass485-linux 정지 중..."
    pkill -TERM -f hass485-linux
fi

if pgrep -f hasstcp > /dev/null; then
    echo "hasstcp 정지 중..."
    pkill -TERM -f hasstcp
fi

# 3. 프로세스 완전 종료 대기
echo "프로세스 종료 대기 중..."
sleep 3

# 4. 강제 종료 (혹시 안 꺼진 프로세스)
pkill -9 -f restart_wrapper 2>/dev/null
pkill -9 -f hass485-linux 2>/dev/null
pkill -9 -f hasstcp 2>/dev/null

echo "=== 모든 프로세스 정지 완료 ==="
sleep 1

# 5. 재시작
echo "=== 프로세스 재시작 중 ==="
./restart_wrapper.sh &

sleep 2
echo "=== 재시작 완료! ==="
echo ""
echo "실행 중인 프로세스:"
ps aux | grep -E "restart_wrapper|hass485-linux|hasstcp" | grep -v grep



