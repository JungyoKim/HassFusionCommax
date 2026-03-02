#!/bin/bash

# HASS485 서비스 설치 스크립트

echo "🚀 HASS485 서비스 설치를 시작합니다..."

# 1. 실행 파일에 실행 권한 부여
chmod +x hass485-linux

# 2. 서비스 파일을 systemd 디렉토리로 복사
sudo cp hass485.service /etc/systemd/system/

# 3. systemd 재로드
sudo systemctl daemon-reload

# 4. 서비스 활성화 (부팅 시 자동 시작)
sudo systemctl enable hass485.service

# 5. 서비스 시작
sudo systemctl start hass485.service

# 6. 상태 확인
echo "📊 서비스 상태 확인 중..."
sudo systemctl status hass485.service

echo "✅ 설치 완료!"
echo ""
echo "📋 유용한 명령어:"
echo "  서비스 상태 확인: sudo systemctl status hass485.service"
echo "  서비스 시작: sudo systemctl start hass485.service"
echo "  서비스 중지: sudo systemctl stop hass485.service"
echo "  서비스 재시작: sudo systemctl restart hass485.service"
echo "  로그 확인: sudo journalctl -u hass485.service -f" 