@echo off
setlocal enabledelayedexpansion

REM hass485 자동 재시작 래퍼 배치 파일
REM 프로그램이 종료되면 자동으로 재시작합니다

set PROGRAM=hass485-linux.exe
set LOG_FILE=hass485_restart.log
set RESTART_COUNT=0

REM 로그 함수
:log_message
echo %date% %time% - %~1 >> %LOG_FILE%
echo %date% %time% - %~1
goto :eof

call :log_message "hass485 자동 재시작 래퍼 시작"

:restart_loop

call :log_message "프로그램 시작 (재시작 횟수: %RESTART_COUNT%)"

REM 프로그램 실행
%PROGRAM%

set EXIT_CODE=%ERRORLEVEL%
set /a RESTART_COUNT+=1

call :log_message "프로그램 종료 (종료 코드: %EXIT_CODE%, 재시작 횟수: %RESTART_COUNT%)"

REM 정상 종료인 경우 (Ctrl+C로 인한 종료)
if %EXIT_CODE% equ 0 (
    call :log_message "정상 종료로 인식되어 재시작을 중단합니다."
    goto :end
)

call :log_message "60초 후 재시작합니다..."
timeout /t 60 /nobreak > nul
goto :restart_loop

:end
call :log_message "자동 재시작 래퍼 종료"
pause
