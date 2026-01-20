# 终端高亮和搜索功能设计文档

## 文档信息

* **项目名称**: OpsCopilot

* **功能模块**: 终端高亮和搜索

* **版本**: 1.0

* **创建日期**: 2025-01-19

* **作者**: Claude AI

***

## 0. 现状对齐与已决策

### 0.1 仓库现状对齐（必须与实现一致）

* 前端技术栈：React 18 + TypeScript + Vite；终端组件基于 xterm.js 5.3.0。

* 现有依赖：`xterm`、`xterm-addon-fit` 已存在；`xterm-addon-search` 当前未在依赖中，需要显式引入。

* 现有终端交互：终端组件已使用 `attachCustomKeyEventHandler` 接管部分按键（如 Tab、方向键、Ctrl+C/Ctrl+V）用于补全与复制粘贴。搜索面板必须定义与这些按键的优先级与冲突策略。

* 输出数据流：终端输出通常经由上层调用 `terminalRef.write()` 写入 xterm。设计中的“监听数据写入事件并增量高亮”需要落到可实现的触发点（见 4.2）。

### 0.2 已决策（本设计稿内置默认值）

* 搜索范围：覆盖终端缓冲区（含滚动历史），默认 scrollback 目标为 5000 行（可配置）。

* 快捷键：Ctrl+F 打开；Esc 关闭；Enter/Shift+Enter 导航；F3/Shift+F3 导航（与常见习惯对齐）。

* 叠加规则：搜索高亮优先级高于规则高亮（搜索匹配处优先显示搜索高亮）。

* Regex 风险策略：本地强校验为硬约束；高风险 pattern 允许保存但默认禁用并强提示风险（避免阻断用户工作流）。

## 1. 功能概述

### 1.1 背景

OpsCopilot 是一个基于 Wails v2 的桌面应用，为运维人员提供 SSH 终端管理和 AI 辅助运维能力。当前终端组件缺乏内容搜索和关键信息突出显示功能，用户在处理大量日志输出时需要手动查找关键信息，效率较低。

### 1.2 目标

为 OpsCopilot 的终端组件添加两大核心功能：

1. **Ctrl+F 搜索功能**

   * 支持在整个终端缓冲区（包括滚动历史）中搜索关键字

   * 提供上一个/下一个匹配项导航

   * 显示当前匹配位置和总匹配数（如"3/15"）

   * 支持区分大小写和正则表达式模式

2. **突出显示规则**

   * 通过配置面板设置正则表达式规则

   * 自动高亮显示匹配规则的终端输出

   * 支持自定义高亮样式（背景色、文字颜色、粗体、下划线）

   * 提供预设规则模板库（错误关键词、日志级别、IP 地址等）

   * 使用 AI 校验正则表达式安全性

   * 支持调整规则优先级（上下箭头调序）

### 1.3 用户价值

* **提高运维效率**: 快速定位错误日志、关键信息

* **降低认知负担**: 自动高亮重要内容，减少人工查找

* **增强可定制性**: 用户可根据需求自定义高亮规则

* **保证安全性**: AI 校验防止恶意正则导致性能问题

***

## 2. 技术架构

### 2.1 技术栈

#### 前端

* **框架**: React 18.2.0 + TypeScript

* **终端库**: xterm.js 5.3.0

  * xterm-addon-fit 0.8.0（尺寸自适应，已存在）

  * xterm-addon-search 0.13.0（搜索功能，需要新增依赖）

* **构建工具**: Vite 5.4.21

#### 后端

* **语言**: Go

* **框架**: Wails v2

* **AI 服务**: OpenAI-compatible API（通过 go-openai）

### 2.2 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                         Frontend (React)                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ SearchPanel  │  │  Terminal    │  │HighlightRules│      │
│  │   (Ctrl+F)   │  │  Component   │  │   Modal      │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│         │                  │                  │             │
│         └──────────────────┴──────────────────┘             │
│                            │                                │
│                     ┌──────▼──────┐                         │
│                     │  App.tsx    │                         │
│                     │ (State Mgmt)│                         │
│                     └──────┬──────┘                         │
└────────────────────────────┼────────────────────────────────┘
                             │
                    ┌────────▼────────┐
                    │   Wails Bridge  │
                    └────────┬────────┘
                             │
