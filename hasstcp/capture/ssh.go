package capture

import (
	"bufio"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHConfig struct {
	Host      string
	User      string
	Password  string
	KeyPath   string
	Command   string
	Timeout   time.Duration
	KeepAlive time.Duration
}

// StartSSHCommand connects to ssh and runs the provided tcpdump command, returning a reader for stdout.
func StartSSHCommand(cfg SSHConfig) (io.ReadCloser, *ssh.Session, *ssh.Client, error) {
	authMethods := []ssh.AuthMethod{}
	if cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			// try without passphrase parsing via x509 to give clearer error
			if _, e2 := x509.ParsePKCS1PrivateKey(key); e2 != nil {
				return nil, nil, nil, fmt.Errorf("parse private key: %w", err)
			}
			return nil, nil, nil, fmt.Errorf("parse private key (maybe needs passphrase): %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}

	if _, _, err := net.SplitHostPort(cfg.Host); err != nil {
		cfg.Host = net.JoinHostPort(cfg.Host, "22")
	}

	client, err := ssh.Dial("tcp", cfg.Host, sshConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ssh dial: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh new session: %w", err)
	}

    session.Stderr = os.Stderr
    stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// ensure unbuffered streaming
	reader := bufio.NewReaderSize(stdout, 64*1024)

	if err := session.Start(cfg.Command); err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh start: %w", err)
	}

	rc := io.NopCloser(reader)
	return rc, session, client, nil
}
