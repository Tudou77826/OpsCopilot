## 问题原因（现状）
- 目前“FTP 窗口”是一个全屏遮罩层的模态弹窗（overlay 覆盖整个应用，z-index 很高）。
- 这会把鼠标/键盘交互都拦截在弹窗层上，终端自然无法点击、无法输入。
- 当前项目是 Wails v2 单窗口模型：仓库内没有可用的“新建原生窗口/多窗口”实现；Wails v3 才有原生多窗口能力（见 v3 文档与 v2 的限制说明）。【WebSearch:2†https://v3alpha.wails.io/features/windows/multiple/】【WebSearch:4†https://wails.io/blog/the-road-to-wails-v3/】

## 解决思路（给你两条路，优先做不阻塞终端的）
### 方案 A（推荐，改动小）：把 FTP 弹窗改成“非模态/可并行操作”的浮动工具窗
- 目标：FTP 窗口打开时，依然可以点击/输入终端。
- 做法：
  1) overlay 背景改为 `pointer-events: none`，只让 FTP 面板本体 `pointer-events: auto`。
  2) 去掉全局 `window.addEventListener('keydown')` 的 Esc 监听（避免终端按键被抢走）；只在 FTP 面板获得焦点时响应 Esc，或只提供按钮关闭。
  3) FTP 面板支持拖拽移动（标题栏拖动）+ 可缩放（resize），方便把它挪到一边不遮住终端。
  4) 增加“最小化/停靠”按钮：一键收起为右下角小条，避免占屏。

### 方案 B（体验更像“工作区切换”）：把 FTP 做成主工作区页面，而不是遮罩弹窗
- 目标：不会遮罩终端；用户在“终端工作区”和“文件传输工作区”之间切换。
- 做法：在主布局增加一个“工作区路由/状态”，FTP 作为独立页面渲染；终端始终不被遮罩。
- 适合：你们希望界面更干净，但允许在两者间切换（而非并行）。

### 方案 C（最彻底）：做真正的“独立系统窗口”
- 目标：FTP 真的是一个 OS 级独立窗口，终端窗口不受影响。
- 现实约束：Wails v2 原生不支持多窗口，通常需要迁移到 Wails v3 才能做（或引入额外窗口/进程方案，复杂度更高）。【WebSearch:2†https://v3alpha.wails.io/features/windows/multiple/】

## 我将要执行的具体改动（默认按方案 A）
1) 修改 [FileTransferWindow.tsx]：
- overlay 改为非模态（pointer-events 策略）
- 移除全局 Esc 监听，改为按钮关闭 +（可选）仅在面板聚焦时处理 Esc
- 增加拖拽移动与右下角 resize
- 增加最小化/还原
2) 快速回归：打开 FTP 窗口后确认
- 终端可正常点击与输入
- FTP 面板内输入不丢焦点
- 最小化/拖拽/缩放可用
3) 跑 `npm -C frontend run build`（以及必要时 `go test ./...`）确保不破坏构建。

确认后我会按方案 A 直接落地；如果你更想要方案 B 或 C，我也能按你选的方向调整实现。