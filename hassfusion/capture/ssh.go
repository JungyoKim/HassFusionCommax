package capture

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcapgo"
	"golang.org/x/crypto/ssh"

	"hassfusion/config"
)

func PacketSourceFromReader(r *pcapgo.Reader) *gopacket.PacketSource {
	return gopacket.NewPacketSource(r, r.LinkType())
}

// StartSSHCommand spawns tcpdump via ssh
func StartSSHCommand(cfg *config.Config) (io.Reader, *ssh.Session, *ssh.Client, error) {
	sshCfg := cfg.TCP.SSH

	var authMethods []ssh.AuthMethod
	if sshCfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(sshCfg.Password))
	}
	// Note: private key omitted for simplicity, can be added if requested

	clientConfig := &ssh.ClientConfig{
		User:            sshCfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", sshCfg.Host, clientConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ssh dial err: %w", err)
	}

	// Keep alive
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for range t.C {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				log.Printf("[TCP] SSH keepalive failed (Connection lost): %v", err)
				return // 커넥션이 끊기면 고루틴 종료
			}
		}
	}()

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh session err: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh stdout pipe err: %w", err)
	}

	stderr, _ := session.StderrPipe()
	if stderr != nil {
		// [개선1] 무한 루프 대신 io.Discard를 사용하여 우아하게 버림
		go io.Copy(io.Discard, stderr)
	}

	if err := session.Start(sshCfg.Command); err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh command execution err: %w", err)
	}

	return stdout, session, client, nil
}
