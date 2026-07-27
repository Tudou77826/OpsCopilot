package remote

import (
	"fmt"
)

// Dialer 是连接工厂函数的类型。
//
// 本包不直接 import 任何协议实现包(否则会与 sshclient/telnetclient
// 形成循环依赖:实现包 import remote 定义接口,remote import 实现包做
// 分派)。因此工厂分派通过注册表 + init 注入完成:
//
//   - 各协议实现包在 init() 里调用 RegisterDialer(ProtocolXxx, factoryFunc)
//   - 上层(Dial)根据 config.Protocol 查表分发
//
// 这样 remote 包保持"纯定义",无编译期循环;新协议只需在自己包里 init
// 注册,无需改动 remote 包或上层调用点。
type Dialer func(config *ConnectConfig) (Connection, error)

// dialers 注册表。键为 NormalizedProtocol() 返回值(小写协议标识)。
// 使用 init 注入,Read 时无需加锁——Go 包初始化顺序保证所有 init 在
// 任何普通函数调用前完成。但为防御运行时注册(测试场景),仍用 mu 保护。
var (
	dialers = map[string]Dialer{}
)

// RegisterDialer 注册一个协议的连接工厂。
// 通常在协议实现包的 init() 中调用。重复注册同一协议会 panic,
// 以便尽早发现配置错误。
func RegisterDialer(protocol string, d Dialer) {
	if d == nil {
		panic("remote: RegisterDialer with nil Dialer for " + protocol)
	}
	if _, exists := dialers[protocol]; exists {
		panic("remote: duplicate Dialer registration for " + protocol)
	}
	dialers[protocol] = d
}

// Dial 根据 config 的协议分派到对应的 Dialer 建立连接。
// 协议为空时按 SSH 处理。未注册的协议返回错误。
//
// 这是协议分派的唯一入口。上层(session/ops/app)只调 Dial,不直接
// import 协议实现包,从而与具体协议解耦。
func Dial(config *ConnectConfig) (Connection, error) {
	if config == nil {
		return nil, fmt.Errorf("remote: nil config")
	}
	proto := config.NormalizedProtocol()
	d, ok := dialers[proto]
	if !ok {
		return nil, fmt.Errorf("remote: unsupported protocol %q (未注册或拼写错误)", proto)
	}
	return d(config)
}
