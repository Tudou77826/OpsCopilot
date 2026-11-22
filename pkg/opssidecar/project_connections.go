package opssidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/secretstore"
)

const (
	projectSecretService = "aaw-remote-project"
	projectDialTimeout   = 20 * time.Second
)

type projectBastionInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	HostKey  string `json:"hostKey,omitempty"`
}

type projectConnectionInput struct {
	Name     string               `json:"name"`
	Host     string               `json:"host"`
	Port     int                  `json:"port"`
	User     string               `json:"user"`
	Password string               `json:"password,omitempty"`
	HostKey  string               `json:"hostKey,omitempty"`
	Bastion  *projectBastionInput `json:"bastion,omitempty"`
}

type projectBastionDTO struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	User               string `json:"user"`
	HasPassword        bool   `json:"hasPassword"`
	HostKey            string `json:"hostKey,omitempty"`
	HostKeyFingerprint string `json:"hostKeyFingerprint,omitempty"`
}

type projectConnectionDTO struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Host               string             `json:"host"`
	Port               int                `json:"port"`
	User               string             `json:"user"`
	HasPassword        bool               `json:"hasPassword"`
	HostKey            string             `json:"hostKey,omitempty"`
	HostKeyFingerprint string             `json:"hostKeyFingerprint,omitempty"`
	Bastion            *projectBastionDTO `json:"bastion,omitempty"`
	Revision           uint64             `json:"revision"`
}

type hostKeyChallenge struct {
	Status              string `json:"status"` // "host_key_untrusted" | "host_key_mismatch"
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Hop                 int    `json:"hop"` // 0 = target, 1 = bastion
	KeyType             string `json:"keyType"`
	Fingerprint         string `json:"fingerprint"`
	HostKey             string `json:"hostKey"`
	ExpectedFingerprint string `json:"expectedFingerprint,omitempty"`
}

type hostKeyCheckError struct {
	challenge hostKeyChallenge
}

func (e *hostKeyCheckError) Error() string {
	if e.challenge.Status == "host_key_mismatch" {
		return fmt.Sprintf("SSH 主机密钥已变化（%s:%d）", e.challenge.Host, e.challenge.Port)
	}
	return fmt.Sprintf("首次连接 %s:%d，需要确认主机指纹", e.challenge.Host, e.challenge.Port)
}

type pooledProjectConn struct {
	revision uint64
	client   *ssh.Client
	bastion  *ssh.Client
	sftp     *sftp.Client
	mu       sync.Mutex
}

func (p *pooledProjectConn) close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sftp != nil {
		_ = p.sftp.Close()
		p.sftp = nil
	}
	if p.client != nil {
		_ = p.client.Close()
		p.client = nil
	}
	if p.bastion != nil {
		_ = p.bastion.Close()
		p.bastion = nil
	}
}

type projectPool struct {
	mu        sync.Mutex
	revisions map[string]uint64
	conns     map[string]*pooledProjectConn
}

func newProjectPool() *projectPool {
	return &projectPool{
		revisions: map[string]uint64{},
		conns:     map[string]*pooledProjectConn{},
	}
}

func (p *projectPool) revisionOf(connectionID string) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.revisions[connectionID]
}

func (p *projectPool) invalidate(connectionID string) uint64 {
	p.mu.Lock()
	p.revisions[connectionID]++
	rev := p.revisions[connectionID]
	existing := p.conns[connectionID]
	delete(p.conns, connectionID)
	p.mu.Unlock()
	if existing != nil {
		existing.close()
	}
	return rev
}

func (p *projectPool) closeAll() {
	p.mu.Lock()
	all := make([]*pooledProjectConn, 0, len(p.conns))
	for id, conn := range p.conns {
		all = append(all, conn)
		delete(p.conns, id)
	}
	p.mu.Unlock()
	for _, conn := range all {
		conn.close()
	}
}

func projectSecretKey(connectionID string, hop int) string {
	return fmt.Sprintf("%s:hop:%d", connectionID, hop)
}

