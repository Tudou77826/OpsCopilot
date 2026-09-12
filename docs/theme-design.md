# OpsCopilot 主题与皮肤设计

> 状态：讨论中 · 负责角色：OpsCopilot 产品与插件维护者（人员待认领）
> 关联文档：[OpsCopilot 花园系统设计](garden-design.md)

本文只覆盖主题与皮肤能力。花园系统的内容见关联文档，两者的边界在第 7 节。

## 1. 背景与范围

主题能力有两个独立的驱动因素，它们指向同一套基础设施。

**其一，现有暗色/亮色主题存在适配缺陷。** 部分元素只在一种主题下成立，最典型的是被无条件引入的第三方暗色样式，以及在组件里写死、不随主题变化的颜色。这些缺陷目前没有门禁拦截，因此可以长期存在。

**其二，需要以插件形态接入 iCode Teams 并复用共享前台。** 复用已经成立——`ProductChrome` 的工具栏、框架、导航由桌面壳与插件挂载同一套实现，并有架构测试防止任一宿主自行重写。但 OpsCopilot 的视觉语言与 iCode Teams 不协调，需要一个皮肤来完成适配。

这两个因素的关系是：**皮肤只能覆盖走语义令牌的部分。** 任何写死的颜色都位于 shadow 边界之内，皮肤无法触及。因此“把硬编码收敛进令牌”既是在修现有缺陷，也是皮肤的前置条件，两项工作重合，不应重复实施。

范围：

- **在范围内**：OpsCopilot 共享前台的语义令牌体系、模式（暗色/亮色）与皮肤（默认/宿主适配）两层结构、iCode Teams 皮肤、以及支撑它们的样式门禁。
- **不在范围内**：终端、会话、快捷命令、脚本和文件能力的业务位置与使用习惯；花园收藏物与场景的外观（花园拥有独立固定视觉体系）。

## 2. 设计结论

1. **主题拆成两个正交的轴**：模式（`dark` / `light`）与皮肤（`default` / `teams`）。模式决定色彩的明暗方向，皮肤决定色彩来源、圆角、密度与字体。两者独立组合，互不枚举。
2. **皮肤是一层对宿主令牌的映射，不是一套硬编码配色。** iCode Teams 皮肤的色值取自宿主公开的 `--ui-*` 样式契约，宿主调整配色时自动跟随。
3. **宿主模式下，OpsCopilot 的模式跟随宿主。** 宿主已有完整的亮暗两套令牌，皮肤只做映射即可同时覆盖两种模式。
4. **修现有主题缺陷是皮肤的前置工作，不是可选项。**
5. 不改变业务能力的布局与操作习惯；皮肤的差异限定在色彩、圆角、密度与字体。

## 3. 目标与非目标

目标：

1. 让 OpsCopilot 在桌面和 iCode Teams 等不同宿主中保持原有布局与操作习惯，同时在视觉上与宿主协调。
2. 消除现有暗色/亮色主题下的适配缺陷，并建立能防止其复发的门禁。
3. 保持主题、皮肤、业务能力和宿主适配之间的边界，使新宿主接入不需要复制 UI、不需要另写一套组件、不需要迁移业务数据。

非目标：

- 不重新设计 OpsCopilot 的顶部工具栏、终端、右侧会话区和快捷命令区布局。
- 不让共享组件直接依赖宿主内部实现、宿主组件库或宿主私有选择器。
- 不要求宿主管理 OpsCopilot 的配置、连接、脚本、会话或花园数据。
- 不改变花园收藏物与场景的外观（见[花园系统设计](garden-design.md)第 4 节）。

## 4. 现状与缺陷

这一节是前置工作的依据。缺陷按成因分四类，均给出可核对的位置。

### 4.1 令牌层本身是完整的

`frontend-shell/src/ui/styles/shell-theme.css` 是颜色的唯一真相源：暗色在 `:root` 声明，亮色在 `:root[data-theme="light"]` 覆盖。实测暗色 95 个自定义属性、亮色覆盖 93 个，未覆盖的 2 个是非颜色的滚动条尺寸，继承默认值即正确。

