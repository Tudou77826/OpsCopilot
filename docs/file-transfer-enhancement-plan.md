# 文件传输页面增强计划

> 状态：✅ 全部完成（8+2 项）。涉及文件主要是 `frontend/src/components/Sidebar/FilesPanel.tsx` 与其测试 `FilesPanel.test.tsx`，以及 `app.go` 后端小改动。

## 现状结论（已审查）

- 实际生产入口是 `FlexLayoutAdapter.tsx:220` 以 Tab 形式嵌入的 `FilesPanel`；`FileTransferWindow`（浮动窗口）是未接入的死代码。
- 前端接口定义在 `FilesPanel.tsx:42-59`（`FileTransferBackend`），走 `window.go.main.App` 的 `FT*`/`Local*` 方法。
- 后端传输层 `pkg/filetransfer/*`，任务事件 `file-transfer-progress` / `file-transfer-done`。

---

## 任务清单

### 1. 删除 FileTransferWindow（死代码）✅
- 删除 `frontend/src/components/FileTransferWindow/` 整个目录。
- 全局确认无 import 引用（已确认全仓库只有自身引用）。

### 2. 下载本地已存在 → 确认防静默覆盖 ✅
- 后端新增 `SelectSavePath`（`app.go`，复用 `ExportScript` 的 `runtime.SaveFileDialog` 模式）；前端接口声明 `LocalStat`（后端已有实现）。
- `ConfirmDialog` 扩展 `choices` 多按钮模式（返回 `boolean | string | null`），现有布尔调用不受影响。
- `startDownloadFile` / `startDownloadByPath` 下载前先 `LocalStat(dst)`，本地已存在则三选：覆盖 / 另存为…（系统对话框）/ 取消。

### 3. 前端递归删除警示 ✅
- `deleteRemoteSelected` / `deleteLocalSelected` 改为多选批量删除，文案按是否含目录追加"将递归删除目录及其全部内容，此操作不可恢复"警示，并展示所选条目摘要。

### 4. 多选与批量操作 ✅
- 选中状态由 `string` 改为 `Set<string>`（`localSelected` / `remoteSelected`），行点击单选、Ctrl/Cmd 增减、Shift 范围选择（`handleSelect`）。
- 工具栏新增"上传"/"下载"批量按钮；删除、重命名、编辑均适配多选语义（批量/首项）。
- 双击打开、拖拽上传、右键菜单等沿用取首项逻辑，保持兼容。

### 5. 队列进度/速度格式化 ✅
- 队列抽屉字节改用 `formatFileSize()`，新增 `formatSpeed()`（B/s → KB/s / MB/s）。
- 渲染后端已下发但未展示的 `step` 字段（步骤提示）。

### 6. 右键上下文菜单 ✅
- 新建通用 `FileContextMenu` 组件（fixed 定位、边缘翻转、ESC/外部点击关闭）。
- 本地/远端两侧支持：文件行右键（上传/下载、编辑、新建、重命名、删除、复制路径、刷新）与空白区域右键（新建文件夹、刷新）。
- 右键未选中条目时先单选该条目；多选状态下显示批量操作。

### 7. 浏览体验增强 ✅
- 列头点击排序（名称/大小/时间），支持升降序切换与 ▲/▼ 指示。
- 每侧 pane 顶部新增文件名过滤输入框。
- 面包屑导航（支持 Windows 盘符与 POSIX 路径，分段点击跳转）。
- "显示隐藏"开关（默认隐藏 `.` 开头条目）。
- 目录始终排文件前，过滤/排序不影响数据源。

### 8. 新建/重命名替换 prompt() ✅
- 新建应用内 `NameDialog` 组件（确认/取消按钮 + 空名与非法字符校验 + Enter/Esc 快捷键），`askName()` Promise 辅助替换全部 4 处 `prompt()`（本地/远端各新建、重命名）。

---

## 后续 UI 优化（用户反馈追加）

### 9. 多选入口改为每行复选框 ✅
- 表格/紧凑列表每行首列加复选框，勾选即多选；表头提供全选/取消全选（半选 indeterminate 态）。
- 复选框 `onChange` 切换选中集合，`onClick stopPropagation` 避免触发行单选；行点击仍保持原有单选/Ctrl/Shift 语义。
- 保留了 Ctrl/Cmd 点击、Shift 范围选择等快捷键兼容。

### 10. 顶部功能区单行精简 ✅
- 精简为单行：会话下拉框 + 协议徽标（仅连接方式，工作方式入 hover 提示）+ 显示队列按钮。
- 去掉了低价值的"会话数"辅助信息。

### 11. pane 头部合并 + 工具栏图标化 → 全部移入右键菜单 ✅
- 删除了与面包屑重复的路径 badge（`paneHeader` 行），标题与面包屑合并为一行（`本地 / C: / Users / 15802`）。
- 操作按钮从文字→图标→**最终全部移除**，新建/上传/下载/重命名/删除/编辑/刷新/复制路径统一收进右键菜单（文件行右键 + 空白区右键）。
- pane 头部从 4 行压缩为 2 行（标题+面包屑、路径栏），过滤栏紧随其后。

### 12. 多选复制路径输出列表 ✅
- 多选时"复制路径"菜单项显示 `复制路径 (N)`，点击后把所有选中路径按换行（`\n`）写入剪贴板；单选行为不变。

### 13. 列宽内容自适应 + 滚动条不覆盖表头 ✅
- 初始列宽按内容自适应：每列取所有显示项的文本测量宽度，**去掉最长的前 20%** 后取剩余最大值 + 内边距（名称列额外容纳复选框/图标）；用户手动拖拽过的列不再被自适应覆盖。
- 表格容器加 `scrollbar-gutter: stable`，为垂直滚动条预留空间，sticky 表头不再覆盖滚动条区域。

---

## 实施顺序

建议按依赖顺序：先做 2（安全）、再 4（状态结构改动大，牵动 3/6/7/8 的选中与交互），最后做 1/5/6/7/8 等纯 UI 项。

每项完成后运行：

```bash
cd frontend && npx tsc --noEmit && npx vitest run
```
