@echo off
echo 엘리베이터 모니터링 프로그램을 시작합니다...
echo.
set GOOS=windows
set GOARCH=amd64
go run ev.go
pause