暗色色板取自 VS Code Dark+；**亮色是暖白工作区**（纸白画布、奶油白浮层、暖灰分区、低饱和琥珀交互色，主色 `#855b23`），不是中性浅色板。注意 `shell-theme.css` 文件头仍写着“亮色取自 One Half Light”，该注释已过期，与文件内的实际取值和 `appearance.contract.test.ts` 的“暖白画布”断言都不一致，应随本次工作修正。

因此缺陷根源不在于令牌缺失，而在于**没有走令牌的地方**。

### 4.2 第三方暗色样式被无条件引入

`flexlayout-react/style/dark.css` 在共享壳与桌面壳中被无条件引入：

- `frontend-shell/src/ui/styles.css:11`
- `frontend/src/main.tsx:14`

共享壳的覆盖文件 `frontend-shell/src/ui/FlexLayout/flexlayout-dark.css` 只覆盖约 20 个选择器，第三方暗色样式中未覆盖的部分在亮色下仍生效。按可见度排列：

| 未覆盖的选择器 | 表现 |
| --- | --- |
| `.flexlayout__popup_menu_container` | 标签溢出菜单整块黑底 |
| `.flexlayout__drag_rect` | 拖拽预览为近黑色方块 |
| `.flexlayout__border_tab_contents`、`.flexlayout__border_button--*` | 边缘停靠标签近黑，本地文件完全未覆盖 border 系列 |
| `.flexlayout__tab_overlay` | 拖拽遮罩为黑色半透明 |
| `.flexlayout__tab_button_textbox`、`.flexlayout__splitter_handle` | 重命名输入框与分隔条手柄为深灰 |
| `.flexlayout__tabset-selected`、`.flexlayout__tabset-maximized` | 渐变背景为黑到深灰，目前依赖本地规则的 `!important` 压制 |

这些是交互触发（拖拽、溢出菜单、停靠），最符合“某些元素在某些主题下不合适”的体感。

该文件同样被打进 iCode Teams 插件包：`plugins/teams-opscopilot/src/ui.tsx` 引入 `frontend-shell/src/ui/styles.css`，且插件的 `app.tsx` 确实使用 `FlexLayoutAdapter`。因此在亮紫色宿主中，停靠区也是暗的——同一个缺陷同时构成第 1 节的两个驱动因素。

另有一处已核实的类名不匹配：本地覆盖写的是 `.flexlayout__floating_window`（`flexlayout-dark.css:91`），而库定义的是 `.flexlayout__float_window`（第三方 `dark.css:672`）。本地选择器不对应任何库类名，浮窗及其标题栏、内容区的样式实际从未被覆盖。

### 4.3 未走令牌的硬编码

组件内联样式与部分 CSS 中存在假定深色背景的写法，按类型归纳：

- **固定色值的语义色**：`frontend/src/components/AI/ai-components.css:823` 的 `color: #d7ba7d` 覆盖了同选择器上的 `var(--stage-orange)`，配合 `:262`、`:822` 的 `color-mix(..., #000)` 往浅底混黑，亮色下形成“中灰底浅黄字”，对比度不达标。
- **用白色透明度做层次**：`frontend/src/components/Sidebar/MessageRenderer.tsx:145`、`:167`、`:237`、`:243` 以 `rgba(255,255,255,.02~.1)` 实现引用块、行内代码、表头与斑马行，亮色下白叠白，元素几乎不可见；`frontend-shell/src/ui/filetransfer/FilesPanel.tsx:2511`、`:2641` 的白色内高光同理。
- **固定暗灰的实心色块**：`frontend-shell/src/ui/FlexLayout/FlexLayoutAdapter.tsx:682` 的静音态胶囊 `rgba(60,60,60,.8)`，亮色下是突兀的深灰块。
- **写死的主色与焦点环**：`frontend-shell/src/ui/script/ScriptEditorModal.tsx:51` 用 `rgba(0,122,204,.2)` 做焦点环，`frontend-shell/src/ui/quickcmd/GroupStrip.tsx:96` 用 `rgba(0,122,204,...)` 做光晕——亮色主题的 accent 是琥珀 `#855b23`，这两处不跟主题。
- **未走语义令牌的状态底色**：`frontend-shell/src/ui/session/XshellImportDialog.tsx:350`、`:351`、`:358` 使用固定的绿/琥珀半透明底，而非 `--success-bg-subtle` 一类令牌。
- **写死的图表色**：`frontend/src/components/Monitoring/Sparkline.tsx:24` 的默认填充色。

