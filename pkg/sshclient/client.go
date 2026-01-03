package sshclient

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
)

type ConnectConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

type Client struct {
	client *ssh.Client
}

func NewClient(config *ConnectConfig) (*Client, error) {
	authMethods := []ssh.AuthMethod{}
	if config.Password != "" {
		authMethods = append(authMethods, ssh.Password(config.Password))
	}

	sshConfig := &ssh.ClientConfig{
		User: config.User,
		Auth: authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use ssh.FixedHostKey or similar
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Client) Run(cmd string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("client is not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return stdoutBuf.String(), nil
}

func (c *Client) StartShell(cols, rows int) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	if c.client == nil {
		return nil, nil, nil, fmt.Errorf("client is not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty("xterm", rows, cols, modes); err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
    
    // Also capture stderr
    stderr, err := session.StderrPipe()
    if err != nil {
        session.Close()
        return nil, nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
    }

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to start shell: %w", err)
	}
    
    // Combine stdout and stderr for simplicity in this reader
    combinedReader := io.MultiReader(stdout, stderr)

	return session, stdin, combinedReader, nil
}
