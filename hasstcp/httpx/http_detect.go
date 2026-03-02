package httpx

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	tcpassembly "github.com/google/gopacket/tcpassembly"

	"hasstcp/mqtt"
)

type HTTPEvent struct {
	Timestamp   time.Time
	Flow        string
	Direction   string // inbound|outbound
	Method      string
	Path        string
	StatusCode  int
	Host        string
	ContentType string
}

// httpStreamFactory ties into gopacket's TCP assembler
type httpStreamFactory struct{}

func (h *httpStreamFactory) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	src := netFlow.Src().String() + ":" + tcpFlow.Src().String()
	dst := netFlow.Dst().String() + ":" + tcpFlow.Dst().String()
	return &httpStream{src: src, dst: dst}
}

type httpStream struct {
	src string
	dst string
	buf bytes.Buffer
}

func (s *httpStream) Reassembled(reassemblies []tcpassembly.Reassembly) {
	for _, r := range reassemblies {
		if len(r.Bytes) == 0 {
			continue
		}
		s.buf.Write(r.Bytes)
		s.parse()
	}
}

func (s *httpStream) parse() {
	// simple HTTP start-line sniffer (does not fully parse)
	r := bufio.NewReader(bytes.NewReader(s.buf.Bytes()))
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if strings.HasPrefix(line, "HTTP/") {
		// response: e.g., HTTP/1.1 200 OK
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			dir := directionFromAddrs(s.src, s.dst)
			body := extractBodyPreview(s.buf.Bytes())
			if body != "" {
				fmt.Printf("%s %s http response %s -> %s %s\npayload: %s\n", time.Now().Format(time.RFC3339), dir, s.src, s.dst, parts[1], body)
			} else {
				fmt.Printf("%s %s http response %s -> %s %s\n", time.Now().Format(time.RFC3339), dir, s.src, s.dst, parts[1])
			}
			// Check for parkService SOAP response
			detectParkService(s.buf.Bytes(), dir, s.src, s.dst, false)
			// Check for doorbell (pushVList) SOAP response
			detectDoorbell(s.buf.Bytes(), dir, s.src, s.dst, false)
			s.buf.Reset()
		}
		return
	}
	// request: e.g., GET /path HTTP/1.1
	methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"}
	for _, m := range methods {
		if strings.HasPrefix(line, m+" ") {
			parts := strings.Fields(line)
			dir := directionFromAddrs(s.src, s.dst)
			path := ""
			if len(parts) >= 2 {
				path = parts[1]
			}
			body := extractBodyPreview(s.buf.Bytes())
			if body != "" {
				fmt.Printf("%s %s http request %s -> %s %s %s\npayload: %s\n", time.Now().Format(time.RFC3339), dir, s.src, s.dst, m, path, body)
			} else {
				fmt.Printf("%s %s http request %s -> %s %s %s\n", time.Now().Format(time.RFC3339), dir, s.src, s.dst, m, path)
			}
			// Check for parkService SOAP request
			detectParkService(s.buf.Bytes(), dir, s.src, s.dst, true)
			// Check for doorbell (pushVList) SOAP request
			detectDoorbell(s.buf.Bytes(), dir, s.src, s.dst, true)
			s.buf.Reset()
			return
		}
	}
}

func (s *httpStream) ReassemblyComplete() {}

// AssemblePackets consumes packets and assembles TCP streams for HTTP heuristics.
func AssemblePackets(packets chan gopacket.Packet) {
	streamFactory := &httpStreamFactory{}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case pkt, ok := <-packets:
			if !ok {
				return
			}
			network := pkt.NetworkLayer()
			transport := pkt.TransportLayer()
			if network == nil || transport == nil {
				continue
			}
			tcp, _ := transport.(*layers.TCP)
			if tcp == nil {
				continue
			}
			assembler.AssembleWithTimestamp(network.NetworkFlow(), tcp, pkt.Metadata().Timestamp)
		case <-ticker.C:
			assembler.FlushWithOptions(tcpassembly.FlushOptions{CloseAll: false})
		}
	}
}

var localNets []*net.IPNet

// SetLocalCIDRs configures local networks used to decide traffic direction.
func SetLocalCIDRs(cidrs []string) {
	var nets []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			nets = append(nets, n)
		}
	}
	localNets = nets
}

func DirectionGuess(srcIP, dstIP net.IP) string {
	if isLocal(srcIP) && !isLocal(dstIP) {
		return "outbound"
	}
	if !isLocal(srcIP) && isLocal(dstIP) {
		return "inbound"
	}
	return "unknown"
}

