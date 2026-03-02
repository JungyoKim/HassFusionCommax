package httpx

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/tcpassembly"

	"hassfusion/config"
	"hassfusion/ws"
)

var wsServer *ws.Server
var localNets []*net.IPNet

func Setup(server *ws.Server, cfg *config.Config) {
	wsServer = server

	// Setup Local CIDRs
	for _, c := range cfg.TCP.LocalCIDRs {
		_, ipnet, err := net.ParseCIDR(c)
		if err == nil {
			localNets = append(localNets, ipnet)
		}
	}

	// Listen for WS commands (door open)
	wsServer.RegisterHandler("button", func(msg ws.WSMsg) {
		if strings.HasPrefix(msg.DeviceID, "door_") && msg.Action == "press" {
			floor := strings.ToUpper(strings.TrimPrefix(msg.DeviceID, "door_"))
			OpenDoor(floor, cfg) // OpenDoor 함수는 별도 파일(예: door.go)에 있다고 가정
		}
	})
}

// AssemblePackets consumes packets and assembles TCP streams for HTTP heuristics.
func AssemblePackets(packets chan gopacket.Packet) {
	streamFactory := &httpStreamFactory{}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case packet, ok := <-packets:
			if !ok {
				return
			}

			if tcp := packet.Layer(layers.LayerTypeTCP); tcp != nil {
				tcpLayer := tcp.(*layers.TCP)
				netFlow := packet.NetworkLayer().NetworkFlow()
				assembler.AssembleWithTimestamp(
					netFlow,
					tcpLayer,
					packet.Metadata().Timestamp,
				)
			}
		case <-ticker.C:
			// Flush old connections to prevent memory leak
			assembler.FlushOlderThan(time.Now().Add(-2 * time.Minute))
		}
	}
}

// httpStreamFactory ties into gopacket's TCP assembler
type httpStreamFactory struct{}

func (h *httpStreamFactory) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	return &httpStream{
		src: fmt.Sprintf("%s:%s", netFlow.Src(), tcpFlow.Src()),
		dst: fmt.Sprintf("%s:%s", netFlow.Dst(), tcpFlow.Dst()),
	}
}

// httpStream will handle the chunks
type httpStream struct {
	src string
	dst string
	buf bytes.Buffer
}

func (h *httpStream) Reassembled(reassemblies []tcpassembly.Reassembly) {
	for _, r := range reassemblies {
		h.buf.Write(r.Bytes)
	}

	// [버그 수정 1] HTTP 파싱 실패 시 버퍼가 무한정 커지는 OOM(메모리 초과) 현상 방지
	if h.buf.Len() > 1024*1024 { // 1MB 초과 시 안전하게 초기화
		h.buf.Reset()
		return
	}

	h.parse()
}

func (h *httpStream) parse() {
	buf := h.buf.Bytes()
	if len(buf) == 0 {
		return
	}

	// Try to parse basic HTTP Request/Response
	r := bufio.NewReader(bytes.NewReader(buf))
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)

	for {
		l, err := r.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
	}

	body, _ := io.ReadAll(r)

	if bytes.Contains(body, []byte("parkService")) {
		if detectParkService(body) {
			// [버그 수정 2] 정상 처리 완료 후 남은 찌꺼기 데이터로 인한 재파싱 실패 방지
			h.buf.Reset()
		}
	}
}

func (h *httpStream) ReassemblyComplete() {
	// [버그 수정 3] 스트림 종료 시 버퍼를 비워 메모리 즉각 반환
	h.buf.Reset()
}

func DirectionGuess(srcIP, dstIP net.IP) string {
	if srcIP == nil || dstIP == nil {
		return "?"
	}
	srcLocal := isLocal(srcIP)
	dstLocal := isLocal(dstIP)
	if srcLocal && !dstLocal {
		return "OUTBOX"
	}
	if !srcLocal && dstLocal {
		return "INBOX"
	}
	return "INTERNAL"
}

func isLocal(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	for _, n := range localNets {
		if n.Contains(ip) {
			return true
		}
	}
	if ip.IsPrivate() {
		return true
	}
	return false
}

// --- Park Service Detect --- //

type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	ParkService *parkService `xml:"parkService"`
}

type parkService struct {
	In parkData `xml:"in"`
}

type parkData struct {
	Time  string `xml:"time"`
	Type  string `xml:"type"`
	CarNo string `xml:"carNo"`
}

// [버그 수정 4] 전역 Map 동시성 에러(Panic) 방지를 위한 Mutex 래핑 구조체 도입
var parkEventManager = struct {
	sync.Mutex
	events map[string]time.Time
}{
	events: make(map[string]time.Time),
}

// [버그 수정 5] 방치된 Map이 메모리를 다 잡아먹는 것을 막기 위한 주기적 삭제 로직
func init() {
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			now := time.Now()

			parkEventManager.Lock()
			for carNo, t := range parkEventManager.events {
				if now.Sub(t) > 1*time.Hour {
					delete(parkEventManager.events, carNo)
				}
			}
			parkEventManager.Unlock()
		}
	}()
}

// [변경점] 파싱 성공 여부를 알리기 위해 반환 타입을 bool로 수정
func detectParkService(httpData []byte) bool {
	if wsServer == nil {
		return false
	}

	var env soapEnvelope
	if err := xml.Unmarshal(httpData, &env); err != nil {
		return false
	}

	if env.Body.ParkService != nil {
		data := env.Body.ParkService.In

		eventType := "unknown"
		switch data.Type {
		case "1":
			eventType = "parkIn"
		case "2":
			eventType = "parkOut"
		}

		if eventType == "parkIn" {
			// Deduplicate events across streams (안전한 Lock 처리)
			parkEventManager.Lock()
			if lastEvent, ok := parkEventManager.events[data.CarNo]; ok {
				if time.Since(lastEvent) < 5*time.Second {
					parkEventManager.Unlock()
					return true // 중복이지만 처리는 된 것이므로 true 반환
				}
			}
			parkEventManager.events[data.CarNo] = time.Now()
			parkEventManager.Unlock()

			log.Printf("[PARKING DETECTED] %s: %s", eventType, data.CarNo)

			// Broadcast Parking Entry (individual and global)
			// Individual
			wsServer.Broadcast(ws.WSMsg{
				Type:     "event",
				Domain:   "sensor",
				DeviceID: "parking_" + data.CarNo,
				State:    eventType,
				Attributes: map[string]interface{}{
					"timestamp": data.Time,
					"car_no":    data.CarNo,
				},
			})

			// Global for the central HA sensor
			wsServer.Broadcast(ws.WSMsg{
				Type:     "event",
				Domain:   "sensor",
				DeviceID: "parking_events_all",
				State:    eventType,
				Attributes: map[string]interface{}{
					"timestamp": data.Time,
					"car_no":    data.CarNo,
				},
			})
		}
		return true // 성공적으로 처리됨
	}
	return false
}