模态遮罩类的 `rgba(0,0,0,.5~.9)` 在两种主题下都成立，属于正常用法，不在缺陷之列。

### 4.4 终端旁路不随主题变化

终端配色本身跟随主题（`Terminal.tsx` 由 `getTerminalTheme(theme)` 注入，切换时广播到所有实例），但两条旁路不跟随：

- `frontend-shell/src/ui/Terminal/search/SearchController.ts:18-23` 的搜索匹配/激活高亮色固定为暗橄榄底与深色前景。
- `frontend-shell/src/ui/settings/HighlightRulesModal.tsx:156` 新建高亮规则的默认色为 `#1d3a5a` 底、`#ffffff` 字，亮色终端下突兀。

另需注意 `Terminal.tsx:50` 的 `theme` 默认值为 `DEFAULT_THEME`（暗色），任何未显式传入主题的渲染路径会出现“界面亮色、终端暗色”的错位。

### 4.5 缺失项

- 全仓库没有 `::selection` 定义，选中文字不受主题控制。
- 没有统一的 `:focus-visible` 规则，各组件自行处理或直接 `outline: none`。
- `frontend/src/components/SettingsModal/AboutPanel.tsx:436` 的品牌 logo 未使用 `--brand-logo-filter`，亮色下与顶栏 logo 表现不一致。
- `frontend/src/components/BottomBar/BottomBar.tsx:133` 的 `var(--text-muted)777` 是值非法的声明，该处颜色回退为继承值。

### 4.6 缺陷长期存在的原因

`frontend-shell/src/ui/appearance.contract.test.ts` 只解析 `:root[data-theme="light"]` 块，从不枚举暗色块做集合比对；对比度断言只覆盖 4 组文字/背景与若干点值，约 20 个令牌。因此存在两处盲区：新增暗色令牌而漏写亮色覆盖不会被发现；组件里出现写死的色值完全不在检测范围内。`frontend/src/themes/sharedTheme.test.ts` 只断言宿主不重复定义令牌，同样不查覆盖完整性。

## 5. 主题模型：模式 × 皮肤

### 5.1 两个轴

| 轴 | 载体 | 取值 | 决定 |
| --- | --- | --- | --- |
| 模式 | `data-theme` | `dark`（默认）/ `light` | 色彩的明暗方向 |
| 皮肤 | `data-skin` | `default` / `teams` | 色彩来源、圆角、密度、字体 |

拆成两轴而不是扁平三选一，原因是：

- 宿主是“同一套语义、亮暗各一套取值”的结构，皮肤本身不区分模式。扁平三选一无法表达“Teams 皮肤 + 暗色”，也无法表达“Teams 皮肤 + 亮色”。
- 后续接入第二个宿主只需新增一个皮肤值，不必扩大模式枚举。
- 密度、圆角、字体与明暗无关（宿主的暗色块只覆写颜色与投影，不改圆角与间距），把它们放在模式轴上语义是错的。

### 5.2 令牌解析顺序

令牌按三层解析，后者覆盖前者：

1. **模式层**（`shell-theme.css`）：`:root` 为暗色全套，`:root[data-theme="light"]` 覆盖为暖白全套。这是完整兜底，保证任何未被皮肤映射的令牌都有正确取值。
2. **皮肤层**：`:root[data-skin="teams"]` 覆写可映射的令牌。该块不区分模式，其取值指向宿主令牌，宿主切换模式时自动跟随。
3. **组件层**：组件一律使用 `var(--xxx)`，不出现字面色值。

因此皮肤是**部分覆写**：宿主没有对应语义的令牌（见 6.4）继续使用模式层的取值，在两种模式下都正确。

