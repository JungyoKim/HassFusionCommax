package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time" // 재연결 대기시간을 위해 추가

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcapgo"

	"hassfusion/capture"
	"hassfusion/config"
	"hassfusion/httpx"
	"hassfusion/rs485"
	"hassfusion/ws"
)

func main() {
	configPath := flag.String("config", "config.yaml", "config file path")
	flag.Parse()

	log.Println("Starting hassfusion (RS485 + TCP Bridge for HA)...")

	// 1. Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Start WebSocket Server
	wsServer := ws.NewServer()
	http.HandleFunc("/ws", wsServer.HandleWS)

	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.WebSocket.Host, cfg.WebSocket.Port)
		log.Printf("Starting WebSocket server on %s", addr)

		// 에러 발생 시 프로그램 강제 종료(Fatalf) 대신 에러만 로깅하도록 변경
		if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket Server Error: %v\n", err)
		}
	}()

	// 3. Start RS485 Managers
	var lc *rs485.LightController
	var bc *rs485.BoilerController
	var ac *rs485.AllOffController
	var dc *rs485.DoorbellController

	if cfg.RS485.Lights != "" {
		lc = rs485.NewLightController(cfg.RS485.Lights, wsServer)
		if lc != nil {
			log.Printf("LightController started on %s", cfg.RS485.Lights)
			defer lc.Close()
		}
	}

	if cfg.RS485.Boilers != "" {
		bc = rs485.NewBoilerController(cfg.RS485.Boilers, wsServer)
		if bc != nil {
			log.Printf("[BOILER] BoilerController started on %s", cfg.RS485.Boilers)
			defer bc.Close()
		}
	}

	if cfg.RS485.Doorbell != "" {
		dc = rs485.NewDoorbellController(cfg.RS485.Doorbell, wsServer)
		if dc != nil {
			log.Printf("[DOORBELL] DoorbellController started on %s", cfg.RS485.Doorbell)
			defer dc.Close()
		}
	}

	if cfg.RS485.AllOff != "" {
		ac = rs485.NewAllOffController(cfg.RS485.AllOff, wsServer)
		if ac != nil {
			log.Printf("[ALLOFF] AlloffController started on %s", cfg.RS485.AllOff)
			defer ac.Close()
		}
	}

	// 4. Start Wallpad API Monitors
	var ev *capture.ElevatorMonitor
	var ems *capture.EnergyMonitor

	ev = capture.NewElevatorMonitor(cfg, wsServer)
	if ev != nil {
		go ev.Run()
	}

	ems = capture.NewEnergyMonitor(cfg, wsServer)
	if ems != nil {
		go ems.Run()
	}

	// Register System Hook to broadcast state upon HA request/reconnect
	wsServer.RegisterHandler("system", func(msg ws.WSMsg) {
		if msg.Action == "request_sync" {
			log.Println("[SYSTEM] Received request_sync from HA. Broadcasting states...")
			if lc != nil {
				lc.BroadcastAll()
			}
			if bc != nil {
				bc.BroadcastAll()
			}
			if ac != nil {
				ac.BroadcastAll()
			}
			if ev != nil {
				ev.BroadcastAll()
			}
			if ems != nil {
				ems.BroadcastAll()
			}
		}
	})

	// 5. Start TCP Packet Sniffing (if configured)
	if cfg.TCP.UseSSH {
		if cfg.TCP.SSH.Host != "" {
			httpx.Setup(wsServer, cfg)

			log.Printf("Starting SSH capture: %s@%s", cfg.TCP.SSH.User, cfg.TCP.SSH.Host)

			// 무한 재연결 로직 적용
			go func() {
				for {
					// 루프 안에서 defer를 안전하게 사용하기 위해 익명 함수로 감쌈
					func() {
						rc, sess, client, err := capture.StartSSHCommand(cfg)
						if err != nil {
							log.Printf("[TCP] SSH capture failed: %v", err)
							return // 실패 시 리턴하여 10초 대기로 넘어감
						}
						defer sess.Close()
						defer client.Close()

						pr, err := pcapgo.NewReader(rc)
						if err != nil {
							log.Printf("[TCP] PCAP Reader failed: %v", err)
							return
						}
						log.Println("[TCP] SSH Capture started successfully.")

						ps := capture.PacketSourceFromReader(pr)
						inCh := make(chan gopacket.Packet, 512)

						// 패킷 조립 고루틴 시작
						go httpx.AssemblePackets(inCh)

						// 패킷을 읽어서 inCh로 전달 (연결이 끊기면 ps.Packets() 루프가 종료됨)
						for packet := range ps.Packets() {
							inCh <- packet
						}

						close(inCh) // SSH 스트림이 끊기면 채널 닫기
						log.Println("[TCP] SSH Capture stream ended. Reconnecting...")
					}()

					// 연결 실패 또는 끊어짐 발생 시 10초 대기 후 재시도
					time.Sleep(10 * time.Second)
				}
			}()

		} else {
			log.Println("TCP Capture enabled but SSH Host is missing from config")
		}
	} else {
		log.Println("TCP Capture disabled in config")
	}

	// 6. Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down hassfusion...")
}
