// Package opssession retains connection facts across CLI processes, not live
// remote shells or model history. The platform owns the lifetime of a Session.
package opssession

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/filetxn"
	"opscopilot/pkg/remote"
)

var ErrClosed = errors.New("Session 已关闭，不能恢复或继续操作")
var ErrNotFound = errors.New("Session 不存在，请由平台先初始化")
var ErrBusy = errors.New("Session 正在收尾，尚未确认所有调用停止；请稍后再次关闭")
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type Node struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Kind      string                `json:"kind"`
	SSH       *remote.ConnectConfig `json:"ssh,omitempty"`
	Database  *dbworkspace.Profile  `json:"database,omitempty"`
	Password  *string               `json:"password,omitempty"`
	GatewayID string                `json:"gatewayId,omitempty"`
	Updated   time.Time             `json:"updated"`
}
type State struct {
	Version     int               `json:"version"`
	ID          string            `json:"id"`
	Environment string            `json:"environment"`
	Closed      bool              `json:"closed"`
	Nodes       []Node            `json:"nodes"`
	Secrets     map[string]string `json:"secrets,omitempty"`
}
type Store struct {
	root string
	aead cipher.AEAD
}
type envelope struct {
	Version int    `json:"version"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

func New(root string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("Session 加密密钥必须为 32 字节")
	}
	if root == "" {
		return nil, errors.New("必须配置服务端数据目录")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	return &Store{root: abs, aead: aead}, nil
}
func (s *Store) path(id string) (string, error) {
	if !identifier.MatchString(id) || id == "." || id == ".." {
		return "", errors.New("Session 标识只能包含字母、数字、点、下划线和连字符，最长 128 字符")
	}
	return filepath.Join(s.root, "sessions", id), nil
}
func (s *Store) lock(id string) (func(), error) {
	return s.lockContext(context.Background(), id)
}
func (s *Store) lockContext(ctx context.Context, id string) (func(), error) {
	if _, err := s.path(id); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return filetxn.LockContext(ctx, filepath.Join(s.root, "locks", id))
}
func (s *Store) read(id string) (State, error) {
	dir, err := s.path(id)
	if err != nil {
		return State{}, err
	}
	b, err := filetxn.ReadBounded(filepath.Join(dir, "state.enc"), 8<<20)
	if os.IsNotExist(err) {
		return State{}, ErrNotFound
	}
	if err != nil {
		return State{}, err
	}
	var e envelope
	if json.Unmarshal(b, &e) != nil || e.Version != 1 || len(e.Nonce) != s.aead.NonceSize() {
		return State{}, errors.New("Session 数据格式无效")
	}
	plain, err := s.aead.Open(nil, e.Nonce, e.Data, []byte(id))
	if err != nil {
		return State{}, errors.New("Session 解密失败，请核对部署密钥或数据完整性")
	}
	var state State
	if json.Unmarshal(plain, &state) != nil || state.ID != id || state.Version != 1 {
		return State{}, errors.New("Session 内容无效")
	}
	return state, nil
}
func (s *Store) write(state State) error {
	dir, err := s.path(state.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	plain, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(plain) > 4<<20 {
		return errors.New("Session 数据超过上限")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	b, err := json.Marshal(envelope{Version: 1, Nonce: nonce, Data: s.aead.Seal(nil, nonce, plain, []byte(state.ID))})
	if err != nil {
		return err
	}
	return filetxn.Write(filepath.Join(dir, "state.enc"), b)
}
func (s *Store) Init(id, environment string) (State, error) {
	return s.InitContext(context.Background(), id, environment)
}
func (s *Store) InitContext(ctx context.Context, id, environment string) (State, error) {
	if strings.TrimSpace(environment) == "" || len(environment) > 256 {
		return State{}, errors.New("请提供任务所属业务环境")
	}
	release, err := s.lockContext(ctx, id)
	if err != nil {
		return State{}, err
	}
	defer release()
	state, err := s.read(id)
	if err == nil {
		if state.Closed {
			return State{}, ErrClosed
		}
		if state.Environment != environment {
			return State{}, errors.New("Session 环境不可变更")
		}
		return state, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return State{}, err
	}
	state = State{Version: 1, ID: id, Environment: environment, Nodes: []Node{}, Secrets: map[string]string{}}
	return state, s.write(state)
}
func (s *Store) Inspect(id string) (State, error) {
	return s.InspectContext(context.Background(), id)
}
func (s *Store) InspectContext(ctx context.Context, id string) (State, error) {
	release, err := s.lockContext(ctx, id)
	if err != nil {
		return State{}, err
	}
	defer release()
	return s.read(id)
}

// Put replaces one declared node. Credentials are retained only for the same
// identity. A non-nil empty database password clears that credential; omitted
// or empty SSH password strings preserve the existing password.
func (s *Store) Put(id string, node Node) (Node, error) {
	return s.PutContext(context.Background(), id, node)
}
func (s *Store) PutContext(ctx context.Context, id string, node Node) (Node, error) {
	if node.ID == "" {
		node.ID = uuid.NewString()
	}
	if !identifier.MatchString(node.ID) {
		return Node{}, errors.New("节点标识无效")
	}
	if node.Kind != "ssh" && node.Kind != "database" {
		return Node{}, errors.New("节点类型必须为 ssh 或 database")
	}
	if node.Kind == "ssh" {
		if node.SSH == nil || node.Database != nil {
			return Node{}, errors.New("SSH 节点需要 ssh 配置")
		}
		if err := validateSSH(node.SSH, 0); err != nil {
			return Node{}, err
		}
	}
	if node.Kind == "database" {
		if node.Database == nil || node.SSH != nil {
			return Node{}, errors.New("数据库节点需要 database 配置")
		}
		p := node.Database
		if p.Host == "" || p.User == "" || p.Port < 1 || p.Port > 65535 {
			return Node{}, errors.New("数据库地址、端口和账号不完整")
		}
		if p.Engine != "" && p.Engine != "jdbc" {
			return Node{}, errors.New("服务端数据库使用部署的 JDBC 驱动")
		}
		p.Engine = "jdbc"
		p.ReadOnly = true
		if node.GatewayID == "" {
			node.GatewayID = p.GatewayID
		} else if p.GatewayID != "" && p.GatewayID != node.GatewayID {
			return Node{}, errors.New("数据库跳板标识不一致")
		}
		p.GatewayID = node.GatewayID
	}
	release, err := s.lockContext(ctx, id)
	if err != nil {
		return Node{}, err
	}
	defer release()
	state, err := s.read(id)
	if err != nil {
		return Node{}, err
	}
	if state.Closed {
		return Node{}, ErrClosed
	}
	if node.GatewayID != "" {
		found := false
		for _, n := range state.Nodes {
			if n.ID == node.GatewayID && n.Kind == "ssh" {
				found = true
			}
		}
		if !found {
			return Node{}, errors.New("数据库跳板节点未登记")
		}
	}
	index := -1
	for i, n := range state.Nodes {
		if n.ID == node.ID {
			index = i
			if n.Kind != node.Kind {
				return Node{}, errors.New("不能改变已有节点类型")
			}
			if node.Password == nil && sameDatabase(node.Database, n.Database) {
				node.Password = n.Password
			}
			if node.Kind == "ssh" {
				preserveSSH(node.SSH, n.SSH)
			}
			break
		}
	}
	if index < 0 && len(state.Nodes) >= 256 {
		return Node{}, errors.New("Session 节点数量超过 256 个")
	}
	node.Updated = time.Now().UTC()
	if index >= 0 {
		state.Nodes[index] = node
	} else {
		state.Nodes = append(state.Nodes, node)
	}
	return node, s.write(state)
}
func validateSSH(c *remote.ConnectConfig, depth int) error {
	if depth > 8 {
		return errors.New("跳板链超过 8 层")
	}
	if c.Host == "" || c.User == "" || len(c.Host) > 253 || len(c.User) > 128 || strings.ContainsAny(c.Host+c.User, "\x00\r\n") {
		return errors.New("SSH 地址或账号无效")
	}
	if c.Protocol != "" && c.Protocol != "ssh" {
		return errors.New("服务端仅支持 SSH")
	}
	c.Protocol = "ssh"
	if c.Port == 0 {
		c.Port = 22
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("SSH 端口无效")
	}
	if len(c.Password) > 8192 || len(c.RootPassword) > 8192 {
		return errors.New("SSH 凭据过长")
	}
	if c.Bastion != nil {
		return validateSSH(c.Bastion, depth+1)
	}
	return nil
}
func preserveSSH(next, old *remote.ConnectConfig) {
	if next == nil || old == nil {
		return
	}
	if next.Host == old.Host && next.Port == old.Port && next.User == old.User {
		if next.Password == "" {
			next.Password = old.Password
		}
		if next.RootPassword == "" {
			next.RootPassword = old.RootPassword
		}
		if next.HostKey == "" {
			next.HostKey = old.HostKey
		}
		if next.Bastion == nil {
			next.Bastion = old.Bastion
		}
		preserveSSH(next.Bastion, old.Bastion)
	}
}
func sameDatabase(next, old *dbworkspace.Profile) bool {
	return next != nil && old != nil && next.Host == old.Host && next.Port == old.Port && next.User == old.User && next.DriverID == old.DriverID
}
func (s *Store) Node(id, nodeID string) (Node, error) {
	return s.NodeContext(context.Background(), id, nodeID)
}
func (s *Store) NodeContext(ctx context.Context, id, nodeID string) (Node, error) {
	state, err := s.InspectContext(ctx, id)
	if err != nil {
		return Node{}, err
	}
	if state.Closed {
		return Node{}, ErrClosed
	}
	for _, n := range state.Nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return Node{}, errors.New("节点不存在，请查询当前 Session 的节点列表")
}
func (s *Store) WorkDir(id string) (string, error) {
	return s.WorkDirContext(context.Background(), id)
}
func (s *Store) WorkDirContext(ctx context.Context, id string) (string, error) {
	release, err := s.lockContext(ctx, id)
	if err != nil {
		return "", err
	}
	defer release()
	state, err := s.read(id)
	if err != nil {
		return "", err
	}
	if state.Closed {
		return "", ErrClosed
	}
	dir, _ := s.path(id)
	work := filepath.Join(dir, "work")
	return work, os.MkdirAll(work, 0700)
}

// Begin holds an OS-owned run lock; crashes release it automatically. A tiny
// watcher propagates explicit Session close to the current operation context.
func (s *Store) Begin(parent context.Context, id string) (context.Context, func(), error) {
	if parent.Err() != nil {
		return nil, nil, parent.Err()
	}
	release, err := s.lockContext(parent, id)
	if err != nil {
		return nil, nil, err
	}
	state, err := s.read(id)
	if err != nil || state.Closed {
		release()
		if err == nil {
			err = ErrClosed
		}
		return nil, nil, err
	}
	dir, _ := s.path(id)
	run := filepath.Join(dir, "runs", uuid.NewString())
	unlock, err := filetxn.TryLock(run)
	release()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, e := s.InspectContext(ctx, id)
				if e != nil {
					cancel(e)
					return
				}
				if current.Closed {
					cancel(ErrClosed)
					return
				}
			}
		}
	}()
	finish := func() { once.Do(func() { cancel(context.Canceled); <-done; unlock(); _ = os.Remove(run + ".lock") }) }
	return ctx, finish, nil
}
func (s *Store) Close(ctx context.Context, id string) error {
	release, err := s.lockContext(ctx, id)
	if err != nil {
		return err
	}
	state, err := s.read(id)
	if err != nil {
		release()
		return err
	}
	state.Closed = true
	err = s.write(state)
	release()
	if err != nil {
		return err
	}
	dir, _ := s.path(id)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		entries, e := os.ReadDir(filepath.Join(dir, "runs"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		busy := false
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
				continue
			}
			run := filepath.Join(dir, "runs", strings.TrimSuffix(entry.Name(), ".lock"))
			unlock, e := filetxn.TryLock(run)
			if e != nil {
				busy = true
				continue
			}
			unlock()
			_ = os.Remove(run + ".lock")
		}
		if !busy {
			break
		}
		select {
		case <-ctx.Done():
			return ErrBusy
		case <-ticker.C:
		}
	}
	release, err = s.lockContext(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBusy, err)
	}
	defer release()
	state, err = s.read(id)
	if err != nil {
		return err
	}
	state.Nodes = []Node{}
	state.Secrets = nil
	if err = s.write(state); err != nil {
		return err
	}
	// Both paths are fixed children of the validated Session directory. RemoveAll
	// removes a symlink itself and never traverses its target.
	for _, name := range []string{"work", "runs"} {
		if err = os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("清理 Session 工作文件失败: %w", err)
		}
	}
	return nil
}

func Public(state State) map[string]any {
	nodes := make([]map[string]any, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		item := map[string]any{"id": n.ID, "name": n.Name, "kind": n.Kind, "updated": n.Updated, "gatewayId": n.GatewayID}
		if n.SSH != nil {
			item["route"] = publicRoute(n.SSH)
			item["passwordSet"] = n.SSH.Password != ""
			item["elevationSet"] = n.SSH.RootPassword != ""
		}
		if n.Database != nil {
			item["database"] = n.Database
			item["passwordSet"] = n.Password != nil && *n.Password != ""
		}
		nodes = append(nodes, item)
	}
	return map[string]any{"id": state.ID, "environment": state.Environment, "closed": state.Closed, "nodes": nodes}
}
func publicRoute(c *remote.ConnectConfig) []map[string]any {
	var out []map[string]any
	if c.Bastion != nil {
		out = publicRoute(c.Bastion)
	}
	return append(out, map[string]any{"host": c.Host, "port": c.Port, "user": c.User})
}