实现约束：`:root[data-skin="teams"]` 与 `:root[data-theme="light"]` 的选择器权重相同，皮肤块必须在文件中位于模式块之后；构建时的 `:root`→`:host` 改写不改变该顺序，但任何后续的 CSS 重排都需保持这一约束。

### 5.3 皮肤不改变模式

模式只由 `data-theme` 决定，皮肤不得隐式改写它，也不得发明 `*-dark` 形式的平行令牌。后者是宿主在其样式规范中明确禁止的做法，理由是平行变量会让消费者在换模式时漏回默认皮肤。

## 6. 宿主适配：iCode Teams 皮肤

### 6.1 宿主的视觉契约

iCode Teams 以自研 CSS 自定义属性 `--ui-*` 作为视觉真相源，定义在 `src/client/ui/design-system.ts` 的 `:root`，经网关注入为全局样式表。该文件明确注明这套变量**同时是插件 uiApi 的样式契约**，并与 antd 的 `ConfigProvider` 同步（`src/client/ui/theme.ts`）。

宿主已提供完整的亮暗两套：`:root[data-theme='dark']` 以同名变量覆写全部颜色与投影，注释写明“同名变量覆写、插件自动跟随”。圆角、间距、字号不在暗色块中覆写，属于与模式无关的皮肤属性。

宿主的切换机制与 OpsCopilot 现有实现同构：`<html data-theme>` 驱动，首帧内联脚本防闪烁，无本地存储时读 `prefers-color-scheme`，手动切换后写入 `localStorage['ui.theme']`。

关键约束：**宿主不向插件注入主题信息。** 插件的挂载入口只接收容器元素，UI 契约中只有类名常量，没有颜色或模式字段。

### 6.2 映射机制

映射依赖两条既有事实，无需宿主改动，也无需提升 UI 契约版本：

1. **CSS 自定义属性是可继承属性，会穿过 shadow 边界。** 插件入口在 shadow root 内渲染，宿主 `:root` 上定义的 `--ui-*` 会继承到 shadow host，在 shadow 内部可直接引用。宿主的暗色块以同名变量覆写，因此引用宿主变量的取值会随宿主模式自动翻转。
2. **构建脚本已处理选择器改写。** `plugins/teams-opscopilot/build.ts` 在打包时把 CSS 中的 `:root[...]` 改写为 `:host([...])`、`:root` 改写为 `:host`，把令牌层搬进 shadow root。新增皮肤块不需要改动构建管线。

因此皮肤块只需写成宿主变量的引用并给出兜底值：

```css
:root[data-skin="teams"] {
  --accent: var(--ui-color-primary, #855b23);
  --bg-primary: var(--ui-color-bg, #faf8f3);
}
```

在 iCode Teams 中取到宿主取值并按宿主模式翻转；在桌面壳中 `--ui-*` 不存在，落到兜底值。这也意味着**桌面壳的渲染结果不因新增皮肤而改变**。

### 6.3 首批映射

宿主没有对应语义的令牌不在表内，它们继续使用模式层取值。

