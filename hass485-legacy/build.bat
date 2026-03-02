@echo off
REM hass485 빌드 스크립트 (Windows)

echo ====================================
echo hass485 크로스 컴파일 빌드
echo ====================================
echo.

echo [1/3] Linux AMD64 빌드 중...
set GOOS=linux
set GOARCH=amd64
go build -o hass485-linux .
if %errorlevel% neq 0 (
    echo 빌드 실패!
    pause
    exit /b 1
)
echo Linux AMD64 빌드 완료: hass485-linux
echo.

echo [2/3] Linux ARM64 빌드 중...
set GOOS=linux
set GOARCH=arm64
go build -o hass485-linux-arm64 .
if %errorlevel% neq 0 (
    echo 빌드 실패!
    pause
    exit /b 1
)
echo Linux ARM64 빌드 완료: hass485-linux-arm64
echo.

echo [3/3] Windows AMD64 빌드 중...
set GOOS=windows
set GOARCH=amd64
go build -o hass485.exe .
if %errorlevel% neq 0 (
    echo 빌드 실패!
    pause
    exit /b 1
)
echo Windows AMD64 빌드 완료: hass485.exe
echo.

echo ====================================
echo 모든 빌드 완료!
echo ====================================
echo.
echo 생성된 파일:
echo   - hass485-linux (Linux AMD64)
echo   - hass485-linux-arm64 (Linux ARM64)
echo   - hass485.exe (Windows AMD64)
echo.
pause


