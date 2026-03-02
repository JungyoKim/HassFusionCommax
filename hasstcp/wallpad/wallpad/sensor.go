// package main

// import (
// 	"bufio"
// 	"fmt"
// 	"strings"
// 	"time"

// 	MQTT "github.com/eclipse/paho.mqtt.golang"
// 	"golang.org/x/crypto/ssh"
// )

// const (
// 	mqttBroker      = "tcp://192.168.0.15:1883"
// 	mqttTopicG80    = "home/packet/car/g80"
// 	mqttTopicSonata = "home/packet/car/sonata"
// 	mqttTopicB4Call = "home/packet/b4call" // 지하4층 호출 센서 토픽
// 	sshUser         = "root"
// 	sshPass         = "1!@Honami"
// 	sshHost         = "192.168.0.60:22"
// 	tcpdumpCmd      = "tcpdump -l -i eth0.2 not port 22"
// )

// func main() {
// 	// MQTT 연결
// 	opts := MQTT.NewClientOptions().AddBroker(mqttBroker)
// 	opts.SetClientID("packet-sensor")
// 	client := MQTT.NewClient(opts)
// 	if token := client.Connect(); token.Wait() && token.Error() != nil {
// 		panic(token.Error())
// 	}
// 	fmt.Println("MQTT 연결 성공")

// 	// SSH 클라이언트 설정
// 	config := &ssh.ClientConfig{
// 		User: sshUser,
// 		Auth: []ssh.AuthMethod{
// 			ssh.Password(sshPass),
// 		},
// 		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
// 		Timeout:         5 * time.Second,
// 	}

// 	// SSH 연결
// 	conn, err := ssh.Dial("tcp", sshHost, config)
// 	if err != nil {
// 		panic(fmt.Sprintf("SSH 연결 실패: %v", err))
// 	}
// 	defer conn.Close()

// 	// 세션 생성 및 명령 실행
// 	session, err := conn.NewSession()
// 	if err != nil {
// 		panic(fmt.Sprintf("SSH 세션 생성 실패: %v", err))
// 	}
// 	defer session.Close()

// 	stdout, err := session.StdoutPipe()
// 	if err != nil {
// 		panic(fmt.Sprintf("StdoutPipe 실패: %v", err))
// 	}

// 	if err := session.Start(tcpdumpCmd); err != nil {
// 		panic(fmt.Sprintf("tcpdump 실행 실패: %v", err))
// 	}

// 	scanner := bufio.NewScanner(stdout)
// 	for scanner.Scan() {
// 		line := scanner.Text()
// 		// fmt.Println(line) // 필요시 주석 해제

// 		// 차량 감지
// 		if strings.Contains(line, "32383134") { // 2814
// 			token := client.Publish(mqttTopicG80, 0, false, "ON")
// 			token.Wait()
// 			fmt.Println("G80 입차 감지!")
// 		}
// 		if strings.Contains(line, "32393933") { // 2993
// 			token := client.Publish(mqttTopicSonata, 0, false, "ON")
// 			token.Wait()
// 			fmt.Println("쏘나타 입차 감지!")
// 		}
// 		// 지하4층 호출 감지 (ON)
// 		if strings.Contains(line, "6672616d6573697a653d51564741") { // framesize=QVGA
// 			token := client.Publish(mqttTopicB4Call, 0, false, "ON")
// 			token.Wait()
// 			fmt.Println("지하4층 호출 감지!")
// 		}
// 		// 지하4층 호출 해제 감지 (OFF)
// 		if strings.Contains(line, "43414e43454c") || strings.Contains(line, "425945") { // CANCEL, BYE
// 			token := client.Publish(mqttTopicB4Call, 0, false, "OFF")
// 			token.Wait()
// 			fmt.Println("지하4층 호출 해제 감지!")
// 		}
// 	}
// 	if err := scanner.Err(); err != nil {
// 		fmt.Println("에러:", err)
// 	}
// }
