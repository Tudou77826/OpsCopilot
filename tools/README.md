# MCP 工具扩展

OpsCopilot 支持 MCP (Model Context Protocol) 协议，允许 AI 调用外部诊断工具进行深度分析。

## 配置方式

MCP 配置文件名为 `mcp.json`，放置在应用运行目录（通常与可执行文件同目录）。

### 配置格式

```json
{
  "mcpServers": {
    "服务器名称": {
      "command": "可执行文件绝对路径",
      "args": ["参数1", "参数2"],
      "env": {
        "环境变量名": "值"
      }
    }
  }
}
```

### 配置字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `mcpServers` | object | 是 | MCP 服务器配置映射表 |
| `command` | string | 是 | 服务器可执行文件的绝对路径 |
| `args` | array | 否 | 启动参数，默认空数组 |
| `env` | object | 否 | 环境变量，默认空 |

### 配置示例

```json
{
  "mcpServers": {
    "diagnostic-server": {
      "command": "/path/to/diagnostic-server",
      "args": [],
      "env": {
        "LOG_LEVEL": "debug"
      }
    }
  }
}
```

## 使用步骤

1. 创建 `mcp.json` 配置文件
2. 配置 MCP 服务器路径和参数
3. 启动 OpsCopilot 应用
4. 在问题定位面板中启用 "MCP 工具增强" 选项

## 故障排查

### MCP 服务器未启动

1. 检查 `mcp.json` 文件路径是否正确
2. 检查可执行文件路径是否存在
3. 查看应用日志（`logs/opscopilot.log`）获取详细错误信息

### 工具调用失败

1. 确认 MCP 服务器正在运行
2. 检查工具参数是否正确
3. 查看 agent 日志了解具体错误

## 开发自定义 MCP 服务器

MCP 服务器需实现 JSON-RPC 2.0 通信（通过 stdin/stdout），支持以下方法：

- `initialize`: 初始化握手
- `tools/list`: 列出可用工具
- `tools/call`: 调用指定工具

### 工具定义示例

```go
// 工具定义示例
{
    "name": "run_command",
    "description": "执行本地命令",
    "inputSchema": {
        "type": "object",
        "properties": {
            "command": {"type": "string"}
        },
        "required": ["command"]
    }
}
```

## 协议参考

- MCP 官方规范：https://modelcontextprotocol.io/