func hostKeyFingerprint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func (s *Service) secretGet(connectionID string, hop int, fallbackHost, fallbackUser, inline string) string {
	if inline != "" {
		return inline
	}
	if s.secrets != nil && connectionID != "" {
		if val, err := s.secrets.Get(projectSecretService, projectSecretKey(connectionID, hop)); err == nil && val != "" {
			return val
		}
		if fallbackHost != "" && fallbackUser != "" {
			if val, err := s.secrets.Get("opscopilot", fallbackHost+"_"+fallbackUser); err == nil && val != "" {
				return val
			}
		}
	}
	return ""
}

func (s *Service) secretSet(connectionID string, hop int, password string) error {
	if s.secrets == nil || connectionID == "" || password == "" {
		return nil
	}
	return s.secrets.Set(projectSecretService, projectSecretKey(connectionID, hop), password)
}

func (s *Service) secretDelete(connectionID string) {
	if s.secrets == nil || connectionID == "" {
		return
	}
	_ = s.secrets.Delete(projectSecretService, projectSecretKey(connectionID, 0))
	_ = s.secrets.Delete(projectSecretService, projectSecretKey(connectionID, 1))
}

func (s *Service) connectionToDTO(node *connectionstore.Node) projectConnectionDTO {
	cfg := node.Config
	port := cfg.Port
	if port <= 0 {
		port = 22
	}
	dto := projectConnectionDTO{
		ID:                 node.ID,
		Name:               node.Name,
		Host:               cfg.Host,
		Port:               port,
		User:               cfg.User,
		HasPassword:        s.secretGet(node.ID, 0, cfg.Host, cfg.User, cfg.Password) != "",
		HostKey:            strings.TrimSpace(cfg.HostKey),
		HostKeyFingerprint: hostKeyFingerprint(cfg.HostKey),
		Revision:           s.projects.revisionOf(node.ID),
	}
	if cfg.Bastion != nil && strings.TrimSpace(cfg.Bastion.Host) != "" {
		bPort := cfg.Bastion.Port
		if bPort <= 0 {
			bPort = 22
		}
		dto.Bastion = &projectBastionDTO{
			Host:               cfg.Bastion.Host,
			Port:               bPort,
			User:               cfg.Bastion.User,
			HasPassword:        s.secretGet(node.ID, 1, cfg.Bastion.Host, cfg.Bastion.User, cfg.Bastion.Password) != "",
			HostKey:            strings.TrimSpace(cfg.Bastion.HostKey),
			HostKeyFingerprint: hostKeyFingerprint(cfg.Bastion.HostKey),
		}
	}
	return dto
}

func collectSSHConnectionNodes(nodes []*connectionstore.Node, out *[]*connectionstore.Node) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Type == connectionstore.KindConnection && node.Config != nil {
			if node.Config.NormalizedProtocol() == remote.ProtocolSSH {
				*out = append(*out, node)
			}
		} else if node.Type == connectionstore.KindFolder {
			collectSSHConnectionNodes(node.Children, out)
		}
	}
}

func (s *Service) handleProjectConnectionsList(req Request) (Response, bool) {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
	var sshNodes []*connectionstore.Node
	collectSSHConnectionNodes(nodes, &sshNodes)
	sort.SliceStable(sshNodes, func(i, j int) bool {
		return strings.ToLower(sshNodes[i].Name) < strings.ToLower(sshNodes[j].Name)
	})
	dtos := make([]projectConnectionDTO, 0, len(sshNodes))
	for _, node := range sshNodes {
		dtos = append(dtos, s.connectionToDTO(node))
	}
	return okResponse(req.ID, map[string]any{"connections": dtos}), false
}

