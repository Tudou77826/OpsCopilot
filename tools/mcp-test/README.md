# MCP 诊断服务器示例

这是一个参考实现，展示如何编写 MCP (Model Context Protocol) 服务器。

## 功能

此 MCP 服务器提供以下诊断工具：

1. **get_system_info**: 获取系统诊断信息
2. **read_log_file**: 读取日志文件内容
3. **check_process**: 检查进程状态

## 使用方法

### 1. 编译服务器

```bash
cd tools/mcp-test
go build -o diagnostic-server.exe .
```

### 2. 在 OpsCopilot 中配置

1. 打开 OpsCopilot 设置
2. 进入"高级功能"选项卡
3. 在"MCP 服务器路径"中填入：
   ```
   D:\dev\workspace-go\OpsCopilot\tools\mcp-test\diagnostic-server.exe
   ```

### 3. 使用 MCP 工具

配置完成后，在问题排查面板中提问，AI 会自动调用 MCP 工具进行诊断。

## MCP 协议实现

此服务器实现了以下 MCP 协议方法：

- `initialize`: 初始化握手
- `tools/list`: 列出可用工具
- `tools/call`: 调用指定工具

## 开发自己的 MCP 服务器

1. 实现 JSON-RPC 2.0 通信（通过 stdin/stdout）
2. 支持 `initialize`、`tools/list`、`tools/call` 方法
3. 定义工具的输入 schema（JSON Schema 格式）
4. 返回符合 MCP 规范的响应

参考 `diagnostic-server.go` 源码了解更多细节。
