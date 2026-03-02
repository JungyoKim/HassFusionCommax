// package main

// import (
// 	"flag"
// 	"fmt"
// 	"log"
// 	"os"
// 	"os/signal"
// 	"syscall"

// 	"wallpad-control/config"
// 	"wallpad-control/services"
// 	"wallpad-control/utils"
// )

// func main() {
// 	// CLI 플래그 파싱
// 	configPath := flag.String("config", "config.yaml", "설정 파일 경로")
// 	flag.Parse()

// 	// 설정 로드
// 	cfg, err := config.Load(*configPath)
// 	if err != nil {
// 		log.Fatalf("설정 로드 실패: %v", err)
// 	}

// 	// MQTT 클라이언트 초기화
// 	mqttClient, err := utils.NewMQTTClient(cfg.MQTT)
// 	if err != nil {
// 		log.Fatalf("MQTT 클라이언트 초기화 실패: %v", err)
// 	}
// 	defer mqttClient.Disconnect(250)

// 	// 서비스들 초기화
// 	elevatorService := services.NewElevatorService(cfg.Elevator, mqttClient)
// 	energyService := services.NewEnergyService(cfg.Energy, mqttClient)
// 	doorService := services.NewDoorService(cfg.Door, mqttClient)

// 	// 서비스 시작
// 	if err := elevatorService.Start(); err != nil {
// 		log.Printf("엘리베이터 서비스 시작 실패: %v", err)
// 	}
// 	if err := energyService.Start(); err != nil {
// 		log.Printf("에너지 서비스 시작 실패: %v", err)
// 	}
// 	if err := doorService.Start(); err != nil {
// 		log.Printf("문 제어 서비스 시작 실패: %v", err)
// 	}

// 	fmt.Println("월패드 제어 서비스가 시작되었습니다.")
// 	fmt.Println("Home Assistant MQTT 연동 준비 완료")

// 	// 종료 신호 대기
// 	sigChan := make(chan os.Signal, 1)
// 	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
// 	<-sigChan

// 	fmt.Println("\n서비스를 종료합니다...")

// 	// 서비스 정리
// 	elevatorService.Stop()
// 	energyService.Stop()
// 	doorService.Stop()
// }
