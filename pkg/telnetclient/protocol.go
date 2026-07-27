package telnetclient

// Telnet 协议常量(RFC 854 / 855 / 856 / 857 / 1073 等)。
//
// IAC (Interpret As Command) = 0xFF 是 telnet 协议的转义标记:
// 数据流中出现的 0xFF 必须成对(0xFF 0xFF 表示一个数据字节 0xFF),
// 而 0xFF 后跟命令字节则表示协商/控制指令。
const (
	cmdIAC  = 255 // Interpret As Command — 转义标记
	cmdDONT = 254 // 拒绝对方启用某选项
	cmdDO   = 253 // 要求对方启用某选项
	cmdWONT = 252 // 拒绝继续启用某选项
	cmdWILL = 251 // 声明愿意启用某选项
	cmdSB   = 250 // Subnegotiation Begin — 子协商开始
	cmdGA   = 249 // Go Ahead
	cmdEL   = 248 // Erase Line
	cmdEC   = 247 // Erase Character
	cmdAYT  = 246 // Are You There
	cmdAO   = 245 // Abort Output
	cmdIP   = 244 // Interrupt Process
	cmdBRK  = 243 // Break
	cmdDM   = 242 // Data Mark
	cmdNOP  = 241 // No Operation — 用于探活
	cmdSE   = 240 // Subnegotiation End — 子协商结束
)

// telnet 选项码。仅列出本客户端需要处理的;其余选项统一拒绝(WONT/DONT),
// 避免被对端拖入我们不支持的协商。
const (
	optBinary = 0   // 8-bit binary transmission
	optEcho   = 1   // 远端回显(对方 echo 我们发的字符)
	optSGA    = 3   // Suppress Go Ahead(现代终端几乎都启用)
	optTTYPE  = 24  // Terminal Type
	optNAWS   = 31  // Negotiate About Window Size — 终端尺寸通知(RFC 1073)
	optLINEMODE = 34 // Line mode
)

// isNegotiation 判断一个 IAC 命令字节是否为三字节协商(WILL/WONT/DO/DONT)。
func isNegotiation(cmd byte) bool {
	return cmd == cmdWILL || cmd == cmdWONT || cmd == cmdDO || cmd == cmdDONT
}