func isLocal(ip net.IP) bool {
	// Prefer configured localNets; fallback to RFC1918/link-local/ULA
	nets := localNets
	if len(nets) == 0 {
		defaults := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16",
			"fd00::/8",
		}
		for _, c := range defaults {
			_, n, _ := net.ParseCIDR(c)
			nets = append(nets, n)
		}
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Drain copies packets from a source into a channel, closing when done.
func Drain(ps *gopacket.PacketSource) chan gopacket.Packet {
	ch := make(chan gopacket.Packet, 512)
	go func() {
		defer close(ch)
		for pkt := range ps.Packets() {
			ch <- pkt
		}
	}()
	return ch
}

// CopyStream copies pcap bytes to a writer (debug helper)
func CopyStream(r io.Reader, w io.Writer) error {
	_, err := io.Copy(w, r)
	return fmt.Errorf("copy: %w", err)
}

func directionFromAddrs(src, dst string) string {
	sHost := hostOnly(src)
	dHost := hostOnly(dst)
	sIP := net.ParseIP(sHost)
	dIP := net.ParseIP(dHost)
	if sIP == nil || dIP == nil {
		return "unknown"
	}
	return DirectionGuess(sIP, dIP)
}

func hostOnly(addr string) string {
	// expects "host:port" or "[v6]:port"
	if strings.HasPrefix(addr, "[") {
		if i := strings.LastIndex(addr, "]:"); i != -1 {
			return strings.Trim(addr[:i+0], "[]")
		}
	}
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

// extractBodyPreview returns up to 1024 bytes after the first blank line (\r\n\r\n)
// If body looks binary, return hex dump of the first bytes to avoid console issues.
func extractBodyPreview(b []byte) string {
	idx := bytes.Index(b, []byte("\r\n\r\n"))
	if idx == -1 {
		return ""
	}
	body := b[idx+4:]
	if len(body) == 0 {
		return ""
	}
	if len(body) > 1024 {
		body = body[:1024]
	}
	if isMostlyText(body) {
		// sanitize to printable
		s := string(body)
		s = strings.ReplaceAll(s, "\r", "\\r")
		s = strings.ReplaceAll(s, "\n", "\\n")
		return s
	}
	return hex.EncodeToString(body)
}

func isMostlyText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	var printable int
	for _, c := range b {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 32 && c < 127) {
			printable++
		}
	}
	return float64(printable)/float64(len(b)) > 0.9
}

// SOAP parkService structures
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	ParkService         *parkService         `xml:"parkService"`
	ParkServiceResponse *parkServiceResponse `xml:"parkServiceResponse"`
	PushVList           *pushVList           `xml:"pushVList"`
	PushVListResponse   *pushVListResponse   `xml:"pushVListResponse"`
}

type parkService struct {
	In parkData `xml:"in"`
}

type parkServiceResponse struct {
	Out string `xml:"out"`
}

type parkData struct {
	Time  string `xml:"time"`
	Type  string `xml:"type"`
	CarNo string `xml:"carNo"`
}

type pushVList struct {
	IP   string `xml:"ip"`
	Port string `xml:"port"`
}

type pushVListResponse struct {
	Out string `xml:"out"`
}

var mqttClient *mqtt.Client
var lastParkEvents = make(map[string]string)    // carNo -> timestamp (중복 방지)
var lastDoorbellEvents = make(map[string]int64) // floor -> unix timestamp (중복 방지)

// SetMQTTClient sets the global MQTT client for publishing events
func SetMQTTClient(client *mqtt.Client) {
	mqttClient = client
}

// detectParkService checks if the HTTP payload contains SOAP parkService and prints vehicle info
func detectParkService(httpData []byte, dir, src, dst string, isRequest bool) {
	// Extract body after HTTP headers
	idx := bytes.Index(httpData, []byte("\r\n\r\n"))
	if idx == -1 {
		return
	}
	body := httpData[idx+4:]
	if len(body) == 0 {
		return
	}

	// Check if it's SOAP/XML
	if !bytes.Contains(body, []byte("parkService")) {
		return
	}

	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return
	}

	if isRequest && env.Body.ParkService != nil {
		data := env.Body.ParkService.In
		eventType := "알 수 없음"
		eventTypeRaw := data.Type
		if strings.Contains(data.Type, "parkIn") {
			eventType = "입차"
			eventTypeRaw = "parkIn"
		} else if strings.Contains(data.Type, "parkOut") {
			eventType = "출차"
			eventTypeRaw = "parkOut"
		}

		// 중복 감지: SOAP 패킷의 타임스탬프로 중복 확인
		if lastTime, exists := lastParkEvents[data.CarNo]; exists && lastTime == data.Time {
			// 같은 차량, 같은 타임스탬프 = 중복 패킷
			fmt.Printf("[중복 무시] 차량 %s 패킷 (번호판: %s, 시간: %s)\n",
				eventType, data.CarNo, data.Time)
			return
		}

		// 새로운 이벤트로 기록
		lastParkEvents[data.CarNo] = data.Time

		fmt.Printf("\n🚗 [차량 %s 패킷 감지] 번호판: %s | 시간: %s | %s -> %s (%s)\n\n",
			eventType, data.CarNo, data.Time, src, dst, dir)

		// Publish to MQTT if client is configured
		if mqttClient != nil {
			ts, _ := time.Parse(time.RFC3339, data.Time)
			if ts.IsZero() {
				ts = time.Now()
			}
			event := mqtt.ParkEvent{
				EventType:   eventTypeRaw,
				CarNo:       data.CarNo,
				Timestamp:   ts,
				Direction:   dir,
				Source:      src,
				Destination: dst,
			}
			if err := mqttClient.PublishParkEvent(event); err != nil {
				fmt.Printf("MQTT publish error: %v\n", err)
			}
		}
	}
}