| OpsCopilot | iCode Teams | 说明 |
| --- | --- | --- |
| `--bg-primary` | `--ui-color-bg` | 页面画布 |
| `--bg-secondary` | `--ui-color-surface-sunken` | 侧栏与次面板 |
| `--bg-tertiary` | `--ui-color-fill` | 卡片内分区 |
| `--bg-elevated` | `--ui-color-surface` | 浮层与卡片表面 |
| `--bg-input` | `--ui-color-surface` | 输入框表面 |
| `--bg-hover` | `--ui-color-fill` | 悬停底 |
| `--bg-active` | `--ui-color-primary-soft` | 选中/激活底 |
| `--bg-active-soft` | `--ui-color-primary-faint` | 激活柔和变体 |
| `--border` | `--ui-color-border` | 主边框 |
| `--border-subtle` | `--ui-color-divider` | 次级边框 |
| `--border-focus` | `--ui-color-ring` | 焦点边框 |
| `--text-primary` | `--ui-color-text` | 主文字 |
| `--text-secondary` | `--ui-color-text-secondary` | 次级文字 |
| `--text-tertiary` | `--ui-color-muted` | 三级文字 |
| `--text-muted` | `--ui-color-faint` | 弱化文字 |
| `--text-disabled` | `--ui-color-disabled` | 禁用文字 |
| `--text-on-accent` | `--ui-color-surface` | 覆盖在主色上的文字，与宿主按钮一致 |
| `--accent` | `--ui-color-primary` | 主强调 |
| `--accent-hover` | `--ui-color-primary-hover` | 主强调悬停 |
| `--accent-soft` | `--ui-color-primary-soft` | 主强调柔和底 |
| `--success` | `--ui-color-success` | 成功 |
| `--success-bg-subtle` | `--ui-color-success-soft` | 成功柔和底 |
| `--info` / `--info-bg-subtle` | `--ui-color-primary` / `--ui-color-primary-soft` | 宿主没有独立信息色，归入主色 |
| `--danger` | `--ui-color-danger` | 危险 |
| `--danger-bg-subtle` | `--ui-color-danger-soft` | 危险柔和底 |
| `--shadow-dialog` | `--ui-shadow-block` | 浮层投影，按宿主更轻的投影基调整体调低 |
| 圆角族 | `--ui-radius` / `--ui-radius-card` / `--ui-radius-block` | 见 6.5 |
| 间距族 | `--ui-space-1/2/3`、`--ui-space-inner/block` | 见 6.5 |
| 字号族 | `--ui-font-size-*` | 见 6.5 |
| 字体族 | `--ui-font-family` | 见 6.5 |

### 6.4 宿主没有的语义

宿主不提供以下语义的令牌：警告色、tooltip、风险等级配色、严重度配色、状态徽标配色、文件类型图标配色、滚动条配色。

处理原则：这些令牌保持模式层的取值，不做猜测性映射。由此产生一个应当被接受的限制——**Teams 皮肤不会让界面完全等同于宿主风格**，风险等级色、图标族等仍是 OpsCopilot 自己的语言。这是有意的：为它们硬凑宿主配色会失去语义辨识度，也会在宿主调整配色后失真。

如果后续确认这些语义需要在宿主中存在，正确做法是推动宿主把它们加入 `--ui-*` 契约，而不是在皮肤里写死近似值。

### 6.5 密度、圆角与字体

这是皮肤的主要工作量，也是“只换颜色看起来仍然不搭”的原因。两边的量级差异明显：

| 维度 | OpsCopilot 现状 | iCode Teams |
| --- | --- | --- |
| 圆角 | 4 / 6 / 8 / 20px | 9 / 12 / 16px |
| 正文基准 | 11–18px 档位 | 1rem（16px） |
| 控件高度 | 未统一 | 32px（antd 默认） |
| 字体族 | 系统默认 | Inter + 系统栈 |
| 块内边距 | 未统一 | 24px |

OpsCopilot 目前的圆角与字号是 `frontend-shell/src/ui/settings/settingsStyles.ts` 中的像素常量，不是主题令牌，无法被皮肤覆写。因此需要先把它们令牌化（例如 `--radius-sm/md/lg`、`--font-size-xs..xl`、`--space-*`、`--control-height`），再让皮肤映射到宿主取值。

令牌化与映射是两个步骤：令牌化本身就有价值（消除散落常量），映射才产生皮肤效果。

### 6.6 模式跟随

宿主已提供亮暗两套，皮肤只做映射即可覆盖两种模式。但需要一条跟随规则，否则会出现错位：若宿主处于暗色而 OpsCopilot 的模式为亮色，未被皮肤映射的令牌会取亮色模式层的值，落在暗色宿主背景上必然失衡。

规则：

- 在 iCode Teams 中，OpsCopilot 的模式默认**跟随宿主的当前模式**（读取外层文档根元素的 `data-theme`），并跟随其运行期切换。
- 用户可以显式覆盖为固定的暗色或亮色，该覆盖按用户偏好持久化；恢复“跟随宿主”需作为可选项提供，避免有意的覆盖被静默回退。
- 桌面壳没有宿主模式，沿用现有默认（暗色）与用户设置。

### 6.7 桌面壳