┌────────────────────────────┼────────────────────────────────┐
│                    Backend (Go)                            │
│         ┌────────────────────▼────────────────────┐        │
│         │              app.go                       │        │
│         │  - GetHighlightRules()                   │        │
│         │  - SaveHighlightRules()                  │        │
│         │  - ValidateRegexWithAI()                 │        │
│         └────────────────────┬────────────────────┘        │
│                              │                             │
│         ┌────────────────────▼────────────────────┐        │
│         │         pkg/config/                      │        │
│         │  - HighlightRule struct                  │        │
│         │  - loadHighlightRules()                  │        │
│         │  - saveHighlightRules()                  │        │
│         └────────────────────┬────────────────────┘        │
│                              │                             │
│         ┌────────────────────▼────────────────────┐        │
│         │         pkg/ai/                          │        │
│         │  - ValidateRegexWithAI()                │        │
│         │  - FastModel/ComplexModel               │        │
│         └─────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────┘
```

### 2.3 核心技术选型

#### xterm.js 集成策略

**搜索功能**:

* 使用 `xterm-addon-search` 实现 Ctrl+F 搜索

* 提供原生的查找和导航能力

* 已针对性能优化，适合大缓冲区

**高亮功能**:

* 使用 `term.registerDecoration()` API 实现自定义高亮

* 两层高亮系统：

  * **规则高亮**（常驻）：根据配置规则持续高亮

  * **搜索高亮**（临时）：用户搜索时的临时高亮

#### 性能优化策略

1. **触发可控**: 以“写入后节流扫描”作为默认实现路径（不依赖 xterm 内部不可控回调）
2. **防抖处理**: 100\~200ms 节流/防抖，避免每次输出都扫描
3. **视口优化**: 仅扫描可见区域 ±100 行（可配置）
4. **预算与上限**: 单次扫描时间预算 + 装饰器上限（默认 1000），超预算立即降级并提示
5. **异步/分片**: 需要时用 `requestIdleCallback` 分片处理（兼容性降级为 `setTimeout(0)`）

***

## 3. 数据结构设计

### 3.1 TypeScript 类型定义

```typescript
// frontend/src/types.ts

/**
 * 高亮规则
 */
export interface HighlightRule {
    id: string;                    // 唯一标识（UUID）
    name: string;                  // 规则名称（如"错误关键词"）
    pattern: string;               // 正则表达式字符串
    isEnabled: boolean;            // 是否启用
    priority: number;              // 优先级（0-100，数值越小优先级越高）
    style: HighlightStyle;         // 视觉样式
}

/**
 * 高亮样式
 */
export interface HighlightStyle {
    backgroundColor?: string;      // 背景色（#RRGGBB 格式）
    color?: string;                // 文字颜色
    fontWeight?: 'normal' | 'bold'; // 粗体
    textDecoration?: 'underline' | 'none'; // 下划线
    opacity?: number;              // 背景透明度（0.0-1.0）
}

/**
 * 高亮预设模板
 */
export interface HighlightPreset {
    id: string;
    name: string;
    description: string;
    rules: Omit<HighlightRule, 'id'>[];  // 预设规则导入时生成新 ID
}

/**
 * 搜索状态
 */
export interface SearchState {
    query: string;                 // 搜索关键字
    matchIndex: number;            // 当前匹配索引（0-based）
    totalMatches: number;          // 总匹配数
    caseSensitive: boolean;        // 是否区分大小写
    regexMode: boolean;            // 是否使用正则模式
    decorations: IDecoration[];    // 搜索高亮装饰器
}
```

### 3.2 Go 后端结构

```go
// pkg/config/store.go

// HighlightRule 高亮规则配置
type HighlightRule struct {
    ID       string         `json:"id"`        // 唯一标识
    Name     string         `json:"name"`      // 规则名称
    Pattern  string         `json:"pattern"`   // 正则表达式
    IsEnabled bool          `json:"is_enabled"` // 是否启用
    Priority int            `json:"priority"`  // 优先级
    Style    HighlightStyle `json:"style"`     // 样式配置
}

// HighlightStyle 高亮样式配置
type HighlightStyle struct {
    BackgroundColor string  `json:"background_color"` // 背景色
    Color           string  `json:"color"`            // 文字颜色
    FontWeight      string  `json:"font_weight"`      // 字体粗细
    TextDecoration  string  `json:"text_decoration"`  // 文本装饰
    Opacity         float64 `json:"opacity"`          // 透明度
}