func buildConnectConfigFromInput(in projectConnectionInput) (remote.ConnectConfig, error) {
	host := strings.TrimSpace(in.Host)
	user := strings.TrimSpace(in.User)
	port := in.Port
	if port == 0 {
		port = 22
	}
	if host == "" || user == "" {
		return remote.ConnectConfig{}, errors.New("主机地址和 SSH 账号必填")
	}
	if port <= 0 || port > 65535 {
		return remote.ConnectConfig{}, errors.New("端口必须在 1 到 65535 之间")
	}
	cfg := remote.ConnectConfig{
		Name:     strings.TrimSpace(in.Name),
		Protocol: remote.ProtocolSSH,
		Host:     host,
		Port:     port,
		User:     user,
		HostKey:  strings.TrimSpace(in.HostKey),
	}
	if in.Bastion != nil && strings.TrimSpace(in.Bastion.Host) != "" {
		bPort := in.Bastion.Port
		if bPort == 0 {
			bPort = 22
		}
		bUser := strings.TrimSpace(in.Bastion.User)
		if bUser == "" {
			return remote.ConnectConfig{}, errors.New("跳板机账号必填")
		}
		if bPort <= 0 || bPort > 65535 {
			return remote.ConnectConfig{}, errors.New("跳板机端口必须在 1 到 65535 之间")
		}
		cfg.Bastion = &remote.ConnectConfig{
			Name:     strings.TrimSpace(in.Bastion.Host),
			Protocol: remote.ProtocolSSH,
			Host:     strings.TrimSpace(in.Bastion.Host),
			Port:     bPort,
			User:     bUser,
			HostKey:  strings.TrimSpace(in.Bastion.HostKey),
		}
	}
	return cfg, nil
}

func (s *Service) handleProjectConnectionCreate(req Request) (Response, bool) {
	var in projectConnectionInput
	if err := json.Unmarshal(req.Params, &in); err != nil {
		return invalidParams(req.ID, "参数解析失败: "+err.Error()), false
	}
	cfg, err := buildConnectConfigFromInput(in)
	if err != nil {
		return invalidParams(req.ID, err.Error()), false
	}
	node, err := s.store.CreateProjectConnection(cfg, "")
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	if in.Password != "" {
		if err := s.secretSet(node.ID, 0, in.Password); err != nil {
			_ = s.store.DeleteNode(node.ID)
			return errorResponse(req.ID, CodeInternal, "保存密码到系统凭据库失败: "+err.Error(), ""), false
		}
	}
	if in.Bastion != nil && in.Bastion.Password != "" {
		if err := s.secretSet(node.ID, 1, in.Bastion.Password); err != nil {
			s.secretDelete(node.ID)
			_ = s.store.DeleteNode(node.ID)
			return errorResponse(req.ID, CodeInternal, "保存跳板机密码失败: "+err.Error(), ""), false
		}
	}
	s.markDirty()
	return okResponse(req.ID, map[string]any{"connection": s.connectionToDTO(node)}), false
}

func (s *Service) handleProjectConnectionUpdate(req Request) (Response, bool) {
	var params struct {
		ID string `json:"id"`
		projectConnectionInput
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ID) == "" {
		return invalidParams(req.ID, "id 必填"), false
	}
	s.mu.Lock()
	existing := findNode(s.store.Snapshot(), params.ID)
	s.mu.Unlock()
	if existing == nil || existing.Type != connectionstore.KindConnection || existing.Config == nil {
		return errorResponse(req.ID, CodeNotFound, "连接不存在", ""), false
	}
	cfg, err := buildConnectConfigFromInput(params.projectConnectionInput)
	if err != nil {
		return invalidParams(req.ID, err.Error()), false
	}
	// 如果地址和端口没变且本次未显式传 hostKey，则保留原已信任的 hostKey；地址/端口变化时清空要求重新确认。
	if cfg.HostKey == "" && existing.Config.Host == cfg.Host && existing.Config.Port == cfg.Port {
		cfg.HostKey = existing.Config.HostKey
	}
	if cfg.Bastion != nil && existing.Config.Bastion != nil && cfg.Bastion.HostKey == "" &&
		existing.Config.Bastion.Host == cfg.Bastion.Host && existing.Config.Bastion.Port == cfg.Bastion.Port {
		cfg.Bastion.HostKey = existing.Config.Bastion.HostKey
	}

	node, err := s.store.UpdateProjectConnection(params.ID, cfg)
	if err != nil {
		if errors.Is(err, connectionstore.ErrNotFound) {
			return errorResponse(req.ID, CodeNotFound, err.Error(), ""), false
		}
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	if params.Password != "" {
		if err := s.secretSet(node.ID, 0, params.Password); err != nil {
			return errorResponse(req.ID, CodeInternal, "更新密码失败: "+err.Error(), ""), false
		}
	}
	if params.Bastion != nil && params.Bastion.Password != "" {
		if err := s.secretSet(node.ID, 1, params.Bastion.Password); err != nil {
			return errorResponse(req.ID, CodeInternal, "更新跳板机密码失败: "+err.Error(), ""), false
		}
	}
	s.projects.invalidate(node.ID)
	s.markDirty()
	return okResponse(req.ID, map[string]any{"connection": s.connectionToDTO(node)}), false
}

func (s *Service) handleProjectConnectionDelete(req Request) (Response, bool) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ID) == "" {
		return invalidParams(req.ID, "id 必填"), false
	}
	if err := s.store.DeleteNode(params.ID); err != nil {
		if errors.Is(err, connectionstore.ErrNotFound) {
			return errorResponse(req.ID, CodeNotFound, err.Error(), ""), false
		}
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.projects.invalidate(params.ID)
	s.secretDelete(params.ID)
	s.markDirty()
	return okResponse(req.ID, map[string]any{"deleted": true}), false
}

