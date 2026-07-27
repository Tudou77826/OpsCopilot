// Package remote 定义协议无关的远程连接抽象层。
//
// 本包是 OpsCopilot 多协议支持的演进锚点:所有协议(SSH、Telnet、未来
// 的串口/RDP 等)都实现本包的 Connection 接口,上层(session、ops、app)
// 只依赖接口而非具体类型。新增协议时仅需在 factory.go 注册一个 case,
// 上层零感知。
//
// ConnectConfig 是协议无关的连接配置,也是 sessions.json 持久化的唯一
// 真相源。JSON tag 使用下划线风格(root_password 等),与历史 sessions.json
// 格式保持一致,确保老数据零迁移。
package remote

// ConnectConfig 是协议无关的远程连接配置。
//
// 字段与 JSON tag 与历史 pkg/sshclient.ConnectConfig 完全一致,以保证
// sessions.json 向后兼容(老数据反序列化后 Protocol 为空,所有判断点
// 按 SSH 处理)。
//
// 注意:本结构体用于持久化层(sessions.json)和协议实现层。Wails 前端
// 通信使用的 app.ConnectConfig 是独立结构体(驼峰 JSON tag),二者在
// app.go 中显式转换,与历史行为保持一致。
type ConnectConfig struct {
	Name         string         `json:"name"`
	Protocol     string         `json:"protocol,omitempty"` // 空值或 "ssh" 走 SSH;"telnet" 走 Telnet
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	User         string         `json:"user"`
	Password     string         `json:"password"`
	RootPassword string         `json:"root_password"`
	Bastion      *ConnectConfig `json:"bastion"`
	Group        string         `json:"group,omitempty"` // UI 分组
}

// 协议常量。比较时统一使用这些常量,避免字符串散落。
const (
	ProtocolSSH    = "ssh"
	ProtocolTelnet = "telnet"
)

// NormalizedProtocol 返回归一化后的协议字符串:空值视为 SSH。
// 所有协议判断点应调用本方法,而非直接读 Protocol 字段。
func (c *ConnectConfig) NormalizedProtocol() string {
	if c == nil || c.Protocol == "" {
		return ProtocolSSH
	}
	return c.Protocol
}