// AppConfig 应用配置（扩展）
type AppConfig struct {
    LLM            LLMConfig          `json:"llm"`
    Prompts        map[string]string  `json:"prompts"`
    // ... 其他现有字段 ...
    HighlightRules []HighlightRule    `json:"highlight_rules"` // 新增：高亮规则
}
```

### 3.3 配置存储

**文件结构**:

```
highlight_rules.json  # 高亮规则配置（独立文件）
config.json           # 主配置文件（包含引用）
```

**存储策略**:

* 遵循现有模式（prompts.json, quick\_commands.json）

* 独立存储便于管理和备份

* 通过 `loadHighlightRules()` 和 `saveHighlightRules()` 方法管理

* 自动创建默认配置（如果文件不存在）

***

## 4. 功能设计

### 4.1 Ctrl+F 搜索功能

#### 4.1.1 用户交互流程

```
用户操作                     系统响应
────────────────────────────────────────────
1. 按 Ctrl+F           →  显示搜索面板
2. 输入搜索关键字       →  实时搜索并高亮匹配项
                        →  显示匹配计数（如"3/15"）
3. 按 Enter/F3/点击下一个  →  跳转到下一个匹配
4. 按 Shift+Enter/Shift+F3 →  跳转到上一个匹配
5. 切换选项           →  重新搜索
   - 区分大小写
   - 正则模式
6. 按 Esc/点击×       →  关闭搜索面板，清除高亮
```

#### 4.1.1.1 键盘优先级（与终端输入的冲突处理）

* 搜索面板关闭时：除 Ctrl+F/F3 外，按键默认透传终端（不影响现有补全/复制粘贴行为）。

* 搜索面板打开时：\n  - Enter/Shift+Enter/F3/Shift+F3 仅用于搜索导航，不发送到远端。\n  - Esc 关闭面板并清理搜索高亮。\n  - Ctrl+C/Ctrl+V 维持原有语义（若终端有选区则复制，否则透传中断）。\n  - Tab/方向键优先用于现有补全逻辑（除非未来显式给搜索面板提供键盘导航）。\n

#### 4.1.2 UI 设计

```
┌──────────────────────────────────────────────────┐
│  Terminal Window                                 │
│  ┌──────────────────────────────────────────────┐│
│  │ $ echo "test error test warn test"          ││
│  │ test error test warn test                    ││
│  │                                             ││
│  │ ┌─────────────────────────────────────────┐ ││
│  │ │ 🔍 [error___________] 2/3  [◀] [▶] [×] │ ││  ← 搜索面板
│  │ │    [☑ Aa] [☐ .*]                       │ ││
│  │ └─────────────────────────────────────────┘ ││
│  │                                             ││
│  └──────────────────────────────────────────────┘│
└──────────────────────────────────────────────────┘
```

**组件说明**:

* **搜索框**: 输入搜索关键字

* **匹配计数**: 显示当前/总数

* **导航按钮**: 上一个/下一个

* **关闭按钮**: 关闭搜索面板

* **选项切换**:

  * Aa: 区分大小写

  * .\*: 正则模式

#### 4.1.3 技术实现

**依赖安装**:

```bash
npm install xterm-addon-search@0.13.0
```

**核心代码**:

```typescript
import { SearchAddon } from 'xterm-addon-search';

// 加载 SearchAddon
const searchAddon = new SearchAddon();
term.loadAddon(searchAddon);

// 搜索
const found = searchAddon.findNext(query, {
    caseSensitive: caseSensitive,
    regex: regexMode
});