type resolvedDialHop struct {
	Host     string
	Port     int
	User     string
	Password string
	HostKey  string
}

func (s *Service) resolveProjectDialConfig(connectionID string, override *projectConnectionInput) (target resolvedDialHop, bastion *resolvedDialHop, err error) {
	var base *remote.ConnectConfig
	if connectionID != "" {
		s.mu.Lock()
		node := findNode(s.store.Snapshot(), connectionID)
		s.mu.Unlock()
		if node == nil || node.Type != connectionstore.KindConnection || node.Config == nil {
			return resolvedDialHop{}, nil, errServerNotFound
		}
		base = node.Config
	}

	if override != nil && strings.TrimSpace(override.Host) != "" {
		cfg, buildErr := buildConnectConfigFromInput(*override)
		if buildErr != nil {
			return resolvedDialHop{}, nil, buildErr
		}
		hostKey := cfg.HostKey
		if hostKey == "" && base != nil && base.Host == cfg.Host && base.Port == cfg.Port {
			hostKey = base.HostKey
		}
		fallbackHost, fallbackUser, inlinePwd := "", "", override.Password
		if base != nil {
			fallbackHost, fallbackUser = base.Host, base.User
			if inlinePwd == "" {
				inlinePwd = base.Password
			}
		}
		pwd := s.secretGet(connectionID, 0, fallbackHost, fallbackUser, inlinePwd)
		target = resolvedDialHop{
			Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: pwd, HostKey: hostKey,
		}
		if cfg.Bastion != nil {
			bHostKey := cfg.Bastion.HostKey
			bFallbackHost, bFallbackUser, bInlinePwd := "", "", ""
			if override.Bastion != nil {
				bInlinePwd = override.Bastion.Password
			}
			if base != nil && base.Bastion != nil {
				if bHostKey == "" && base.Bastion.Host == cfg.Bastion.Host && base.Bastion.Port == cfg.Bastion.Port {
					bHostKey = base.Bastion.HostKey
				}
				bFallbackHost, bFallbackUser = base.Bastion.Host, base.Bastion.User
				if bInlinePwd == "" {
					bInlinePwd = base.Bastion.Password
				}
			}
			bPwd := s.secretGet(connectionID, 1, bFallbackHost, bFallbackUser, bInlinePwd)
			bastion = &resolvedDialHop{
				Host: cfg.Bastion.Host, Port: cfg.Bastion.Port, User: cfg.Bastion.User, Password: bPwd, HostKey: bHostKey,
			}
		}
		return target, bastion, nil
	}

	if base == nil {
		return resolvedDialHop{}, nil, errors.New("缺少连接配置")
	}
	port := base.Port
	if port <= 0 {
		port = 22
	}
	target = resolvedDialHop{
		Host:     base.Host,
		Port:     port,
		User:     base.User,
		Password: s.secretGet(connectionID, 0, base.Host, base.User, base.Password),
		HostKey:  strings.TrimSpace(base.HostKey),
	}
	if base.Bastion != nil && strings.TrimSpace(base.Bastion.Host) != "" {
		bPort := base.Bastion.Port
		if bPort <= 0 {
			bPort = 22
		}
		bastion = &resolvedDialHop{
			Host:     base.Bastion.Host,
			Port:     bPort,
			User:     base.Bastion.User,
			Password: s.secretGet(connectionID, 1, base.Bastion.Host, base.Bastion.User, base.Bastion.Password),
			HostKey:  strings.TrimSpace(base.Bastion.HostKey),
		}
	}
	return target, bastion, nil
}

