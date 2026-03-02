package capture

import (
	"fmt"
	"io"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcapgo"
)

// ReaderFromStdin wraps stdin expecting pcap stream (e.g., tcpdump -w -)
func ReaderFromStdin(stdin io.Reader) (*pcapgo.Reader, error) {
	r, err := pcapgo.NewReader(stdin)
	if err != nil {
		return nil, fmt.Errorf("pcap reader: %w", err)
	}
	return r, nil
}

// PacketSourceFromReader converts a pcapgo.Reader into a gopacket PacketSource
func PacketSourceFromReader(r *pcapgo.Reader) *gopacket.PacketSource {
	return gopacket.NewPacketSource(r, r.LinkType())
}