// 导航
searchAddon.findNext(query, options);      // 下一个
searchAddon.findPrevious(query, options);  // 上一个
```

### 4.2 突出显示规则

#### 4.2.1 高亮引擎

**工作原理**:

1. 规则集合变更或终端输出写入后，触发一次“高亮调度”（节流）
2. 在时间预算内扫描“可见区域 ±N 行”（默认 N=100），对每行应用已启用规则
3. 将匹配结果转为 decoration 计划并应用到 xterm（并清理旧 decoration）
4. 根据规则优先级与层级规则（搜索高亮 > 规则高亮）解决冲突

**优先级规则**:

* Priority 数值越小，优先级越高

* 高优先级规则覆盖低优先级

* 同一位置只显示最高优先级的高亮

**性能优化**:

* 扫描窗口：仅扫描可见区域 ±N 行（默认 N=100，可配置）

* 节流策略：100\~200ms（避免每个 write 都触发扫描）

* 本地硬限制（必须写入实现）：\n  - 单条规则 pattern 最大长度（例如 200）\n  - 单行最大匹配次数（例如 20）\n  - 单次扫描最大匹配总数（例如 500）\n  - 装饰器上限（默认 1000）\n  - 单次扫描时间预算（例如 8\~12ms；超预算直接停止并进入降级模式）

* 降级模式：当超预算/装饰器超限时，停止本轮高亮并提示“已降级”；用户可一键关闭高亮（紧急止损）。

#### 4.2.1.1 Regex 安全策略（AI 不是唯一防线）

* 保存前本地静态检查（示例）：\n  - 嵌套量词（如 `(a+)+`）\n  - 宽泛回溯热点（如 `(.+)+`、`(.*)+`）\n  - 过长模式或大量分支\n- 运行时保护：限制每行匹配次数/总匹配数；对可见行分片处理；避免在一次渲染帧内做过多 work。\n- AI 校验（可选）：仅提供风险提示与改进建议，不作为唯一判定依据。\n

提供以下预设模板：

| 规则名称  | 正则模式                                     | 样式       | <br />          | <br />    | <br />     | <br /> | <br />     | <br /> |
| ----- | ---------------------------------------- | -------- | :-------------- | :-------- | :--------- | :----- | :--------- | :----- |
| 错误关键词 | \`(?i)\b(error                           | fail     | fatal           | exception | panic)\b\` | 红底白字粗体 | <br />     | <br /> |
| 警告关键词 | \`(?i)\b(warn                            | warning  | deprecated)\b\` | 黄底黑字      | <br />     | <br /> | <br />     | <br /> |
| 日志级别  | \`\[?(TRACE                              | DEBUG    | INFO            | NOTICE    | WARN       | ERROR  | FATAL)]?\` | 蓝底白字粗体 |
| IP 地址 | `\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b` | 绿底黑字下划线  | <br />          | <br />    | <br />     | <br /> | <br />     | <br /> |
| URL   | `https?://[^\s]+`                        | 青底黑字下划线  | <br />          | <br />    | <br />     | <br /> | <br />     | <br /> |
| 成功关键词 | \`(?i)\b(success                         | complete | done)\b\`       | 绿底白字粗体    | <br />     | <br /> | <br />     | <br /> |
| 时间戳   | `\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}` | 紫底白字     | <br />          | <br />    | <br />     | <br /> | <br />     | <br /> |
| 文件路径  | `[/\w-]+\.\w+`                           | 粉底黑字下划线  | <br />          | <br />    | <br />     | <br /> | <br />     | <br /> |

#### 4.2.3 规则配置 UI

**主界面**:

```
┌────────────────────────────────────────────────────────┐
│  高亮规则配置                              [X]         │
├────────────────────────────────────────────────────────┤
│  [我的规则] [预设模板]                                 │
│                                                        │
│  [+ 添加规则]                     [从模板导入 ▼]       │
│                                                        │
│  ┌──────────────────────────────────────────────────┐ │
│  │ ✓ 错误关键词    \b(error|fail)\b      [红底白字粗体] │ │
│  │   [编辑] [删除] [↑] [↓]                          │ │
│  ├──────────────────────────────────────────────────┤ │
│  │ ⚠ 警告关键词    \b(warn|warning)\b     [黄底黑字]   │ │
│  │   [编辑] [删除] [↑] [↓]                          │ │
│  └──────────────────────────────────────────────────┘ │
│                                                        │
│                   [取消] [保存] [应用]                 │
└────────────────────────────────────────────────────────┘
```

**规则编辑器**:

```
┌────────────────────────────────────────────────────────┐
│  编辑高亮规则                                 [X]       │
├────────────────────────────────────────────────────────┤
│  规则名称: [自定义错误___________________]              │
│  匹配模式: [(?i)CRITICAL__________________]            │
│            (正则表达式)                               │
│                                                        │
│  样式配置：                                            │
│  ┌──────────────────────────────────────────────────┐ │
│  │ 背景色:   [颜色选择器] □                         │ │
│  │ 文字颜色: [颜色选择器] □                         │ │
│  │ 粗体:     ☑                                      │ │
│  │ 下划线:  ☐                                      │ │
│  │ 透明度:   ████○○○○○○ 40%                         │ │
│  └──────────────────────────────────────────────────┘ │
│                                                        │
│  测试文本：                                            │
│  ┌──────────────────────────────────────────────────┐ │
│  │ [输入测试文本...]                                 │ │
│  └──────────────────────────────────────────────────┘ │
│  实时预览：                                            │
│  ┌──────────────────────────────────────────────────┐ │
│  │ 2025-01-19 CRITICAL: System failure              │ │
│  │            ↑ 红底白字粗体高亮                     │ │
│  └──────────────────────────────────────────────────┘ │
│                                                        │
│  [使用 AI 验证]  显示验证结果                          │
│                                                        │
│                   [取消] [保存]                        │
└────────────────────────────────────────────────────────┘
```

### 4.3 AI 正则校验

#### 4.3.1 校验流程

```
用户输入正则
     ↓
