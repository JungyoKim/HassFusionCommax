package main

import (
	"flag"
	"log"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcapgo"

	"hasstcp/capture"
	"hasstcp/config"
	"hasstcp/httpx"
	"hasstcp/mqtt"
)

func main() {
	configPath := flag.String("config", "config.yaml", "config file path")
	flag.Parse()

	// Load config from file
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("SSH: %s@%s", cfg.SSH.User, cfg.SSH.Host)

	// Initialize MQTT client if broker is configured
	if cfg.MQTT.Broker != "" {
		mqttClient, err := mqtt.NewClient(
			cfg.MQTT.Broker,
			cfg.MQTT.ClientID,
			cfg.MQTT.Username,
			cfg.MQTT.Password,
			cfg.MQTT.Topic,
		)
		if err != nil {
			log.Fatalf("mqtt init: %v", err)
		}
		defer mqttClient.Close()
		httpx.SetMQTTClient(mqttClient)
		log.Printf("MQTT enabled: %s -> %s", cfg.MQTT.Broker, cfg.MQTT.Topic)
	}

	// Set local CIDRs if configured
	if len(cfg.Network.LocalCIDRs) > 0 {
		httpx.SetLocalCIDRs(cfg.Network.LocalCIDRs)
	}

	// Start SSH capture
	rc, sess, client, err := capture.StartSSHCommand(capture.SSHConfig{
		Host:      cfg.SSH.Host,
		User:      cfg.SSH.User,
		Password:  cfg.SSH.Password,
		KeyPath:   cfg.SSH.Key,
		Command:   cfg.SSH.Command,
		Timeout:   10 * time.Second,
		KeepAlive: 15 * time.Second,
	})
	if err != nil {
		log.Fatalf("ssh start: %v", err)
	}
	defer sess.Close()
	defer client.Close()
	defer rc.Close()

	pr, err := pcapgo.NewReader(rc)
	if err != nil {
		log.Fatalf("pcap reader: %v", err)
	}
	ps := capture.PacketSourceFromReader(pr)
	inCh := httpx.Drain(ps)

	outCh := make(chan gopacket.Packet, 512)
	go func() {
		defer close(outCh)
		var c uint64
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				log.Printf("packets seen=%d", c)
			case pkt, ok := <-inCh:
				if !ok {
					return
				}
				c++
				outCh <- pkt
			}
		}
	}()

	httpx.AssemblePackets(outCh)
}
