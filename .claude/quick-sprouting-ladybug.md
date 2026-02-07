# 外部定位脚本参数传递增强功能

## 需求概述

当前外部定位脚本在执行时无法接收参数，导致脚本无法获取问题描述、代码仓库路径等关键信息。本功能旨在为外部定位脚本添加参数传递机制，使其能够接收动态参数并输出结论到指定目录。

## 用户确认的设计方案

### 参数传递方式
- **仅使用命令行参数**（非环境变量）

### 配置方式
- **目标代码仓库路径**：每次排查时在故障排查面板中输入
- **外挂脚本增强模式**：在故障排查面板中可手动开启，开启后允许配置参数
- **结论输出目录**：使用固定临时目录（`%TEMP%\OpsCopilot\troubleshoot`）
- **自定义变量**：在问题排查面板中允许指定自定义的变量（键值对形式）

## 实现计划

### Phase 1: 后端改造

#### 1.1 修改 `app.go` - 更新函数签名和参数处理

**文件**: `d:\dev\workspace-go\OpsCopilot\app.go`

**修改 `runTroubleshootWithExternal` 函数**（第563行）：

```go
// 修改前
func (a *App) runTroubleshootWithExternal(problem, scriptPath string) string

// 修改后
func (a *App) runTroubleshootWithExternal(problem, scriptPath string, repoPath string, customVars map[string]string) string
```

#### 1.2 实现命令行参数传递逻辑

在 `runTroubleshootWithExternal` 函数中（第594-610行），修改脚本执行部分：

```go
// 创建临时输出目录
tempDir := filepath.Join(os.TempDir(), "OpsCopilot", "troubleshoot")
os.MkdirAll(tempDir, 0755)

// 构建命令行参数
args := []string{
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", scriptPath,
    "-Problem", problem,
    "-OutputDir", tempDir,
}

// 如果提供了代码仓库路径，添加参数
if repoPath != "" {
    args = append(args, "-RepoPath", repoPath)
}

// 添加自定义变量
for key, value := range customVars {
    args = append(args, "-"+key, value)
}

cmd := exec.Command("powershell", args...)
output, err := cmd.CombinedOutput()
```

#### 1.3 更新 `AskTroubleshoot` 函数

**文件**: `d:\dev\workspace-go\OpsCopilot\app.go`

需要修改 `AskTroubleshoot` 函数的签名，增加参数：

```go
// 修改前
func (a *App) AskTroubleshoot(problem string) string

// 修改后
func (a *App) AskTroubleshoot(problem string, repoPath string, customVars map[string]string) string
```

然后在调用 `runTroubleshootWithExternal` 时传递这些参数。

### Phase 2: 前端UI改造

#### 2.1 修改故障排查面板 - 添加增强模式UI

**文件**: `d:\dev\workspace-go\OpsCopilot\frontend\src\components\Sidebar\TroubleshootingPanel.tsx`

**添加状态变量**（第40行附近）：

```tsx
const [externalScriptEnhanced, setExternalScriptEnhanced] = useState(false);
const [repoPath, setRepoPath] = useState('');
const [customVars, setCustomVars] = useState<Record<string, string>>({});
const [showCustomVarEditor, setShowCustomVarEditor] = useState(false);
```

**在输入框上方添加增强模式控制区域**（在第641行 `isInvestigating &&` 之后）：