桌面壳不定义 `--ui-*`，皮肤块中的引用全部落到兜底值，渲染结果与现状一致。这使桌面壳成为皮肤行为的对照基线：新增皮肤不应改变桌面壳的任何渲染输出，可用截图比对作为回归手段。

## 7. 与花园的边界

花园拥有独立的固定视觉规范，不随模式与皮肤切换场景、植物形态、品质颜色和成长语义（见[花园系统设计](garden-design.md)第 4 节）。

从主题侧看需要满足两点：

- 模式与皮肤只影响花园入口在工具栏中的普通、悬停和选中状态，以及花园面板外侧的宿主边界；
- 花园面板内控件使用花园自己的固定样式，其对比度要求由花园设计负责，不由主题令牌保证。

花园固定视觉体系带来的接缝处理要求，由花园设计负责，见该文档第 4 节。

## 8. 门禁与验收

现有门禁的两处盲区（4.6）必须补上，否则第 4 节的缺陷会随迭代复发。iCode Teams 已有一套可直接参照的做法：`npm run lint:style` 的裸色值扫描加双主题完整性校验，再由 CI 契约测试作为第二道。

### 8.1 裸色值扫描

除令牌文件与终端配色数据外，组件中不得出现字面色值。检测范围包括共享前台与宿主壳的 TS/TSX/CSS，豁免项为 `shell-theme.css`、`terminalSchemes.ts` 与皮肤块自身。这条直接对应 4.3 的全部缺陷。

### 8.2 双主题完整性

`:root` 中声明的每个颜色与投影令牌，必须在 `:root[data-theme="light"]` 中有同名覆写；`data-skin` 的每个皮肤块中引用的宿主变量必须存在于宿主契约清单。这条对应 4.1 的盲区，也是后续增加令牌时的防漏网。

### 8.3 对比度

对比度断言从当前的抽样扩展到全部文字/背景语义组合，覆盖两种模式与每个皮肤。这条对应 4.3 中“对比度不达标”一类问题。

### 8.4 皮肤验收

- 皮肤块的取值全部来自宿主契约变量或兜底值，不出现字面色值。
- 桌面壳在引入皮肤前后的渲染输出一致（截图比对）。
- iCode Teams 宿主中，切换宿主亮暗时 OpsCopilot 跟随，且界面上不出现未映射令牌造成的失衡色块。
- 共享组件不含宿主私有选择器与宿主组件库依赖。
- 同一花园状态在各模式与皮肤下的场景与植物快照保持一致。

## 9. 分阶段实施

### 第一阶段：令牌收敛与门禁

- 修正 `shell-theme.css` 文件头已过期的亮色色板注释；
- 补齐 4.2–4.5 的缺陷：改造或替换第三方暗色样式引入方式、清理写死色值、补齐 `::selection` 与统一 `:focus-visible`、修正 logo 与畸形声明；
- 把圆角、间距、字号、控件高度令牌化，替换 `settingsStyles.ts` 中的常量；
- 建立 8.1–8.3 三道门禁。

此阶段不引入皮肤，但产出皮肤所依赖的令牌层。

### 第二阶段：皮肤机制

- 引入 `data-skin` 轴与令牌解析顺序；
- 实现 iCode Teams 皮肤块与 6.3 的映射；
- 实现 6.6 的模式跟随与用户覆盖；
- 按 8.4 验收。

### 第三阶段：宿主扩展

按需增加皮肤，扩展只增加映射，不改动组件、不新增模式。

## 10. 待确认事项

1. 花园入口与快捷命令入口是否统一放在现有右侧工具栏——该决定同时影响入口选中态在皮肤下的规范。
2. 在 iCode Teams 中首次进入时的默认模式是否直接跟随宿主，还是先给出一次可见提示。
3. 用户显式覆盖模式后，回到“跟随宿主”的入口放在哪里。
4. 宿主缺失语义（警告色、风险等级、严重度、状态徽标、图标族、滚动条）是否推动宿主纳入 `--ui-*` 契约，还是在皮肤内保持默认语言。
5. 令牌化后的圆角与密度是否允许 `default` 与 `teams` 使用不同取值，还是所有皮肤共用 OpsCopilot 自己的密度，仅换颜色。