点击"使用 AI 验证"
     ↓
调用 ValidateRegexWithAI(pattern)
     ↓
后端构建提示词
     ↓
调用 LLM (FastModel)
     ↓
解析 JSON 响应
     ↓
显示验证结果
     ↓
[可选] 应用 AI 建议的改进
```

#### 4.3.1.1 无 LLM 配置时的行为

* 若未配置 LLM（缺少 APIKey/BaseURL 等），UI 中“使用 AI 验证”按钮置灰，并提示“未配置 AI 服务，无法使用 AI 校验”。

* 无论 AI 是否可用，规则的保存与启用/禁用必须依赖本地强校验与运行时保护策略。

#### 4.3.2 提示词模板

```
你是正则表达式专家。验证以下正则模式用于终端输出高亮：

模式：{pattern}

请以 JSON 格式回复：
{
  "is_valid": true/false,
  "issues": ["潜在问题列表"],
  "suggestions": ["改进建议"],
  "fixed_pattern": "改进后的版本"
}

考虑以下因素：
1. 灾难性回溯风险（如嵌套量词）
2. 大缓冲区性能（>1000 行文本）
3. 意外的匹配模式
4. 浏览器正则兼容性
```

#### 4.3.3 响应示例

**有效的正则**:

```json
{
  "is_valid": true,
  "issues": [],
  "suggestions": ["模式看起来很安全，性能良好"],
  "fixed_pattern": null
}
```

**有问题的正则**:

```json
{
  "is_valid": false,
  "issues": [
    "嵌套量词可能导致灾难性回溯",
    "在长文本上可能性能很差"
  ],
  "suggestions": [
    "使用非贪婪量词",
    "添加边界限制"
  ],
  "fixed_pattern": "(?i)\\bCRITICAL\\b"
}
```

***

## 5. 实施计划

### 5.1 Iteration 0：设计定稿（本轮只改文档）

**目标**：把“方案描述”升级为“可实现 + 可测试 + 可降级”的工程规格。

**交付物**：

* 现状对齐与已决策（0 章节）

* 键盘优先级矩阵与冲突规则（4.1.1.1）

* 高亮引擎数据流与预算/上限（4.2.1）

* Regex 本地强校验策略（4.2.1.1）

* 可验证迭代计划与验收标准（5/6 章节）

### 5.2 Iteration 1：搜索 MVP（可测）

**范围**：

* Ctrl+F 打开搜索面板；Esc 关闭并清理高亮

* Enter/Shift+Enter/F3/Shift+F3 导航

* 区分大小写/正则模式开关

**可验证点（测试建议）**：

* SearchPanel 状态机测试：打开/关闭/导航/计数

* 键盘冲突测试：搜索打开时 Enter 不透传终端；关闭时不影响现有 Ctrl+C/Ctrl+V/Tab 行为

### 5.3 Iteration 2：规则高亮 MVP（可测）

**范围**：

* 规则 CRUD、启用/禁用、优先级排序（上下箭头；不引入拖拽库）

* 可见区 ±N 行节流扫描；装饰器回收；降级提示

**可验证点（测试建议）**：

* 高亮引擎纯函数/最小集成测试：给定可见文本与规则，输出 decoration 计划稳定、上限/降级生效

* 装饰器泄漏测试：连续多次刷新/滚动后 decoration 数量不无限增长

### 5.4 Iteration 3：配置持久化 + 预设模板（可测）

**范围**：

* `highlight_rules.json` 读写（对齐现有 prompts/quick\_commands 的拆分落盘模式）

* 预设模板导入；全局开关（紧急止损）

**可验证点（测试建议）**：

* 配置读写单测/集成测试：保存→重启→仍生效

* UI 关键路径测试：导入模板→启用→生效→关闭全局开关立即停止高亮

### 5.5 Iteration 4：AI 校验（可选增强）

**范围**：

* ValidateRegexWithAI 作为可选按钮；仅做风险提示/建议，不作为唯一防线

**可验证点（测试建议）**：

* LLM 不可用/超时/返回格式异常时的降级路径

### 5.6 关键文件清单（按迭代逐步涉及）

* 前端：`frontend/src/components/Terminal/Terminal.tsx`、SearchPanel、新增高亮引擎模块、Settings 入口、类型定义、`frontend/package.json`

* 后端（若做持久化/AI 校验）：`app.go` + `pkg/config`（新增 highlight\_rules.json 的 load/save）+ `pkg/ai`（可选）

***

## 6. 验收标准

### 6.1 功能性

* ✅ Ctrl+F 搜索整个缓冲区（包括滚动历史）

* ✅ 上一个/下一个导航流畅工作

* ✅ 匹配计数器准确（如"3/15"）

* ✅ 5+ 预设规则包含并正常工作

* ✅ 用户可使用正则创建自定义规则

* ✅ 规则优先级系统正确解决冲突

* ✅ AI 正则验证提供有用的反馈

* ✅ 规则排序工作正常（上下箭头调序）

### 6.2 性能

* ✅ 新数据在 100ms 内高亮

* ✅ 搜索查询在 200ms 内执行

* ✅ 无内存泄漏（装饰器正确释放）

* ✅ 终端滚动保持流畅（60fps）

### 6.3 可用性

* ✅ UI 遵循现有 OpsCopilot 模式

* ✅ 键盘快捷键直观（Ctrl+F, F3, Esc）

* ✅ 错误消息清晰可操作

* ✅ 配置更改立即生效

* ✅ 可以全局禁用高亮

### 6.4 代码质量

* ✅ 所有现有测试仍通过

* ✅ 新增测试覆盖核心路径（搜索面板状态机、高亮引擎上限/降级、配置持久化）

* ✅ 无 TypeScript 错误

* ✅ 不引入新的控制台错误（允许既有非阻断 warning 存在）

* ✅ 代码遵循现有模式

***

## 7. 风险和缓解措施

| 风险     | 影响       | 概率 | 缓解措施                                    |
| ------ | -------- | -- | --------------------------------------- |
| 性能下降   | 终端变卡顿    | 中  | • 默认仅对新数据高亮• 可配置性能限制• 紧急禁用开关            |
| 正则灾难   | 浏览器冻结    | 中  | • 本地强校验（禁用高风险模式）• 运行时预算/上限/降级• 默认仅扫描可见区 |
| 装饰器限制  | 装饰器过多崩溃  | 中  | • 硬上限 1000 个• 清理屏幕外装饰器• 接近上限时警告         |
| LLM 成本 | API 使用过多 | 低  | • 缓存验证结果• 速率限制（5次/分钟）• "跳过 AI 验证"选项     |

***

## 8. 参考资料

### 8.1 xterm.js 文档

* **官方文档**: <https://xtermjs.org/>

* **Decoration API**: <https://xtermjs.org/docs/api/decoration/>

* **SearchAddon**: <https://github.com/xtermjs/xterm.js/tree/master/addons/addon-search>

### 8.2 相关代码

* **Terminal 组件**: `frontend/src/components/Terminal/Terminal.tsx`

* **配置管理**: `pkg/config/store.go`

* **AI 服务**: `pkg/ai/intent.go`

### 8.3 示例代码

**Decoration API**:

```typescript
const marker = term.registerDecoration({
    marker: term.buffer.active.addMarker(cursorY),
    anchor: 'left',
    backgroundColor: '#ff0000',
    color: '#ffffff',
    layer: 'top'
});
marker.dispose();
```

**SearchAddon**:

```typescript
import { SearchAddon } from 'xterm-addon-search';
const searchAddon = new SearchAddon();
term.loadAddon(searchAddon);
searchAddon.findNext('query', { regex: false, caseSensitive: false });
```

***

## 附录：预设规则调色板

```
错误关键词:   bg=#5a1d1d, text=#ff6b6b, bold
警告关键词:   bg=#5a4a1d, text=#feca57, normal
日志级别:     bg=#1d3a5a, text=#54a0ff, bold
IP 地址:      bg=#1d5a2e, text=#5f9ea0, underline
URL:          bg=#1d5a4a, text=#1dd1a1, underline
成功关键词:   bg=#1d5a25, text=#2ecc71, bold
时间戳:       bg=#1d1d5a, text=#a29bfe, normal
文件路径:     bg=#5a1d5a, text=#fd79a8, underline
```