```tsx
{isInvestigating && (
    <div style={styles.footer}>
        {/* 增强模式开关 */}
        <div style={styles.enhancedModeToggle}>
            <label style={styles.checkboxLabel}>
                <input
                    type="checkbox"
                    checked={externalScriptEnhanced}
                    onChange={(e) => setExternalScriptEnhanced(e.target.checked)}
                    style={styles.checkbox}
                />
                <span style={styles.checkboxText}>外挂脚本增强模式</span>
            </label>
        </div>

        {/* 增强模式配置区域 */}
        {externalScriptEnhanced && (
            <div style={styles.enhancedConfig}>
                <div style={styles.configRow}>
                    <label style={styles.configLabel}>代码仓库路径：</label>
                    <input
                        type="text"
                        value={repoPath}
                        onChange={(e) => setRepoPath(e.target.value)}
                        placeholder="D:\dev\my-project"
                        style={styles.configInput}
                    />
                </div>

                <div style={styles.configRow}>
                    <label style={styles.configLabel}>自定义变量：</label>
                    <button
                        onClick={() => setShowCustomVarEditor(!showCustomVarEditor)}
                        style={styles.editButton}
                    >
                        {showCustomVarEditor ? '收起' : '编辑'}
                    </button>
                </div>

                {showCustomVarEditor && (
                    <div style={styles.customVarEditor}>
                        {Object.entries(customVars).map(([key, value]) => (
                            <div key={key} style={styles.varRow}>
                                <input
                                    type="text"
                                    value={key}
                                    onChange={(e) => {
                                        const newVars = {...customVars};
                                        delete newVars[key];
                                        setCustomVars(newVars);
                                    }}
                                    style={{...styles.varInput, width: '40%'}}
                                    placeholder="变量名"
                                />
                                <input
                                    type="text"
                                    value={value}
                                    onChange={(e) => {
                                        setCustomVars({...customVars, [key]: e.target.value});
                                    }}
                                    style={{...styles.varInput, flex: 1}}
                                    placeholder="变量值"
                                />
                                <button
                                    onClick={() => {
                                        const newVars = {...customVars};
                                        delete newVars[key];
                                        setCustomVars(newVars);
                                    }}
                                    style={styles.deleteButton}
                                >
                                    删除
                                </button>
                            </div>
                        ))}
                        <button
                            onClick={() => {
                                const newKey = `var${Object.keys(customVars).length + 1}`;
                                setCustomVars({...customVars, [newKey]: ''});
                            }}
                            style={styles.addButton}
                        >
                            + 添加变量
                        </button>
                    </div>
                )}
            </div>
        )}

        {/* 原有的按钮和输入框 */}
        {/* ... */}
    </div>
)}
```

#### 2.2 修改 `handleSend` 函数，传递增强参数

在调用 `AskTroubleshoot` 时传递新参数（第140行和第313行）：

```tsx
// 修改前
const response = await window.go.main.App.AskTroubleshoot(problem);

// 修改后
const response = await window.go.main.App.AskTroubleshoot(
    problem,
    externalScriptEnhanced ? repoPath : '',
    externalScriptEnhanced ? customVars : {}
);
```

#### 2.3 添加样式

在 `styles` 对象中添加（第707行附近）：

```tsx
enhancedModeToggle: {
    padding: '8px 12px',
    borderBottom: '1px solid #3a3a3a',
},
checkboxLabel: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    cursor: 'pointer',
},
checkbox: {
    width: '16px',
    height: '16px',
    cursor: 'pointer',
},
checkboxText: {
    color: '#ccc',
    fontSize: '13px',
},
enhancedConfig: {
    padding: '12px',
    backgroundColor: '#1a1a1a',
    borderBottom: '1px solid #3a3a3a',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
},
configRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
},
configLabel: {
    color: '#999',
    fontSize: '12px',
    minWidth: '100px',
},
configInput: {
    flex: 1,
    backgroundColor: '#252526',
    color: '#ddd',
    border: '1px solid #3a3a3a',
    borderRadius: '4px',
    padding: '6px 10px',
    fontSize: '13px',
},
editButton: {
    backgroundColor: '#3a3a3a',
    color: '#ccc',
    border: '1px solid #4a4a4a',
    borderRadius: '4px',
    padding: '4px 12px',
    fontSize: '12px',
    cursor: 'pointer',
},
customVarEditor: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
    paddingLeft: '108px',
},
varRow: {
    display: 'flex',
    gap: '6px',
    alignItems: 'center',
},
varInput: {
    backgroundColor: '#252526',
    color: '#ddd',
    border: '1px solid #3a3a3a',
    borderRadius: '4px',
    padding: '6px 10px',
    fontSize: '13px',
},
deleteButton: {
    backgroundColor: '#5c3a3a',
    color: '#ff8080',
    border: '1px solid #6c4a4a',
    borderRadius: '4px',
    padding: '4px 8px',
    fontSize: '11px',
    cursor: 'pointer',
},
addButton: {
    backgroundColor: '#3a5a3a',
    color: '#80cc80',
    border: '1px solid #4a6a4a',
    borderRadius: '4px',
    padding: '4px 12px',
    fontSize: '12px',
    cursor: 'pointer',
},
```

