# 迭代二：高级连接管理与安全存储开发计划 (TDD)

本计划旨在完成迭代二的核心功能：跳板机连接、自动提权及密码安全存储。开发将严格遵循 TDD (测试驱动开发) 模式。

## 1. 后端开发：安全存储模块 (SecretStore)
**目标**：实现密码的安全存储，避免明文落盘。
- [ ] **Define**: 定义 `SecretStore` 接口 (`Set`, `Get`, `Delete`)。
- [ ] **Test**: 编写 `pkg/secretstore/store_test.go`，使用 Mock 验证业务逻辑。
- [ ] **Implement**: 使用 `github.com/zalando/go-keyring` 实现接口。

## 2. 后端开发：SSH 跳板机支持 (Bastion Tunneling)
**目标**：支持 `Local -> Bastion -> Target` 的链式连接。
- [ ] **Refactor**: 重构 `ConnectConfig` 结构体，增加 `Bastion *ConnectConfig` 字段（支持嵌套）。
- [ ] **Test**: 编写 `pkg/sshclient/client_test.go` 中的跳板机连接测试用例（使用 Mock SSH Server 或 Docker 容器模拟多跳环境，或重点测试连接链构建逻辑）。
- [ ] **Implement**: 修改 `NewClient` 逻辑，若存在 Bastion 配置，先建立 Bastion Client，再通过 `bastionClient.Dial` 连接目标。

## 3. 后端开发：自动提权 (Auto-Sudo)
**目标**：自动识别 `sudo` 密码提示并注入密码。
- [ ] **Test**: 编写 `pkg/sshclient/autosudo_test.go`。
    -   模拟 `Stdout` 输出流包含 `Password:` 或 `密码：`。
    -   验证 `Stdin` 是否自动写入了预设的 Root 密码。
- [ ] **Implement**: 实现 `SudoHandler`，在 `StartShell` 的数据流处理中通过 `io.TeeReader` 或类似机制监听关键字。

## 4. 前端开发：UI 增强 (ConnectionModal)
**目标**：提供跳板机配置和 Root 密码输入的界面。
- [ ] **Test**: 更新 `frontend/src/components/ConnectionModal/ConnectionModal.test.tsx`。
    -   验证是否存在 "Jump Host" 配置区域（可折叠或标签页）。
    -   验证是否存在 "Root Password" 输入框。
    -   验证表单提交数据结构是否包含嵌套的 Bastion 配置。
- [ ] **Implement**: 修改 `ConnectionModal.tsx`，增加相关表单项和交互逻辑。

## 5. 集成与验收
- [ ] **Integration**: 更新 `app.go`，串联 UI 参数 -> SecretStore 保存 -> SSH Client 连接。
- [ ] **Verify**:
    -   **跳板机**：验证通过跳板机连接测试服务器。
    -   **提权**：连接后执行 `sudo -i` 验证自动输入。
    -   **安全**：检查本地文件，确认无明文密码。
