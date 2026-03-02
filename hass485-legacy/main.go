package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

func main() {
	// 시뮬레이션 모드 확인
	simulationMode := len(os.Args) > 1 && os.Args[1] == "--simulation"

	// 운영체제별 시리얼 포트 경로 설정
	var lightPort, elevatorPort, doorbellPort, boilerPort string

	if simulationMode {
		// 시뮬레이션 모드 - 가상 포트 사용
		lightPort = "SIM_LIGHT"
		elevatorPort = "SIM_ELEVATOR"
		doorbellPort = "SIM_DOORBELL"
		boilerPort = "SIM_BOILER"
	} else if runtime.GOOS == "windows" {
		// Windows 환경 - 실제 사용 가능한 COM 포트로 설정
		lightPort = "COM3"
		elevatorPort = "COM1"
		doorbellPort = "COM2"
		boilerPort = "COM4"
	} else {
		// Linux/Unix 환경
		lightPort = "/dev/ttyUSB3"
		elevatorPort = "/dev/ttyUSB0"
		doorbellPort = "/dev/ttyUSB1"
		boilerPort = "/dev/ttyUSB2"
	}

	mqttBroker := "tcp://192.168.0.15:1883"
	mqttPrefix := "home"

	log.Printf("[SYSTEM] 운영체제: %s\n", runtime.GOOS)
	if simulationMode {
		log.Printf("[SYSTEM] 시뮬레이션 모드로 실행 중\n")
	}
	log.Printf("[SYSTEM] 시리얼 포트 설정:\n")
	log.Printf("  - 조명: %s\n", lightPort)
	log.Printf("  - 엘리베이터: %s\n", elevatorPort)
	log.Printf("  - 도어벨: %s\n", doorbellPort)
	log.Printf("  - 보일러: %s\n", boilerPort)
	log.Printf("  - MQTT 브로커: %s\n", mqttBroker)

	// 무한 재시작 루프
	for {
		log.Println("[SYSTEM] 컨트롤러 시작 중...")

		// 컨트롤러 실행
		err := runControllers(lightPort, elevatorPort, doorbellPort, boilerPort, mqttBroker, mqttPrefix)

		if err != nil {
			log.Printf("[SYSTEM] 컨트롤러 실행 중 오류 발생: %v\n", err)
		}

		log.Println("[SYSTEM] 컨트롤러가 종료되었습니다. 60초 후 재시작합니다...")
		time.Sleep(60 * time.Second)
	}
}

func runControllers(lightPort, elevatorPort, doorbellPort, boilerPort, mqttBroker, mqttPrefix string) error {
	// 에러 채널 생성
	errorChan := make(chan error, 4)
	doneChan := make(chan struct{})

	// 컨트롤러 실행 함수
	runController := func(name string, controllerFunc func() error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] 패닉 복구: %v\n", name, r)
				errorChan <- fmt.Errorf("%s 패닉: %v", name, r)
			}
		}()

		if err := controllerFunc(); err != nil {
			log.Printf("[%s] 오류: %v\n", name, err)
			errorChan <- fmt.Errorf("%s 오류: %v", name, err)
		}
	}

	// 기기별 실행
	go runController("LIGHT", func() error {
		RunLightController(lightPort, mqttBroker, mqttPrefix)
		return nil
	})

	go runController("BOILER", func() error {
		RunBoilerController(boilerPort, mqttBroker, mqttPrefix)
		return nil
	})

	go runController("ELEVATOR", func() error {
		RunElevatorController(elevatorPort, mqttBroker, mqttPrefix)
		return nil
	})

	go runController("DOORBELL", func() error {
		RunDoorbellController(doorbellPort, mqttBroker, mqttPrefix)
		return nil
	})

	// 컨트롤러 상태 모니터링
	go func() {
		errorCount := 0
		for {
			select {
			case err := <-errorChan:
				errorCount++
				log.Printf("[SYSTEM] 컨트롤러 오류 발생 (%d/4): %v\n", errorCount, err)

				// 2개 이상의 컨트롤러에서 오류가 발생하면 전체 재시작
				if errorCount >= 2 {
					log.Println("[SYSTEM] 다수 컨트롤러 오류로 인한 전체 재시작")
					doneChan <- struct{}{}
					return
				}
			}
		}
	}()

	// 종료 신호 대기
	select {
	case <-doneChan:
		return fmt.Errorf("다수 컨트롤러 오류")
	}
}
