#!/bin/bash

# hass485 빌드 스크립트 (Linux/Mac)

echo "===================================="
echo "hass485 크로스 컴파일 빌드"
echo "===================================="
echo ""

echo "[1/3] Linux AMD64 빌드 중..."
GOOS=linux GOARCH=amd64 go build -o hass485-linux .
if [ $? -ne 0 ]; then
    echo "빌드 실패!"
    exit 1
fi
echo "Linux AMD64 빌드 완료: hass485-linux"
echo ""

echo "[2/3] Linux ARM64 빌드 중..."
GOOS=linux GOARCH=arm64 go build -o hass485-linux-arm64 .
if [ $? -ne 0 ]; then
    echo "빌드 실패!"
    exit 1
fi
echo "Linux ARM64 빌드 완료: hass485-linux-arm64"
echo ""

echo "[3/3] Windows AMD64 빌드 중..."
GOOS=windows GOARCH=amd64 go build -o hass485.exe .
if [ $? -ne 0 ]; then
    echo "빌드 실패!"
    exit 1
fi
echo "Windows AMD64 빌드 완료: hass485.exe"
echo ""

echo "===================================="
echo "모든 빌드 완료!"
echo "===================================="
echo ""
echo "생성된 파일:"
echo "  - hass485-linux (Linux AMD64)"
echo "  - hass485-linux-arm64 (Linux ARM64)"
echo "  - hass485.exe (Windows AMD64)"
echo ""