### Phase 3: 示例脚本更新

#### 3.1 更新测试脚本以演示参数接收

**文件**: `d:\dev\workspace-go\OpsCopilot\scripts\test_troubleshoot.bat`

```batch
@echo off
chcp 65001 >nul

REM 解析命令行参数
:parse_args
if "%~1"=="" goto end_parse
if "%~1"=="-Problem" (
    set PROBLEM=%~2
    shift
    shift
    goto parse_args
)
if "%~1"=="-RepoPath" (
    set REPO_PATH=%~2
    shift
    shift
    goto parse_args
)
if "%~1"=="-OutputDir" (
    set OUTPUT_DIR=%~2
    shift
    shift
    goto parse_args
)
REM 处理自定义变量（以 - 开头）
if "%~1"=="-~1" (
    set VAR_NAME=%~1
    set VAR_NAME=!VAR_NAME:~1!
    set !VAR_NAME!=%~2
    shift
    shift
    goto parse_args
)
shift
goto parse_args
:end_parse

echo # 外部工具定位结论（增强模式）
echo.
echo ## 接收到的参数
echo.
if defined PROBLEM echo - **问题描述**: %PROBLEM%
if defined REPO_PATH echo - **代码仓库**: %REPO_PATH%
if defined OUTPUT_DIR echo - **输出目录**: %OUTPUT_DIR%
echo.
echo ## 测试结果
echo.
echo 这是一个支持参数传递的测试脚本。
echo.
```

### Phase 4: 重新生成 Wails 绑定

在项目根目录执行：

```bash
wails generate module
```

这将更新前端对后端 API 的调用接口。

## 验证计划

### 1. 后端验证
- 编译项目，确保无语法错误
- 测试临时目录创建逻辑

### 2. 前端验证
- 打开故障排查面板
- 开启"外挂脚本增强模式"
- 输入代码仓库路径
- 添加自定义变量
- 发送问题排查请求

### 3. 端到端测试
1. 在设置面板配置外部脚本路径
2. 在故障排查面板开启增强模式
3. 输入代码仓库路径和自定义变量
4. 发送问题描述
5. 检查脚本是否正确接收参数（查看输出）
6. 验证临时输出目录是否创建

### 4. 脚本测试
- 创建一个测试脚本，输出接收到的所有参数
- 验证 `-Problem`、`-RepoPath`、`-OutputDir` 参数
- 验证自定义变量是否正确传递

## 关键文件清单

### 需要修改的文件
1. `d:\dev\workspace-go\OpsCopilot\app.go` - 后端核心逻辑
2. `d:\dev\workspace-go\OpsCopilot\frontend\src\components\Sidebar\TroubleshootingPanel.tsx` - 前端UI
3. `d:\dev\workspace-go\OpsCopilot\scripts\test_troubleshoot.bat` - 示例脚本

### 需要更新的文件（wails generate）
- `d:\dev\workspace-go\OpsCopilot\frontend\wailsjs\go\main\App.js` - 自动生成

## 注意事项

1. **Wails 绑定更新**：修改后端函数签名后必须重新生成绑定
2. **参数传递**：PowerShell 命令行参数需要正确转义，避免特殊字符问题
3. **临时目录**：每次执行前应清理或创建新的临时目录
4. **向后兼容**：增强模式参数应该是可选的，不开启时保持原有行为
5. **错误处理**：脚本执行失败时应显示详细的错误信息