func makeStrictHostKeyCallback(hop resolvedDialHop, hopIndex int) (ssh.HostKeyCallback, error) {
	var expected ssh.PublicKey
	expectedFP := ""
	if strings.TrimSpace(hop.HostKey) != "" {
		parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hop.HostKey))
		if err != nil {
			return nil, fmt.Errorf("已保存的 SSH 主机密钥格式无效: %w", err)
		}
		expected = parsed
		expectedFP = ssh.FingerprintSHA256(parsed)
	}
	return func(hostname string, remoteAddr net.Addr, key ssh.PublicKey) error {
		actualFP := ssh.FingerprintSHA256(key)
		actualAuthorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		if expected == nil {
			return &hostKeyCheckError{
				challenge: hostKeyChallenge{
					Status:      "host_key_untrusted",
					Host:        hop.Host,
					Port:        hop.Port,
					Hop:         hopIndex,
					KeyType:     key.Type(),
					Fingerprint: actualFP,
					HostKey:     actualAuthorized,
				},
			}
		}
		if !bytes.Equal(expected.Marshal(), key.Marshal()) {
			return &hostKeyCheckError{
				challenge: hostKeyChallenge{
					Status:              "host_key_mismatch",
					Host:                hop.Host,
					Port:                hop.Port,
					Hop:                 hopIndex,
					KeyType:             key.Type(),
					Fingerprint:         actualFP,
					HostKey:             actualAuthorized,
					ExpectedFingerprint: expectedFP,
				},
			}
		}
		return nil
	}, nil
}

func dialSingleHop(ctx context.Context, via *ssh.Client, hop resolvedDialHop, hopIndex int) (*ssh.Client, error) {
	callback, err := makeStrictHostKeyCallback(hop, hopIndex)
	if err != nil {
		return nil, err
	}
	// 当尚未信任主机密钥时，先不提供密码也允许进入握手阶段以获取远端公钥。
	auth := []ssh.AuthMethod{}
	if hop.Password != "" {
		pwd := hop.Password
		auth = append(auth,
			ssh.Password(pwd),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = pwd
				}
				return answers, nil
			}),
		)
	}
	sshCfg := &ssh.ClientConfig{
		User:            hop.User,
		Auth:            auth,
		HostKeyCallback: callback,
		Timeout:         projectDialTimeout,
	}
	host := strings.Trim(hop.Host, "[]")
	addr := net.JoinHostPort(host, fmt.Sprint(hop.Port))

	var rawConn net.Conn
	if via != nil {
		rawConn, err = via.DialContext(ctx, "tcp", addr)
	} else {
		rawConn, err = (&net.Dialer{Timeout: projectDialTimeout}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("无法连接到 %s: %w", addr, err)
	}
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()

	ncc, chans, reqs, err := ssh.NewClientConn(rawConn, addr, sshCfg)
	if err != nil {
		_ = rawConn.Close()
		var hkErr *hostKeyCheckError
		if errors.As(err, &hkErr) {
			return nil, hkErr
		}
		if hop.Password == "" {
			return nil, fmt.Errorf("缺少 SSH 密码（%s@%s）: %w", hop.User, addr, err)
		}
		return nil, fmt.Errorf("SSH 认证或握手失败（%s@%s）: %w", hop.User, addr, err)
	}
	return ssh.NewClient(ncc, chans, reqs), nil
}

func dialProjectSSHAndSFTP(ctx context.Context, target resolvedDialHop, bastion *resolvedDialHop) (*pooledProjectConn, error) {
	var bastionClient *ssh.Client
	var err error
	if bastion != nil {
		bastionClient, err = dialSingleHop(ctx, nil, *bastion, 1)
		if err != nil {
			return nil, err
		}
	}
	targetClient, err := dialSingleHop(ctx, bastionClient, target, 0)
	if err != nil {
		if bastionClient != nil {
			_ = bastionClient.Close()
		}
		return nil, err
	}
	sftpClient, err := sftp.NewClient(targetClient)
	if err != nil {
		_ = targetClient.Close()
		if bastionClient != nil {
			_ = bastionClient.Close()
		}
		return nil, fmt.Errorf("无法开启 SFTP 子系统: %w", err)
	}
	return &pooledProjectConn{
		client:  targetClient,
		bastion: bastionClient,
		sftp:    sftpClient,
	}, nil
}

