# MCP 配置说明

## 概述

OpsCopilot 支持 MCP (Model Context Protocol) 协议，允许 AI 调用配置的诊断工具进行深度分析。

## 配置文件

MCP 配置文件名为 `mcp.json`，应放置在应用的 logs 目录中（通常与 `config.json` 同目录）。

### 配置格式

```json
{
  "mcpServers": {
    "服务器名称": {
      "command": "可执行文件路径",
      "args": ["参数1", "参数2"],
      "env": {
        "环境变量名": "环境变量值"
      }
    }
  }
}
```

### 配置项说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `mcpServers` | object | 是 | MCP 服务器配置映射表 |
| `command` | string | 是 | 服务器可执行文件的绝对路径 |
| `args` | array | 否 | 启动参数列表，默认为空数组 `[]` |
| `env` | object | 否 | 环境变量映射表，默认为空 |

### 示例配置

#### 1. 基础配置

```json
{
  "mcpServers": {
    "diagnostic-server": {
      "command": "D:\\dev\\workspace-go\\OpsCopilot\\tools\\mcp-test\\diagnostic-server.exe",
      "args": []
    }
  }
}
```

#### 2. 多服务器配置

```json
{
  "mcpServers": {
    "echo-server": {
      "command": "D:\\dev\\workspace-go\\OpsCopilot\\tools\\mcp-test\\echo-server.exe",
      "args": []
    },
    "diagnostic-server": {
      "command": "D:\\dev\\workspace-go\\OpsCopilot\\tools\\mcp-test\\diagnostic-server.exe",
      "args": []
    }
  }
}
```

#### 3. 带环境变量的配置

```json
{
  "mcpServers": {
    "custom-server": {
      "command": "C:\\path\\to\\server.exe",
      "args": ["--port", "8080"],
      "env": {
        "API_KEY": "your-api-key",
        "LOG_LEVEL": "debug"
      }
    }
  }
}
```

## 示例 MCP 服务器

项目提供了两个示例 MCP 服务器：

### 1. Echo Server

简单的回显服务器，用于测试 MCP 连接。

**位置**: `tools/mcp-test/echo-server.exe`

**工具**:
- `echo` - 回显输入的文本

### 2. Diagnostic Server

诊断服务器，提供常用的系统诊断工具。

**位置**: `tools/mcp-test/diagnostic-server.exe`

**工具**:
- `run_command` - 执行本地命令
- `read_file` - 读取文件内容
- `write_file` - 写入文件内容

## 使用方式

### 1. 配置 MCP 服务器

1. 复制 `tools/mcp.json.example` 到 logs 目录并重命名为 `mcp.json`
2. 根据实际情况修改可执行文件路径
3. 重启 OpsCopilot 应用

### 2. 启用 MCP 工具增强

在问题定位面板中，勾选 "MCP 工具增强" 选项。

### 3. 查看状态

MCP 服务器状态会在问题定位面板中显示：
- 🔧 MCP 工具增强已启用 - MCP 连接正常
- ⚠️ 部分 MCP 服务器未启动 - 存在配置但未成功启动的服务器

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

参考 `tools/mcp-test/` 目录下的示例代码开发自定义 MCP 服务器：

- `echo-server.go` - 简单的 echo 工具实现
- `diagnostic-server.go` - 完整的诊断工具实现

MCP 协议规范：https://modelcontextprotocol.io/
