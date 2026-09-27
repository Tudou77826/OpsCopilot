# E2E 黄金用例

仿真的关键业务动作测试：真实前端构建产物 + 真实 Wails Go 后端 + 本地 fakessh
SSH 端点。定位是「跨层接缝」的回归防护——单测覆盖不到的边界（Wails 绑定、
落盘线格式、SSH 数据面时序、真实浏览器事件调度）在这里收口。

一条命令，全程约 20 秒（含 Go 构建 + 环境拉起 + 6 条用例）：

```bash
cd e2e && npm install && npm test
```

## 被测环境（harness/launch.mjs 自动组装）

| 组件 | 形态 | 说明 |
|------|------|------|
| 应用 | `go build -tags dev` 的 exe | wails v2 dev exe 读环境变量 `devserver`/`assetdir` 自托管 web 界面，无需 wails CLI / vite |
| 前端 | `frontend/dist` 构建产物 | 与生产形态一致；改了前端要先 `npm --prefix frontend run build` |
| SSH 端点 ×2 | fakessh（test/test） | :34316 带 SFTP 沙箱；:34317 无 SFTP（逼出降级路径） |
| 数据 | `e2e/.runtime/`（gitignore） | exe 目录 = 应用配置目录，每次运行清空重建，与开发实例完全隔离 |

环境每次运行重置（"全新装机"状态），harness 作为 Playwright webServer 存活并
监管子进程，测试结束整棵进程树被回收。浏览器用系统 Chrome（channel），
无需下载 Playwright Chromium；无 Chrome 环境设 `E2E_BROWSER_CHANNEL=chromium`。

## 设计原则

1. **真实应用**：不 mock Wails 绑定、不 mock 落盘、不 mock SSH——全部走真实链路。
2. **落盘/磁盘断言**：sessions.json 直接读应用目录的下划线线格式；文件传输
   直接断言 fakessh 沙箱和本地夹具目录的字节内容。前端方言问题在此不存在。
3. **时限即断言**：每个动作都有硬时限，超时即红——「点了没反应」类挂起
   回归（如 v1.10.4 上传假死）的统一 tripwire。
4. **白屏即断言**：每条用例收尾检查 `#root` 仍有内容，React 整树卸载直接红。

## 用例清单（15 条，持续扩充）

| 用例 | 守护的回归类 |
|------|-------------|
| G-CONN-01 连接生命周期 | #71 root 密码静默清空；v1.10.2 树出参丢 rootPassword → 编辑弹窗空显、改名即清盘 |
| G-CONN-04a/b 连接失败路径 | 密码错误/端口不可达必须 12s 内弹「连接失败」，不许无反应；错误路径是历史回归高发区 |
| G-CONN-05 树管理动作 | 复制连接必须完整携带 root_password；删除只删目标不殃及同端点其他连接（全磁盘断言） |
| G-TERM-09 终端 smoke | 键盘→xterm→绑定→sshclient→PTY→回显的完整数据面；会话串号/PTY 假死/DOM 崩溃 |
| G-TERM-10 多会话并存 | 双终端各自收发互不串号（数据路由错 session 类缺陷） |
| G-TERM-11 关闭重连 | 关标签后句柄/监听泄漏导致重连错乱 |
| G-TERM-12 日常信息流 | cd/grep 命令往返；服务端刷屏（150 行日志推送）渲染不卡死；Ctrl+F 搜索跳转命中滚动历史行 |
| G-FILE-14 SFTP 上传/下载 | v1.10.4 上传假死；SFTP 数据面任何一层断裂 |
| G-FILE-16 远端增删改名 | FTList/Mkdir/Rename/Remove 命令面；沙箱路径解析 |
| G-FILE-17 同名冲突覆盖 | 冲突检测（FTStat）与传输分链路：确认框必须弹、覆盖必须逐字节生效 |
| G-FILE-18 无 SFTP 降级 | v1.10.3 退化为历史表单；v1.10.4 su 空密码假死（要求限时给出 SFTP_NOT_SUPPORTED 结论） |
| G-SET-27 侧栏越界拖拽 | v1.10.5 pointerup/updater 竞态整树卸载白屏（jsdom 无法复现，只有真实浏览器能守住） |
| G-SET-28 视口连续变化 | resize 链路（refit 定时器/重排/fit）崩溃与白屏 |
| G-SET-29 字号切换 fit 完整性 | fit 记账与留白错位：行区越界/最后一行贴底栏（dpr 1 与 1.5 双场景，含"与底缘 ≥6px 留白"断言） |

**过程中挖出并修复的产品缺陷**（E2E 偶发失败 → 根因 → 修复 + 红线单测）：

1. 新建/编辑连接弹窗的表单会被后台刷新整体清空——`ConnectionPropertiesModal`
   把 `initialConfig` 放进重置 effect 的依赖，而父组件每次渲染（5 秒轮询、
   共享会话加载失败重试）都传入新引用。已在
   `frontend/src/components/Sidebar/ConnectionPropertiesModal.business.test.tsx`
   加确定性红线用例。
2. 未选中的文件行直接右键上传/下载会走「请先选择」分支（右键的同步选中进
   不了菜单回调闭包）——真实用户跳过左键也会踩到，待定是否修产品侧。
3. 字号切换后最后一行贴底/被裁（G-SET-29 守护）：FitAddon 按"父元素
   computed height − xterm 自身 padding"记账，而视觉留白历史上加在 fit 父元素
   `.terminal-host` 上（border-box 下 computed height 含 padding），fit 账面
   虚增、特定 视口/字号/DPI 相位下多排一行。修复：留白挪到 Terminal 根包裹层。
4. 传输完成事件的自动刷新会把目录拽回旧路径（G-FILE-17 偶发的真相）：
   `refreshLocalAuto` 跟随"最后提交路径"，落在用户切目录窗口内时覆盖用户
   导航，后续下载落错目录。修复：自动刷新跟随"最新导航意图"+ 序号守卫。

## 维护约定

- **每次运行都重建 Go 侧**：launch.mjs 默认执行 go build（热缓存秒级），
  保证防护网测的是当前代码。跳过构建：`E2E_SKIP_BUILD=1 npm test`。
- **选择器优先用 testid/role/label**：组件里已有 `session-tree`、`file-pane-远端`、
  `name-dialog-input`、`tree-row-*` 等稳定锚点；改 UI 时保住这些锚点。
- **有状态长流程，串行执行**（workers=1），用例间靠"唯一名称 + 每次运行重置"
  保证互不干扰。
- **新增回归先补用例再修**：每次生产回归修完，把复现路径固化成一条 golden
  spec（时限、落盘、白屏三件套是默认断言面）。

## 路线图

- E2 真实开发服务器端点（长流程跨域链路：跳板机、root 提权、断线重连）。
- 失败自动截图 + trace 已开启（`test-results/`），配合 `npm run report` 复盘。
