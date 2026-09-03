package shellsidecar

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ProbeHostKey ends the SSH handshake before user authentication. The returned
// key must be independently confirmed; obtaining it is not itself trust.
func ProbeHostKey(ctx context.Context, host string, port int) (map[string]string, error) {
	if strings.TrimSpace(host) == "" || len(host) > 255 || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SSH endpoint")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(strings.Trim(host, "[]"), fmt.Sprint(port)))
	if err != nil {
		return nil, fmt.Errorf("SSH host key probe failed")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var observed ssh.PublicKey
	_, _, _, _ = ssh.NewClientConn(conn, host, &ssh.ClientConfig{User: "host-key-probe", HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		observed = key
		return errors.New("host key probe complete")
	}})
	if observed == nil {
		return nil, fmt.Errorf("SSH host key unavailable")
	}
	return map[string]string{"key": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(observed))), "fingerprint": ssh.FingerprintSHA256(observed), "algorithm": observed.Type()}, nil
}