func (s *Service) handleProjectConnectionTest(req Request) (Response, bool) {
	var params struct {
		ConnectionID string                  `json:"connectionId,omitempty"`
		Config       *projectConnectionInput `json:"config,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "参数解析失败: "+err.Error()), false
	}
	target, bastion, err := s.resolveProjectDialConfig(strings.TrimSpace(params.ConnectionID), params.Config)
	if err != nil {
		if errors.Is(err, errServerNotFound) {
			return errorResponse(req.ID, CodeNotFound, err.Error(), ""), false
		}
		return invalidParams(req.ID, err.Error()), false
	}
	ctx, cancel := context.WithTimeout(context.Background(), projectDialTimeout)
	defer cancel()

	pooled, err := dialProjectSSHAndSFTP(ctx, target, bastion)
	if err != nil {
		var hkErr *hostKeyCheckError
		if errors.As(err, &hkErr) {
			return okResponse(req.ID, hkErr.challenge), false
		}
		code := CodeConnectFailed
		if strings.Contains(err.Error(), "缺少 SSH 密码") {
			code = CodeNoPassword
		}
		return errorResponse(req.ID, code, err.Error(), ""), false
	}
	defer pooled.close()

	homeDir, err := pooled.sftp.Getwd()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = "/"
	}
	return okResponse(req.ID, map[string]any{
		"status":  "ok",
		"homeDir": homeDir,
		"host":    target.Host,
		"port":    target.Port,
		"user":    target.User,
	}), false
}

func (s *Service) withProjectSFTP(connectionID string, fn func(client *sftp.Client) error) error {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return errors.New("connectionId 必填")
	}
	for attempt := 0; attempt < 2; attempt++ {
		s.projects.mu.Lock()
		rev := s.projects.revisions[connectionID]
		existing := s.projects.conns[connectionID]
		s.projects.mu.Unlock()

		if existing == nil || existing.revision != rev {
			target, bastion, err := s.resolveProjectDialConfig(connectionID, nil)
			if err != nil {
				return err
			}
			if target.HostKey == "" {
				return &hostKeyCheckError{
					challenge: hostKeyChallenge{
						Status: "host_key_untrusted",
						Host:   target.Host,
						Port:   target.Port,
						Hop:    0,
					},
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), projectDialTimeout)
			dialed, err := dialProjectSSHAndSFTP(ctx, target, bastion)
			cancel()
			if err != nil {
				return err
			}
			dialed.revision = rev
			s.projects.mu.Lock()
			if s.projects.revisions[connectionID] != rev {
				s.projects.mu.Unlock()
				dialed.close()
				continue
			}
			if old := s.projects.conns[connectionID]; old != nil {
				old.close()
			}
			s.projects.conns[connectionID] = dialed
			existing = dialed
			s.projects.mu.Unlock()
		}

		existing.mu.Lock()
		client := existing.sftp
		var err error
		if client == nil {
			err = errors.New("SFTP 连接已关闭")
		} else {
			err = fn(client)
		}
		existing.mu.Unlock()

		if err == nil {
			return nil
		}
		// 若不是权限或不存在类业务错误，而是底层通道断开，清理缓存并重试一次。
		if attempt == 0 && isTransportError(err) {
			s.projects.mu.Lock()
			if s.projects.conns[connectionID] == existing {
				delete(s.projects.conns, connectionID)
			}
			s.projects.mu.Unlock()
			existing.close()
			continue
		}
		return err
	}
	return errors.New("SSH 连接重试失败")
}

func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *sftp.StatusError
	if errors.As(err, &statusErr) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection lost") ||
		strings.Contains(msg, "closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "reset by peer")
}

// SetSecretStoreForTest 允许测试注入内存凭据库，避免污染系统凭据管理器。
func (s *Service) SetSecretStoreForTest(store secretstore.SecretStore) {
	s.secrets = store
}
