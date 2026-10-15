# 伙伴养成与功能奖章完整本地实测

在 Windows 上运行真实 Wails 桌面后端、构建后的前端、本地排名服务和 SSH 测试端点。主页面默认使用 `http://127.0.0.1:34115/`。每次启动使用新的 `e2e/.runtime/social-<时间戳>/`，保留业务文件、奖章账本与数据库，不修改常用桌面实例的数据。

## 启动和测试

需要 Go、Node.js、已安装的前端和 E2E npm 依赖，以及系统 Microsoft Edge。首次准备依赖时在仓库根目录执行：

```powershell
npm --prefix frontend-shell ci
npm --prefix frontend ci
npm --prefix e2e ci
```

执行完整实测，成功后保留环境供查看：

```powershell
npm --prefix e2e run social:test
```

仅启动一个空白测试实例：

```powershell
npm --prefix e2e run social:start
```

执行完整实测并在结束后停止所有本次启动的进程：

```powershell
npm --prefix e2e run social:test:stop
```

已有34115实例时，可在其他端口运行隔离的完整回归；服务、SSH和辅助桌面使用随后三个端口：

```powershell
$env:SOCIAL_TEST_PORT='34135'
npm --prefix e2e run social:test:stop
```

开蛋实测会只读检查34115现有伙伴，并在独立34125实例中验证免费开蛋、付费确认与取消、50点扣费、排除当前形象、余额不足、旧版本拒绝和重启恢复。付费用例仅对停止后的隔离测试存档预置73点经验，不改写34115的数据：

```powershell
node e2e/harness/companion-hatch-review.mjs docs/assets/companion-hatch-v1
```

脚本每次重新构建前端和后端。端口占用时会退出，不会终止其他实例。运行期间按 Ctrl+C 停止该脚本拥有的进程；数据目录保留。`E2E_BROWSER_CHANNEL` 可覆盖测试浏览器，默认 `msedge`。

恢复最近一次已通过验收的实例，沿用伙伴、奖章和排名数据：

```powershell
./e2e/harness/social-resume.ps1
```

恢复脚本使用已有构建，独立启动主桌面、排名服务和 SSH 测试端点，不启动辅助桌面；辅助实例退出不会连带停止主页面。可通过 `-RunDirectory` 指定本仓库 `e2e/.runtime` 下的数据目录。进程日志与进程 ID 保存在该目录，已有监听端口保持原样。

在已运行的验收实例上检查紧凑设置布局：

```powershell
node e2e/harness/companion-settings-review.mjs e2e/.runtime/settings-density
```

该检查沿用完整实测的成员数据，在 1351×833px 和 960×833px 下记录页面高度、滚动区域、奖章列数和截图；验证宽窗口无需滚动、无横向溢出、编辑取消、说明展开和另一成员资料查看，不提交成员凭据或修改精选选择。先构建桌面前端以加载最新页面；可添加 `--baseline` 仅采集布局尺寸与截图。

人设入口检查使用 `node e2e/harness/persona-review.mjs e2e/.runtime/persona-review`，在真实 Wails 页验证“伙伴与奖章”内的人设入口默认折叠、独立侧栏入口移除、折叠保留草稿与 Ctrl+S 保存，以及重新打开保持、恢复默认、未保存关闭确认和响应式布局，结束时恢复原人设。模型请求中的人设注入及旧入口隔离由 `TestAssistantPersonaPersistsAndReachesOnlyCompanionRequest` 捕获验证；本地浏览器实例未配置可用模型，不用该检查评估人设回复质量。

| 端口 | 用途 |
| --- | --- |
| 34115 | 主桌面实例，快捷命令旁宠物进入助手；启用养成后从助手或“伙伴与奖章”进入伙伴页 |
| 34116 | 本地排名 HTTP 服务，独立 `social.db` |
| 34117 | 隔离 SSH 回显端点，测试成功派发及失败目标 |
| 34118 | 完整实测中的第二个桌面实例，验证他人的排名与资料 |

测试成员由本地服务的管理员接口创建，属于两个隔离组织。账号及测试端点信息写入本次目录的 `endpoint.json`；它包含本次测试凭据，不纳入 Git。桌面连接凭据通过 Windows Keyring 保存，本地 `achievements.json` 不保存凭据明文。测试实例关闭后，其数据与 Keyring 凭据仍可供后续检查。

## 验收范围

已运行的 34115 实例可执行 `node e2e/harness/companion-roster-review.mjs` 验证七种伙伴：从设置页逐一预览、保存与同步公开资料，检查经验、领养时间及自定义人设保持；随后验证待机眨眼、点击招呼与开心回应、减少动态效果及窄窗口。结束时通过正式接口恢复原有伙伴与公开展示，不清空存档。输出位于 `e2e/.runtime/roster-review`。

- 从默认关闭开始，通过真实设置保存启用功能；领养、命名和换色，检查招呼与休息动作、陈旧修改版本拒绝和伙伴成长每日上限。
- 检查八类像素奖章、最多三枚精选和伙伴主入口；个人养成、功能统计与公开参与各自独立。
- 通过 Wails 业务接口真实保存脚本、快捷指令、排查记录与知识归档，查阅五个知识文件，经本地 SSH 通道派发脚本和快捷指令；失败派发不计数。
- 验证重复操作去重、旧编辑版本拒绝、同文件归档保留、删除制作成果后重算及使用记录保留。
- 两个独立桌面分别领养并同步伙伴与成果；页面切换分类、查看排名和另一成员的正式像素资料，跨组织读取返回 404，公开资料不包含经验、存档 ID 或业务明细。
- 停止真实排名服务，离线退出并保存新成果，重启桌面和服务，先确认退出再暂停上传；重新参与后从 Keyring 读取凭据并补传，重复同步不增加数量。
- 关闭养成后保留助手入口、停止读取个人伙伴，实际连接不再写入伙伴或奖章存档；重新启用恢复原伙伴。
- 在实际产品页检查 1440px 与 960px 窗口的明暗主题、图像加载、布局溢出、运行时换色及减少动态效果；保存他人资料截图与伙伴实际动效视频。同时检查 794×833px 的伙伴展示、大幅角色、260–340px 大幅场景、最多三枚已获奖章在背景右侧纵排、名称与阶段及经验条同一行显示、图标收起按钮和无介绍文案、键盘聚焦提示、稳定姿态连续运动及收起后问答空间与草稿保留。

结果写入本次目录的 `evidence/result.json`、`visual-result.json` 与实际页面截图；失败时记录 `failure.json` 与可获取的页面截图。业务创建部分通过真实 Wails 绑定调用；设置启用、领养、改名换色、账号认领、参与、精选、排名、资料查看和同步通过页面控件操作。

该实测验证本机持久化、完整产品链路和正式素材的实际显示。SSH 使用回显服务器，派发通过不等于远端业务执行成功；不调用 LLM，不访问实际用户的 SSH 服务器。跨日成长和所有阶段由领域测试与素材审核覆盖；Teams 真实适配器集成另执行 `npm --prefix plugins/teams-opscopilot run test:integration`。内网部署及外部 Teams 宿主人工试用单独记录。

动作与成本复查使用 `companion-motion-preview.mjs` 和 `companion-performance-review.mjs`，检查动作收尾、缓存传输、离屏暂停、输入与角色选择开销。`social-resume.ps1` 会保留验收目录中的 `legacy-assets` 图片，使已打开的旧标签在重建后继续显示；刷新页面加载最新版代码，当前未落盘问答不被强制清除。
