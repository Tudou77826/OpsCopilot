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

再补一条核实到的事实，它决定了修法：**库的两套调色板声明在同一个选择器 `.flexlayout__layout` 上**（`--color-text`、`--color-background`、`--color-1..6`、`--color-drag1/2`、`--color-overflow`、`--color-icon` 等），`light.css` 与 `dark.css` 结构一致，只有少量声明不同——`.flexlayout__border_button`（暗色多 `border-radius`、`box-shadow`）、`.flexlayout__tabset-maximized`（暗色多一层 `background-image` 渐变），暗色另有 `.flexlayout__tab_top`、`.flexlayout__tab_bottom`。因此存在比继续追加 `!important` 覆盖更彻底的修法：换 `light.css` 作结构基座，把库的 `--color-*` 映射到语义令牌，删除本地覆盖文件。第 9 节第 2 步按此实施。

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

皮肤层的取值一律是 `var(--ui-x, <兜底>)`，而兜底写法有硬约束——实测（真实浏览器，三种写法各注入一次读 computed 值）：

| 写法 | 解析结果 | 后果 |
| --- | --- | --- |
| `var(--ui-x, var(--bg-primary))` | 空值 | 自定义属性自我引用成环，皮肤层与模式层一起失效 |
| `var(--ui-x)` | 空值 | 不会回落到上一条声明，也不会自动降级 |
| `var(--ui-x, var(--mode-bg-primary))` | 兜底生效；宿主变量存在时取宿主值（实测 22px 覆盖 11px） | 正确 |

因此模式层为每个被映射的令牌另存一份**字面量镜像** `--mode-<token>`（暗亮各一份），皮肤块引用镜像作兜底；镜像不能写成 `var(--本体)`（与皮肤块对同一属性的覆写成环）。契约测试逐条断言镜像与本体取值相等，防止漂移。

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

OpsCopilot 目前的圆角与字号是 `frontend-shell/src/ui/settings/settingsStyles.ts` 中的像素常量，不是主题令牌，无法被皮肤覆写。因此需要先把它们令牌化（`--radius-*`、`--font-size-*`、`--space-*`；控件高度并入 `--space-*` 刻度，见步骤 5），再让皮肤映射到宿主取值。

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

现有门禁的两处盲区（4.6）必须补上，否则第 4 节的缺陷会随迭代复发。iCode Teams 已有一套可直接参照的做法：`npm run lint:style` 的裸色值扫描加双主题完整性校验，再由 CI 契约测试作为第二道。实际落地为 `tools/checks/check-style-tokens.mjs` 的四条检查，自测在 `tools/checks/check-style-tokens.test.mjs`（`node --test`，8 条用例，含 fixture 反证）。

### 8.1 裸色值扫描

除令牌文件与终端配色数据外，组件中不得出现字面色值。检测范围包括共享前台与宿主壳的 TS/TSX/CSS，豁免项为 `shell-theme.css`、`terminalSchemes.ts` 与皮肤块自身。这条直接对应 4.3 的全部缺陷。`color-mix(in srgb, var(--x) N%, transparent)` 与全透明字面量放行：前者随令牌走，后者不构成主题依赖。

### 8.2 双主题完整性

`:root` 中声明的每个颜色与投影令牌，必须在 `:root[data-theme="light"]` 中有同名覆写；`data-skin` 的每个皮肤块中引用的宿主变量必须存在于宿主契约清单。这条对应 4.1 的盲区，也是后续增加令牌时的防漏网。与模式无关的令牌（从文字色派生的 `--layer-*`）在行尾标注 `mode-independent` 豁免，每次运行打印豁免清单，避免它变成静默例外。

### 8.3 尺寸令牌引用完整性

第 5 步引入 `--space-*`/`--radius-*`/`--font-size-*` 时补的一条。自定义属性写错名字不会报错，只会让整条声明静默失效、属性回落初始值，页面直接塌掉；所以每个 `var()` 引用都必须在令牌文件里有声明，且尺寸令牌只能声明在 `:root`（尺寸与明暗无关，写进亮色块意味着两模式密度不同）。这条是本步"机械替换 144 处"能安全落地的依据。

### 8.4 对比度

对比度断言从当前的抽样扩展到全部文字/背景语义组合，覆盖两种模式与每个皮肤。这条对应 4.3 中“对比度不达标”一类问题。

### 8.5 皮肤验收

- 皮肤块的取值全部来自宿主契约变量或兜底值，不出现字面色值。
- 桌面壳在引入皮肤前后的渲染输出一致（截图比对）。
- iCode Teams 宿主中，切换宿主亮暗时 OpsCopilot 跟随，且界面上不出现未映射令牌造成的失衡色块。
- 共享组件不含宿主私有选择器与宿主组件库依赖。
- 同一花园状态在各模式与皮肤下的场景与植物快照保持一致。

## 9. 落地步骤与验证

排序原则：先建立"能不能验证"，再改代码。缺陷修复（第 2–4 步）与令牌化（第 5 步）必须在皮肤之前完成——它们是皮肤的前置条件（第 1 节），也共用同一套门禁。每一步独立可验收、可单独回退；除第 9 步外都不改变桌面壳的渲染结果，因此每一步都能用"桌面壳与基线一致"作为兜底回归。

验证工具的现状需要如实说明：两个仓库都没有像素级视觉回归工具。本方案用**令牌级等价断言**（在真实浏览器里读 computed 值，判断皮肤在无宿主变量的环境下降级为模式层取值）替代像素比对，它比截图更精确、可进 CI；只有真正由交互触发的面（拖拽预览、溢出菜单、停靠标签、浮窗）保留人工截图复核。是否引入像素级工具是独立决策，见第 10 节。

### 步骤总览

| # | 步骤 | 主要产出 | 依赖 |
| --- | --- | --- | --- |
| 1 | 门禁与基线 | 三条门禁 + 两模式基线截图 | — |
| 2 | FlexLayout 第三方样式令牌化 | 去掉 `style/dark.css`，库调色板映射到语义令牌 | 1 |
| 3 | 硬编码与缺失项收敛 | 4.3、4.5 全部消除 | 1 |
| 4 | 终端旁路与主题错位 | 4.4 全部消除 | 1 |
| 5 | 尺寸类令牌化 | `--radius-*`/`--font-size-*`/`--space-*`（含控件高度） | 1 |
| 6 | `data-skin` 轴与解析顺序 | 皮肤机制（只有 `default`） | 5 |
| 7 | iCode Teams 色彩映射 | 6.3 映射，宿主内验收 | 6 |
| 8 | 模式跟随与用户覆盖 | 6.6 行为 | 7 |
| 9 | 密度、圆角与字体映射 | 6.5 尺寸映射（取决于待确认 5） | 8 |
| 10 | 门禁收口与元验证 | CI 接入 + 门禁自检 | 9 |

### 步骤 1：门禁与基线

改动：

- 新增 `tools/checks/check-style-tokens.mjs`（仓库根，带目录参数以便用 fixture 自测），实现三条检查：除 `shell-theme.css`、`terminalSchemes.ts` 与皮肤块外，TS/TSX/CSS 不得出现字面色值；`:root` 中每个颜色与投影令牌必须在 `:root[data-theme="light"]` 有同名覆写；FlexLayout 映射文件必须覆盖库声明的每个 `--color-*` 变量且入口不得再引入库的暗色样式。前两条的写法参照宿主 `tools/checks/check-style-tokens.mjs`（把 `--ui-*` 换成我们的令牌前缀，把 `:root[data-theme='dark']` 换成 `:root[data-theme="light"]`）。
- 门禁自身的用例放在 `tools/checks/check-style-tokens.test.mjs`，用 `node --test` 运行（`npm --prefix frontend-shell run lint:style:self`）。放在 vitest 里会需要给浏览器代码放开 Node 类型，削弱 `boundaries.test.ts` 的防线。
- `frontend-shell` 与 `frontend` 各加 `lint:style` 入口。**接入 CI 放在第 10 步**：门禁与对比度在修完第 2–4 步之前是红的，提前接入会让 main 一直失败。
- 对比度断言从 `appearance.contract.test.ts` 现有的 4 组抽样扩到全部文字/背景组合、覆盖两模式。
- 基线取证：`start_dev.bat` 起桌面壳，在两种模式下截取标签条、标签页、分隔条、边缘停靠标签、tab 溢出菜单、拖拽预览、拖拽遮罩、浮窗，以及消息渲染器的引用块、行内代码、斑马行，存入 `docs/assets/theme-verification/baseline/`。
- 修正 `shell-theme.css` 文件头已过期的亮色色板注释。

验证（顺序不可颠倒）：

1. **门禁必须先跑出红**：在修复前运行，确认裸色值检查命中 4.3 列举的每一处；再临时新增一个只有暗色取值的令牌，确认双主题完整性检查失败。门禁没有红过就不算建成，这一步本身就是元验证。

   实测基线（2026-09-12，本轮）：裸色值 **98 处**，覆盖 4.3 列举的全部位置；双主题完整性**通过**（暗色 95 个令牌、亮色覆写 93 个，与 4.1 一致）；扩到全组合的对比度断言发现 **35 组不达标**（暗色 20、亮色 15），其中一类是用户可见的：亮色下 `--text-on-accent` 落在 `--bg-active` 上只有 1.30，而文件传输的段选控件正是这个组合（`FilesPanel.tsx` 的 `segmentedActive`）。这批即第 3 步的修复清单。

   门禁自身另有 6 条用例（`tools/checks/check-style-tokens.test.mjs`），覆盖"该红必红、该绿必绿"，含用 fixture 证明缺覆写与漏映射会被抓到。
2. 门禁接线可用：`node tools/checks/check-style-tokens.mjs`、`npm --prefix frontend-shell run lint:style`、`npm --prefix frontend run lint:style` 均有确定退出码。
3. 基线截图存在且可复核（两模式共 22 张）。

门槛：第 1 条的命令输出留存，写入 release notes 的 `## 验证`。

### 步骤 2：第三方 FlexLayout 样式令牌化

依据是 4.2 补充的事实：库的调色板声明在 `.flexlayout__layout` 上，两套文件结构一致。

改动：

- `frontend-shell/src/ui/styles.css` 与 `frontend/src/main.tsx` 的 `flexlayout-react/style/dark.css` 改为 `style/light.css`；
- 新增 `frontend-shell/src/ui/styles/flexlayout-tokens.css`，写成 `.flexlayout__layout { --color-text: var(--text-primary); --color-background: var(--bg-primary); … }`，把库声明的每个 `--color-*` 映射到语义令牌。该块与模式无关，映射后随 `data-theme` 自动翻转；
- 删除 `frontend-shell/src/ui/FlexLayout/flexlayout-dark.css`（含那条匹配不到任何库类名的 `.flexlayout__floating_window`）与桌面壳的转发文件；`frontend/src/components/FlexLayout/` 下如仍有必须保留的规则，逐条迁入新文件并注明原因；
- 显式决定是否保留暗色独有的两项结构差异。实测结论：**不保留**。`.flexlayout__tab_top`/`.flexlayout__tab_bottom` 的 inset 阴影与 3px 圆角、`.flexlayout__border_button` 的 inset 阴影与圆角、以及 `.flexlayout__tabset-maximized` 的渐变只存在于库的暗色文件里，属暗色装饰；换基座后自然消失。标签圆角若要保留，应在第 5 步作为我们自己的 `--radius-*` 令牌给出，而不是抄库的暗色取值。

验证：

1. 枚举检查（门禁第三条，`check-style-tokens.mjs`）：解析库 `light.css` 声明的全部 `--color-*` 变量（实测 51 个），断言每个都在映射文件里有取值；同时断言两个样式入口不再引用 `style/dark.css` 且都引用了 `style/light.css`。库升级新增变量时该检查失败，而不是静默漏色。
2. 静态测试：两处 `style/dark.css` 引用消失；`flexlayout-dark.css` 不再被任何文件引用。
3. computed 值断言（真实浏览器，不用 jsdom）：把 `data-theme` 依次设为 `light`/`dark`，读取 `.flexlayout__layout` 上 `--color-background`、`--color-text` 的解析结果，断言等于 `--bg-primary`、`--text-primary` 的解析值；再断言 `.flexlayout__tab`、`.flexlayout__tabset` 的 `background-color` 与令牌一致。只断言变量名存在是自证，必须断言解析后的颜色。
4. 交互取证：逐一触发这 8 个面。亮色下不得出现暗块，暗色下需与基线一致。这是本步唯一不能自动化的一环。

   实测（2026-09-12，本机 dev 应用 + 阿里云会话）：库的 51 个 `--color-*` 全部映射，两模式下逐项读取，12 组库变量与其语义令牌的解析值**完全相等**；标签条、标签页、会话侧栏与终端在两种模式下均无暗块（亮色下终端背景 `#faf8f3`、正文字色 `#39362f`，与语义令牌一致）。4.2 表中"未覆盖"的选择器现在都跟随令牌：

   | 选择器 | 暗色解析值 | 亮色解析值 | 原缺陷 |
   | --- | --- | --- | --- |
   | `.flexlayout__popup_menu_container` | `#2a2a2a` | `#fffdf8` | 标签溢出菜单整块黑底 |
   | `.flexlayout__drag_rect` | `#2d2d2d` | `#eae4d8` | 拖拽预览为近黑方块 |
   | `.flexlayout__border_tab_contents` | `#1e1e1e` | `#faf8f3` | 边缘停靠标签近黑 |
   | `.flexlayout__tab_button_textbox` | `#3c3c3c` | `#fffefa` | 重命名输入框深灰 |
   | `.flexlayout__splitter_handle` | `#555555` | `#a99a84` | 分隔条手柄深灰 |
   | `.flexlayout__float_window` | `#252526` | `#f2eee5` | 类名不匹配，从未被覆盖 |

   `.flexlayout__tab_overlay` 两模式下都是库自己的 24% 中性黑遮罩（`rgba(0,0,0,.24)`），与 4.3 对遮罩的判断一致，未强行改。

   溢出菜单与拖拽预览需要多标签或拖拽手势才出现，所以用注入同级节点读级联结果的方式验证（上表即探针结果），未为此连接更多服务器；真实截图覆盖标签条、标签页、侧栏与终端，存 `docs/assets/theme-verification/baseline/connected-layout-{dark,light}.png`（空壳状态另有 `empty-shell-{dark,light}.png`）。

门槛：第 4 条的亮色截图逐面复核通过。若某面仍有暗色残留，说明还有未映射的库变量，回到第 1 条补齐。

### 步骤 3：硬编码与缺失项收敛

改动：

- 4.3 的六类逐条替换为语义令牌：固定色值的语义色、以白色透明度做层次、固定暗灰实心块、写死的主色与焦点环、未走令牌的状态底色、写死的图表色；
- 新增四组令牌承接这些位置：`--scrim-soft/-strong/-heavy`（遮挡层，亮色用暖褐而非纯黑）、`--layer-hairline/-1/-2`（从文字色派生的半透明层次，暗色提亮、亮色压暗，故与模式无关）、`--bg-sunken`（凹陷表面，取代"把画布混黑"）、阴影改用既有的 `--shadow` 与 `--shadow-dialog`；
- 4.5 的缺失项：`::selection` 与统一的 `:focus-visible` 落进 `shell-theme.css`（规则一套，取值走随模式切换的令牌；组件自定义焦点用「类名 + :focus-visible」覆盖）、`AboutPanel` 的 logo 改用 `--brand-logo-filter`、`BottomBar.tsx:133` 的 `var(--text-muted)777` 修正；
- 新增令牌必须同步补两模式取值，否则门禁红。

验证：

1. 门禁裸色值从 **98 处降到 12 处**（30 个文件，82 处替换，每处都校验过命中次数），剩下 12 处全部属于 4.4 的终端旁路，留给第 4 步。
2. 对比度用例从 **35 组不达标转为全绿**。修复不是靠目测：先用脚本按 4.5 阈值反算每个令牌所需的最小调整量，再按方向分块改值（语义色保饱和、过于靠色的"柔和底"压深、反色文字那对压暗底色）。最终改动 16 个取值，暗色如 `--text-muted #888→#a6a6a6`、`--accent-hover #1f8ad3→#1470b0`、`--success-bg-subtle #2e5a3a→#1e3a26`，亮色如 `--warning #b08800→#866700`、`--severity-warning #bf8700→#8f6500`。
3. `--layer-*` 是半透明叠加层，不能当实心表面算对比度。用例里补了按 alpha 合成到具体底色的计算（只认 `color-mix(in srgb, var(--x) N%, transparent)`，出现别的写法直接报"无法计算"），并只对真实出现在这些层上的主/次文字断言。
4. 真实浏览器复核：亮色下 `--warning`/`--severity-warning`/`--scrim-soft`/`--bg-sunken`/`--shadow` 均为改后的取值，`--layer-1` 解析为 `color-mix(in srgb, #39362f 5%, transparent)`——证实层次令牌确实按当前模式的文字色派生。

门槛与遗留：新增令牌无遗漏（门禁证明）；4.3 列举的每一处都有断言或截图。**4.5 第 3 条（键盘 Tab 遍历断言可见焦点）尚未自动化**，目前只有规则存在性；取证截图不得包含设置页（该页按产品决定明文显示密钥）。

### 步骤 4：终端旁路与主题错位

改动：

- 新增 `terminalSearchDecorations` 与 `terminalHighlightDefaults`（均在 `terminalSchemes.ts`）：搜索命中与高亮规则默认色属于 xterm 配色数据，必须给具体色值、不能用 CSS 变量，所以与终端配色放在同一处、按主题两套；
- `Terminal/search/SearchController.ts` 不再持有模块级的写死装饰色，改为构造时接收 `() => Theme` 取值函数（复用 `Terminal.tsx` 已有的 `themeRef`，主题切换时无需重建控制器）；
- `settings/HighlightRulesModal.tsx` 新建规则的默认前景/背景改为 `getTerminalHighlightDefaults(currentTheme())`；
- `Terminal.tsx` 的 `theme` 默认值由 `DEFAULT_THEME` 改为 `currentTheme()`；`appearance.ts` 新增 `currentTheme()`（以 `<html data-theme>` 为准），供拿不到 theme 属性的组件取值；
- 顺带修正一处既有可读性缺陷：暗色下"当前匹配"用亮琥珀底（`#f59e0b`）配浅灰前景，对比度只有 **1.45**，基本读不出内容；改为深琥珀底 `#7a4a00` + 亮边框 `#ffd75f`（滚动条指示条仍是亮色）。

验证：

1. `Terminal.search-highlight.test.tsx`（壳与桌面各一份）改为断言"取当前主题的装饰色"，并新增亮色用例；不再复述字面值，取色真相源只有 `terminalSchemes` 一处。
2. `appearance.contract.test.ts` 新增：两种主题下，匹配底与当前匹配底分别与终端前景色的对比度 ≥ 4.5（这条抓出了上面那处 1.45）；新建高亮规则默认前景/背景对比度 ≥ 4.5；两套主题取值互不相同。
3. `currentTheme()` 单测：以 `data-theme` 为准，缺失或非法值回退暗色。

门槛与遗留：第 2 条用等比计算覆盖了"可读"，但**第 3 条的实机截图未做**——搜索面板需要聚焦终端后按 Ctrl+F 再输入才能出现命中高亮，本次未在真实界面上取到。另外发现 `Terminal.search-highlight.test.tsx` 在壳与桌面各存一份完全重复的用例，桌面包那份测的只是转发组件，属重复维护，本次只同步了断言语义，未做合并。

### 步骤 5：尺寸类令牌化（不改外观）

改动：

- `shell-theme.css` 的 `:root` 新增一套与明暗无关的尺寸令牌：圆角 6 档（`--radius-xs/sm/md/lg/full/circle`）、字号 10 档（`--font-size-xs`…`2xl` 与三个 rem 显示档）、间距刻度 19 档。间距刻度把**像素值写进名字**（`--space-8: 8px`）：这样"某个值该用哪个令牌"没有判断空间，换密度时按刻度整体换算即可；间距与控件高度共用一套刻度，与 Tailwind 的做法同构。
- `settingsStyles.ts` 的 `radius`/`font` 两个导出对象由像素值改为令牌引用。这是本步的杠杆：所有消费者（壳设置页、桌面壳旧设置页、`DiagnosePanel`）不改一行就随皮肤走。
- 设置 UI 的尺寸字面量机械替换为令牌，共 **144 处**（`settingsStyles.ts` 28 处、壳设置页六个组件 116 处）。脚本逐处打印 `行号: 属性 '旧值' → '令牌'` 供核对后再写盘。
- 有意保留字面量的三类：`width`/`maxWidth`/`minWidth` 等布局几何（皮肤改的是密度与圆角，不是布局宽度）、`88vh` 与 `auto`、负偏移（布局对齐，不属密度刻度）。

验证：

1. **零外观变化的硬证据**：新增 `settingsStyles.contract.test.ts`，把导出对象里 31 个样式的 **64 个**尺寸字段解析后与冻结表逐条比对。这条用例在令牌化**之前**跑是绿的（字面量即取值），令牌化**之后**跑仍是绿的（`var()` 解析回同名令牌取值），两次绿之间的差集就是"外观没变"。同一文件还断言三件事：`--space-N` 必须等于 `Npx`（否则按刻度换算密度会算错）、每个尺寸字段都必须走令牌（令牌化前这条列出 **64 处**未走令牌的字段，令牌化后为 0）、布局几何**不得**被令牌化。
2. **引用完整性**：门禁新增检查 4（8.3），并当场验证它有效——故意把 `ShellSettingsModal.tsx` 的一处 `var(--space-12)` 改成 `--space-13`，门禁报 `ShellSettingsModal.tsx:276 引用了未声明的尺寸令牌 --space-13` 并以退出码 1 结束；还原后恢复通过。这条比截图更能兜住"写错名字 → 声明静默失效 → 页面塌掉"。
3. 全量回归：`frontend-shell` 21 文件 185 用例、`frontend` 35 文件 242 用例、两包 `build`、`go test ./...`、`go vet ./...` 全绿；门禁自测 8/8。
4. 真实浏览器（运行中的桌面壳，只读计算值、不取页面文本）：`<html>` 上 18 个尺寸令牌全部解析为预期像素（`--space-8 → 8px`、`--radius-sm → 4px`、`--font-size-base → 13px`…），且没有任何尺寸令牌声明在亮色块。

门槛与遗留：

- 第 1 条零差异成立。第 2 条的红/绿两次输出都留存。
- 原第 3 条（两模式截图与基线逐张一致）**改为上面的令牌级断言**：本步唯一改动的是设置页，而设置页按产品决定明文显示密钥、不纳入截图取证（步骤 3 的门槛）；非设置面则用"本步没有改动任何非设置文件"直接证明，比截图更确定。将来若引入像素级工具（第 10 节），这条可补回。
- 遗留：`frontend/src/components/SettingsModal/` 是桌面壳当前使用的旧设置页（2310 行），其中可令牌化的尺寸字面量还有 **253 处**。它的圆角与字号已经通过 `settingsStyles.ts` 的导出跟随令牌，但自带的内边距/间距仍是字面量，因此在密度映射下不会跟随。这属独立的机械收敛，复用同一脚本即可，但需要单列一次提交——混进本步会把 144 处的可复核 diff 变成 397 处，反而看不清。

### 步骤 6：`data-skin` 轴与解析顺序

改动：

- `appearanceTypes.ts` 增加 `Skin` 类型；`appearance.ts` 增加 `DEFAULT_SKIN`、`normalizeSkin`（只接受 `default`/`teams`，其余回落 `default`）、`currentSkin`、`applySkin(skin, root?)`，以及与主题键分开的 `SKIN_STORAGE_KEY` 与 `persistSkin`/`readPersistedSkin`。
- `data-skin` 写到与 `data-theme` 同一元素（桌面壳是 `<html>`，插件是 shadow host）。`applySkin` 的 `root` 参数就是为插件传 shadow host 预留的。
- `shell-theme.css` 末尾加 `:root[data-skin="teams"]` 块。本步为空块——只建立机制，`data-skin="teams"` 与 `default` 渲染完全一致。
- 决定：**皮肤不进后端 `AppearanceConfig`**。用哪个皮肤由宿主决定、桌面壳恒为 `default`，所以只存前端的独立 localStorage 键，不需要 Go 侧往返；用户明暗偏好仍走原有键与后端配置。

验证：

1. 单元用例（并入 `appearance.contract.test.ts`，该文件从 20 条增至 28 条）：归一化只接受 `default`/`teams`（`undefined`/`null`/空串/`'dark'`/`'teams-x'` 均回落）；写皮肤不动主题键；`applySkin` 写入与 `data-theme` 同一元素且 `currentSkin` 读回。
2. 顺序约束（静态断言）：断言 `:root[data-skin="teams"]` 在文本上位于 `:root[data-theme="light"]` 之后。5.2 的实现约束靠人记不住，用用例钉住。
3. **桌面壳无副作用**：在运行中的桌面壳（无宿主变量）里，把 `data-skin` 依次设为 `default`/`teams`，对**两种模式**各取一次全量计算样式指纹——`document.querySelectorAll('*')` 覆盖 **436 个元素**，每个元素取 `backgroundColor`/`color`/`borderRadius`/`padding`/`margin`/`gap`/`fontSize`/`fontFamily`/`borderColor`/`boxShadow`/`outlineColor`/`outlineWidth` 共 12 个属性。结果：两模式下 `default` 与 `teams` 的指纹哈希**完全相同**（暗色 `-1460168067`、亮色 `-1420089030`），6.3 映射表里的 30 个令牌逐项零差异。同皮肤连测两次哈希一致（证明指纹可复现，上述相等不是抖动）；暗亮两模式哈希不同（证明这个指纹有分辨力，不是恒等式）。另确认运行中的样式表确实含 `[data-skin]` 规则，排除"测的是旧 CSS"。
4. 门禁两条新断言——皮肤块取值必须来自 `var(--ui-*)`、模式层不得引用宿主变量——在本步是空跑（皮肤块还没有声明），它们随第 7 步写映射立刻生效。之所以放在契约测试而不是 shell 门禁：`shell-theme.css` 整体在裸色值扫描的豁免名单里，皮肤块的字面色值只有解析该块才能发现。

元验证（两次故意注入，都要先红）：

- 把皮肤块移到亮色块之前 → 顺序断言报 `找不到皮肤块: expected 7033 to be greater than 7063`，1 条失败。
- 在皮肤块里写死 `--bg-primary: #123456` → 2 条失败：等价断言报 `暗色 --bg-primary: teams #123456 ≠ default …`，皮肤块契约报 `--bg-primary: #123456`。还原后 28 条全绿。

门槛与遗留：第 3 条成立，可以进入步骤 7。第 4 条的"截图与基线一致"改为上面的全量计算样式指纹比对：登记的基线是空壳状态，而当前应用已连着会话（且设置页不纳入截图），逐张对比不可复现；指纹比对覆盖了每个元素的 12 个属性，比截图更精确且已进 CI。遗留：桌面壳首屏内联脚本暂不读皮肤键（桌面壳恒为 `default`，不产生 FOUC），插件的首屏跟随在第 8 步处理。

**皮肤入口与持久化（2026-09-14 补）**：原先 `persistSkin`/`readPersistedSkin`/`currentSkin` 只有测试在用，插件里写死 `applySkin('teams')`，桌面壳从不设 `data-skin`——机制在、但没有入口。现在：插件首帧按 `readPersistedSkinChoice() ?? 'teams'` 应用（新增该函数是因为"没选过"与"选成了 default"必须分开：宿主内默认跟随宿主，桌面壳默认 default，同一个键不能既表默认又表选择）；外观页在 `ThemeChoiceCard` 之后多一张 `SkinChoiceCard`（`ShellSettingsModal` 的可选 `skin` 属性注入，桌面壳不注入所以看不到——在没有宿主令牌的环境里换皮肤是**空操作**，已用 436 元素 × 12 属性的指纹证明 teams 与 default 逐项等值，所以那张卡在那里只会误导）；换皮肤只写前端键 `opscopilot-skin` 并 `applySkin(next, surface)`，不碰明暗、不写后端配置。

实测（宿主内，宿主保持暗色不动）：`teams` → `--bg-primary #1c1c1c`、`--accent #af87ff`；在外观页点"OpsCopilot 默认" → 皮肤键落 `default`、**宿主仍是暗色、插件也仍是暗色**，而令牌切成 OpsCopilot 自己的暗色 `#1e1e1e` / `#2a2a2a` / `--accent #007acc`；整页重载后 `data-skin` 仍为 `default`（持久化生效）；点回"跟随 iCode Teams"令牌回到宿主的 `#1c1c1c` / `#af87ff`。这就是两轴正交的可操作证据：换皮肤只换颜色来源，明暗一个 bit 都没动。

### 步骤 7：iCode Teams 色彩映射

改动：`:root[data-skin="teams"]` 按 6.3 写全映射，每个值形如 `var(--ui-color-xxx, <兜底>)`；兜底取模式层默认值，保证宿主缺变量时不崩。

验证：

1. 门禁：皮肤块内不出现字面色值；每个引用的宿主变量名必须存在于随仓库维护的契约清单。契约清单的取法（已按宿主实际文件形态修正）：`src/client/ui/design-system.ts` 导出的是一个模板字符串，内含 `:root{…}` 与 `:root[data-theme='dark']{…}` 两段、`--ui-*` 变量共 46 个，所以快照是对整份文件文本做 `--ui-[a-zA-Z0-9-]+` 提取而非解析某个 `:root {}` 块，且必须在快照里注明取自哪个 revision（见第 10 节第 8 条）。清单同时是"宿主改契约我们能知道"的提醒机制，更新方式需写入文档。其中"皮肤块取值必须来自 `var(--ui-*)`""模式层不得引用宿主变量"两条已在步骤 6 立起来（在 `appearance.contract.test.ts`，因为 `shell-theme.css` 整体在裸色值扫描的豁免名单里，皮肤块的字面色值只有解析该块才能发现）；本步只需补宿主变量名清单。
2. 真实宿主冒烟：本地起宿主（`ICODE_LOCAL_CREDENTIAL=local-dev-credential npm run dev`）→ 打包并应用 OpsCopilot 插件 bundle → 打开插件页。在宿主 `light`/`dark` 两种状态下读取插件内根元素的令牌 computed 值，断言 `--bg-primary` 等于宿主 `--ui-color-bg`、`--accent` 等于宿主主色。这是"映射真的穿过 shadow 边界"的唯一证据，6.2 的两条推论都在这里被检验。

   **已用注入模拟验证到机制层（2026-09-13）**：宿主服务运行时（API 45831 / UI 45833，插件页 `#/plugins/opscopilot`），把当前 `shell-theme.css` **原样套用 `build.ts` 的选择器改写规则**（`:root[…]`→`:host([…])`、`:root`→`:host`）后临时注入插件自己的 shadow root，读到的结果——宿主文档 `--ui-color-bg: #fafafa`、`--ui-color-primary: #9470c4` 确实出现在 shadow host 元素上，也出现在 shadow 内部元素上（继承穿透成立）；`data-skin` 切到 `teams` 后，映射令牌全部取宿主值（`--bg-primary` `#faf8f3`→`#fafafa`、`--accent` `#855b23`→`#9470c4`、`--text-primary` `#39362f`→`#18211c`、`--success` `#197c36`→`#08783e`、`--danger` `#cf222e`→`#a12622`、`--border-subtle` `#e9e2d6`→`#ececf1`），6.4 不映射的 `--warning`(`#866700`) 与 `--severity-danger`(`#d1242f`) 一个都没动，且没有空值令牌；尺寸令牌在 shadow 内解析正常（`--radius-sm` 4px、`--space-8` 8px、`--font-size-base` 13px）。注入已撤除、属性已还原。**这只是模拟**：没有验证 `pack.ts`/`build.ts` 的产物体（改写规则是照抄其代码，未跑打包）与 12 个面的观感。
3. 覆盖矩阵：工具栏、导航、会话栏、终端外框、快捷命令面板、脚本、文件、设置、对话框、tooltip、滚动条、风险与严重度徽标共 12 个面，在宿主两种模式下截图。6.4 明确不映射的语义（风险等级、状态徽标、图标族、滚动条）标注为"预期保持默认语言"，不算缺陷。
4. 反例检查：故意把某个宿主变量名写错，确认兜底值生效、界面不崩，证明 6.4 的降级语义成立。

已落地的一半（2026-09-13）：

- `shell-theme.css` 按 6.3 写入 **27 条**映射，值一律 `var(--ui-x, var(--mode-<token>))`；模式层两处各新增 **27 条**字面量镜像（共 54 条），理由见 5.2 的兜底写法表——这是本步唯一偏离原计划的写法：原计划写的是"兜底取模式层默认值"，但模式层默认值无法用 `var()` 表达（写 `var(--本体)` 会成环），所以改成镜像副本。
- 宿主契约快照落在 `frontend-shell/src/ui/styles/teams-host-contract.json`：46 个名字，取自 `icode-teams` 的 `origin/main` = `ed88a1d6c4d1`，由 `tools/checks/extract-teams-host-contract.mjs` 生成（宿主路径用 `ICODE_TEAMS_REPO` 传入，脚本本身不含机器路径）。
- 契约测试 `appearance.contract.test.ts` 增至 **31 条**，本步新增/强化的四条：皮肤块取值形状必须严格是 `var(--ui-*, var(--mode-*))`；引用的宿主变量必须都在快照清单内（先断言引用的确非空，避免空转）；兜底镜像与模式层本体逐项相等；快照自身可信（非空、名字规范、带 40 位 revision）。**两次故意注入**验证有效：把镜像取值改一位、把宿主名拼错、去掉一条兜底，三条断言分别报错（外加等价断言兜住，共 4 红），还原后 31 条全绿。
- **无宿主环境下的真实浏览器验证**（桌面壳，非宿主内）：① 不注入宿主变量时，teams 与 default 在全部映射令牌与未映射令牌上零差异，且没有任何令牌解析为空值——这是镜像兜底在真实 CSS 引擎里确实生效（不成环、不漏兜底）的证据；② 注入 `origin/main` 亮色块的真实 `--ui-*` 取值后，15 个抽样映射令牌**全部**跟随宿主（`--bg-primary` → `#fafafa`、`--accent` → `#9470c4`、`--text-primary` → `#18211c`），而 6.4 明确不映射的 `--warning`/`--severity-danger`/`--risk-moderate-fg`/`--overlay` **一个都没被改动**，即 6.4 的降级语义成立；③ 注入宿主变量的同时切到亮色，令牌仍取宿主值（皮肤不区分模式）；④ 渲染指纹：436 个元素 × 8 个属性在 teams 与 default 下哈希相同，非空皮肤块对桌面壳零影响。
- 第 4 条"反例检查"中的"宿主变量名写错"在桌面壳等价于"宿主变量全部缺失"（就是上面 ①），已证明不缺兜底、不塌且逐项等于模式层；真正的"写错名字 + 宿主存在"要在宿主内验证。

**打包产物的真实宿主冒烟（2026-09-14，已完成）**：插件已整合进本仓库 `main`（见第 10 节第 10 条），并已按同事给的流程挂到运行中的宿主上——`npm run package` → `POST /api/bundles/apply`（`Authorization: Bearer local-dev-credential`，body 为 `{manifest, artifactPath}`，`artifactPath` 是**目录**而非文件）。返回 `200`，组件状态 `{"id":"opscopilot","version":"0.1.1","health":"healthy","lifecycle":"active"}`，`#/plugins/opscopilot` 完整渲染出工具栏、终端区空状态、快捷命令栏、会话管理面板、脚本录制面板。

1. **27/27 映射逐条取到宿主值**（在 `#/plugins/opscopilot` 内读插件 shadow host 后代的 computed 值，与宿主 `:root` 同名变量逐一比对）：`--bg-primary` `#fafafa`、`--bg-secondary` `#f7f7f9`、`--bg-tertiary` `#f4f4f6`、`--bg-elevated`/`--bg-input`/`--text-on-accent` `#fff`、`--bg-hover` `#f4f4f6`、`--bg-active`/`--accent-soft`/`--info-bg-subtle` `#f0eaf9`、`--bg-active-soft` `#f4f2fa`、`--border` `#d8e1db`、`--border-subtle` `#ececf1`、`--border-focus` `#c0abe4`、`--text-primary` `#18211c`、`--text-secondary` `#3f3f46`、`--text-tertiary` `#526359`、`--text-muted` `#71717a`、`--text-disabled` `#a1a1aa`、`--accent`/`--info` `#9470c4`、`--accent-hover` `#7a5aad`、`--success` `#08783e`、`--success-bg-subtle` `#e2f5e9`、`--danger` `#a12622`、`--danger-bg-subtle` `#fde9e7`、`--shadow-dialog` `0 1px 2px #18181b08`。**无一为空值，无一条走兜底**。
2. **A/B 对照**：在同一条 shadow host 上把 `data-skin` 切到 `default` 再切回 `teams`，令牌随之变化并复原——`--bg-primary` `#faf8f3`↔`#fafafa`、`--bg-elevated` `#fffdf8`↔`#fff`、`--accent` `#855b23`↔`#9470c4`、`--text-primary` `#39362f`↔`#18211c`、`--shadow-dialog` `0 16px 48px rgba(52,43,29,.16), 0 2px 8px …`↔`0 1px 2px #18181b08`。这条排除了"看起来变了但其实没变"的巧合。
3. **第 4 条反例检查（宿主内真做）**：在 shadow root 里临时注入一条把宿主变量名写错的规则（`--bg-primary: var(--ui-color-bg-typo, var(--mode-bg-primary))`），读回 `#faf8f3` / `#855b23`——**逐项等于模式层兜底值而非空值**，撤除注入后恢复 `#fafafa`。降级语义在真实宿主内成立。
4. **第 3 条 `--shadow-dialog` 目视结论：可接受**。宿主的 `--ui-shadow-block` 确实比我们的浮层投影轻得多，"新建连接"对话框因此明显变平，但对话框自身的 `--border`(`#d8e1db`) 与白色面板仍把层次撑住了，观感与宿主一致、不算失衡色块。按 6.4 的语言（向宿主靠），判定为预期代价而非缺陷；若日后宿主提供专门的浮层投影变量，优先映射过去。
5. 已截图取证的面：工具栏与终端外框（空状态）、快捷命令栏（含分组 `default`、`+ 添加`、搜索框）、会话管理面板、脚本录制面板（含禁用主按钮 `--bg-active` 与 `--text-disabled` 的"未录制"徽标）、新建连接对话框。**未覆盖**：终端（有会话时）、文件面板、tooltip 气泡、滚动条、风险与严重度徽标——前两者需要真实 SSH 会话；设置面按产品决定（明文显示密钥）不纳入截图；tooltip 悬停未复现出气泡，未取得证据。产物里的改写、属性落在同一个宿主元素上、以及 6.4 的不映射语义都已在前面几条里印证，这几个面留待有会话时补。

**这一轮顺带解决的两个前置问题**（都属于"不改就没法挂上去"）：插件的 `entry.ts` 原先要求宿主传 `config.host = {protocol, bundleId, version, artifactDirectory, dataDirectory}`，那是宿主 `demo/src/v2` 原型分支的契约；真实宿主对 `hostApi` 4/5/6 传的是 `config.dataDir`，所以插件必须迁到 `hostApi: '6'` + `uiApi: '1'`（同事在工作树里的未提交改动正是这个，已一并整合）。另外宿主的旧版本号内容不可变（`retained bundle version cannot change its contents`），所以插件版本从 `0.1.0` 升到 `0.1.1`。

另外两点实测（2026-09-13，运行中的宿主）：该宿主的文档根元素上**没有** `data-theme`（其检出早于暗色模式，见第 10 节第 8 条），所以"宿主亮暗两套"在当前宿主构建里并不可得。该宿主至今仍未换 revision，因此步骤 8 的跟随机制是在它上面**手工改 `data-theme`** 验证的（见步骤 8），暗色观感待换 revision 后补。

门槛与遗留：第 1、2、4 条成立，第 3 条在可及的面里无失衡色块、`--shadow-dialog` 目视可接受，第 7 步的"打包产物在真实宿主里跑通"这一环已完成。遗留：上面第 5 条列出的五个面（终端、文件、tooltip、滚动条、徽标）待有会话时补截图；设置面按产品决定不再纳入。

### 步骤 8：模式跟随与用户覆盖

改动：

- 插件 `ui.tsx` 不再写死 `host.dataset.theme = 'light'`；改为读取外层文档根元素的 `data-theme` 并监听其运行期变化；
- 统一插件内现有的两处主题状态（`ui.tsx` 的初始值与 `app.tsx` 的 `initialSettings.theme`、`surface.dataset.theme`）到一个来源；
- 用户显式覆盖后持久化，并另提供"恢复跟随宿主"入口；
- 桌面壳沿用现有默认与用户设置，不受影响。

验证：

1. 单元/集成用例：宿主属性 `light→dark` 变化时 shadow host 的 `data-theme` 跟随；显式覆盖后宿主再变化不再改写；恢复跟随后重新跟随。
2. 终端一致性：宿主切暗色后，断言终端配色（`getTerminalTheme` 的输出）与界面同时翻转，不出现"界面暗、终端亮"（4.4 的同型错位）。
3. 真实宿主：切换宿主主题，插件即时跟随，截图佐以终端背景的 computed 值。
4. 兜底：宿主根元素缺 `data-theme` 时的取值需明确并有用例——取默认暗色、默认亮色还是读 `prefers-color-scheme`，实现时确认。

门槛：第 1、2 条自动化通过，第 3 条实机通过；桌面壳截图与基线一致。

**已实施（2026-09-14）**：

- 共享壳 `appearance.ts` 新增跟随三件套（前端键，与主题键、皮肤键并列）：`ThemeFollow = 'host' | 'manual'`、`THEME_FOLLOW_STORAGE_KEY = 'opscopilot-theme-follow'`（默认 `host`）、`hostTheme()`、`observeHostTheme(listener)`。
- 关键设计：`hostTheme()` **宿主没有 `data-theme` 时返回 `undefined` 而不是回退 `DEFAULT_THEME`**。"宿主没有明暗"与"宿主说是亮色"是两件事，合成一个值会让旧宿主的插件被强行翻成暗色；返回 `undefined` 时保持插件自己的取值。
- "跟随与否"与"用户选了哪个"分开记：跟随期间展示宿主的明暗，但**不把宿主的取值写进 sidecar**，所以用户手动选过的那一份偏好原样留着，将来恢复跟随时不会丢。
- 插件侧：`ui.tsx` 首帧 `host.dataset.theme = hostTheme() ?? 'light'`；`app.tsx` 用 `themeFollow` 状态驱动一个 effect——`host` 时先对齐一次再 `observeHostTheme` 订阅运行期切换，`manual` 时不订阅。两处显式覆盖入口（工具栏明暗按钮、设置页里改明暗）都会把跟随置为 `manual`；`ProductToolbar` 新增可选 `hostThemeFollow` 属性，只在被覆盖时渲染一个「跟随宿主明暗」按钮，桌面壳不传该属性因此不受影响。

验证：

1. **共享壳用例 31 → 35 条**（`appearance.contract.test.ts` 新增"模式跟随宿主"一组 4 条）：归一化只接受 `host`/`manual`（`undefined`/`null`/空串/`'dark'`/`'light'`/`'host-x'` 均回落 `host`）；写跟随键不动主题键与皮肤键；`hostTheme()` 在属性缺失或写了非法值时返回 `undefined`、写了 `dark`/`light` 时原样返回；`observeHostTheme` 在宿主改属性后被回调一次、解除监听后不再回调。第 4 条**先红后绿**过：最初在同一个同步块里断言，看到的是空数组，改成真的等一轮宏任务才通过——这条同时证明了 MutationObserver 是下一轮任务投递。
2. **真实宿主（打包后的插件，`0.1.2`）完整走了一遍状态机**：
   - 宿主无 `data-theme` → 插件 `light`（兜底分支，与改动前一致）；
   - 宿主置 `dark` → 插件随之为 `dark`（跟随）；
   - 点插件工具栏明暗按钮 → 插件 `light`、`opscopilot-theme-follow = manual`、工具栏出现「跟随宿主明暗」；
   - 覆盖后宿主再 `light→dark` → 插件**停在 `light` 不动**（覆盖生效）；
   - 点「跟随宿主明暗」→ 插件立刻取到 `dark`、跟随键回到 `host`、按钮消失；
   - 之后再让宿主 `dark→light` → 插件随之为 `light`（确实回到跟随，不是碰巧对上）。
   收尾已把宿主的 `data-theme` 删回原状；期间明暗按钮把主题写回 sidecar 的值仍是原来的 `light`（已核对 exe 旁的 `config.json` 未变）。
3. 桌面壳：`frontend-shell` 216 条、`frontend` 242 条全绿，`ProductToolbar` 的新属性是可选且桌面不传，渲染结果不变。
4. 终端一致性（原第 2 条）**只在界面侧验证**：`getTerminalTheme` 的输入是同一个 `settings.theme`，`FlexLayoutAdapter` 与 `Terminals` 都吃它，代码路径上不存在第二个主题源（这正是 4.4 修掉的那类错位）。但**当前宿主没有暗色令牌**，"界面暗、终端亮"的实机截图仍取不到，留待宿主换到暗色 revision 后补。

**暗色实测与一个真实缺陷（2026-09-14，宿主已升级后）**：宿主那条线按同事的意图落定——先把暂存的原生插件传输层提交为 `fed9cd9`（hostApi 6 / 浏览器终端与文件通道），再把 `origin/main`（`c9d99e9`，领先 26 个提交）合并进来（`1cfd32e`，10 个冲突文件逐个按语义合并）。升级后宿主有了明暗：`<head>` 内联脚本按 `localStorage['ui.theme']` 优先、否则 `prefers-color-scheme` 决定，侧边栏底部是一个 antd Switch，写的是 `<html data-theme>`。

用宿主自己的开关（不是手工改属性）走了一遍：

- 宿主暗色（系统偏好，`#1c1c1c`）→ 插件 shadow host `data-theme` 为 `dark`，令牌全部取宿主暗色值：`--bg-primary` `#1c1c1c`、`--bg-elevated` `#262626`、`--text-primary` `#eeeeee`、`--accent` `#af87ff`、`--border` `#44444c`、`--shadow-dialog` `0 1px 2px #00000059`（即宿主暗色的 `--ui-shadow-block`），插件自己的 `--mode-bg-primary` 为 `#1e1e1e`。
- 点侧边栏开关 → 宿主 `data-theme` 由 `dark` 变 `light`、`localStorage['ui.theme']` 落 `light`，插件 shadow host **同步变 `light`**，令牌翻回 `#fafafa` / `#fff` / `#18211c` / `#9470c4`。再点回暗色同样跟随。
- 结论：**第 2 条（终端一致性/实机）随之补齐**——界面与终端用的是同一个 `settings.theme`，宿主切暗后两者同时翻，不再有"界面暗、终端亮"的可乘之机。

这一轮抓到一个我自己的实现缺陷：`settings.load` 是异步的，原来直接 `then(setSettings)`，而跟随同步在挂载时就跑完了——**持久化的旧值（`light`）在加载完成时把已对齐的宿主模式冲掉**，表现为宿主是暗色、插件 shadow host 仍是 `light`，于是映射过的令牌取到宿主的暗色、没映射的仍是亮色，出现混色。修法：加载回来的值在跟随时以宿主明暗为准（用 ref 读当下的跟随状态，避免闭包旧值）。修复后重新打包为 `0.1.3` 挂载复验，即上面的数据。这条也说明"挂载时对齐一次"不够，凡是有第二个异步来源写同一状态，都要在那里再对齐一次。

门槛与遗留：第 1–4 条全部成立，暗色观感已在升级后的真实宿主里确认。**宿主那条线的后续（推送、PR）由宿主仓库的负责人决定**，本仓只依赖它的行为契约。

### 步骤 9：密度、圆角与字体映射（取决于待确认 5）

前置：待确认 5 需先定论。若结论是"皮肤只换颜色"，本步取消，步骤 7 即终态。

改动：皮肤块引入宿主圆角、间距、字号与字体族。宿主不提供控件高度令牌（32px 是 antd 默认值，不在 `--ui-*` 清单内），控件高度只能取我们自己的令牌值，需明确是固定对齐 32px 还是维持现状。

验证：

1. 令牌级断言：宿主页内读取控件高度与圆角的 computed 值，与约定值一致；`default` 皮肤下这些值与本步之前完全相同，尺寸类改动不得外溢到桌面壳。
2. 变更面清单：本步必然改变宿主内观感，需按 6.5 的五个维度列出全部受影响面并逐面截图，与步骤 7 的截图对比，确认无裁切、错位、对比度下降。
3. 可访问性：字号与控件高度变化后，最小可点击区域与对比度断言仍通过。

门槛：第 1、3 条自动化通过，第 2 条逐面复核通过。

**范围修正（2026-09-14，采纳评审意见）**：字体**不进皮肤**。原提案把字体族与字号当皮肤的一部分，这与"终端应用"的核心体验冲突——终端字体/字号必须独立可调，而本项目的 `TerminalConfig` 已经拥有 `font_family`/`font_size`（`TerminalAppearanceCard` 提供 5 套等宽字体的字形预览、字号加减与 `Ctrl+滚轮`/`Ctrl+0` 快捷键）。若皮肤也写字体，同一个东西就有两个所有者，和"跟随被异步 load 覆盖"是同一类缺陷。据此把视觉相关的维度重新分成三组：

| 组 | 管什么 | 谁决定 | 现状 |
| --- | --- | --- | --- |
| 明暗（mode） | 亮/暗方向 | 用户，或跟随宿主 | 已做（跟随 + 覆盖 + 恢复 + 持久化） |
| 皮肤（skin） | **色彩来源 + 圆角（形状）** | 跟随宿主，或用我们自己的 | 色彩已做；圆角待定 |
| 人体工学（ergonomics） | 终端字体/字号、（将来）界面密度 | 用户自己，走独立设置 | 终端字体与字号已独立可调；界面密度没有 |

这个分法不是我们发明的，是主流产品的通行做法，而且与宿主自身一致：

- **宿主自己的主题就只有明暗算法 + 主色 + `borderRadius: 9`**（`src/client/ui/theme.ts` 的 `teamsTheme`），字体一个字都没有——"跟随宿主视觉语言"自然不包含字体。
- VS Code：颜色主题归主题，字体归设置，**图标主题是另一类主题**，高对比度又是**另一种主题类型**；编辑器字号与界面缩放都在设置里。
- Windows Terminal：color scheme（含 ANSI 调色板）与字体/光标/内边距分属不同配置段。
- Slack / Discord：主题是配色（含具名皮肤），消息密度与字号是独立设置。
- Chrome / Edge / Windows / macOS：明暗 + 强调色，字号与缩放独立。

所以结论：**皮肤只做色彩与圆角；字体、字号、界面密度属于人体工学，走独立设置。** 第 9 步的范围随之收窄为"圆角映射（+ 视需要挪出间距）"，而不是原提案的四类一起上。

**形状已实施（2026-09-15）**：评审确认"形状"属于皮肤，并明确"宿主不给这个参数也可以读宿主代码把值敲定"。实际情况是宿主的圆角**有契约**（`--ui-radius` 9 / `--ui-radius-card` 12 / `--ui-radius-block` 16，其 antd 主题也是 `borderRadius: 9`），所以 teams 皮肤直接引用宿主令牌、按角色对齐；Windows 那套没有宿主可读，按 Fluent 的 4px 控件 / 8px 卡片写死。

| 皮肤 | xs 徽标 | sm 控件 | md 卡片 | lg 弹窗 |
| --- | --- | --- | --- | --- |
| default | 3 | 4 | 6 | 8 |
| windows | 4 | 4 | 8 | 8 |
| teams | 3（不映射） | `var(--ui-radius, 4px)` | `var(--ui-radius-card, 6px)` | `var(--ui-radius-block, 8px)` |

- 徽标那档在 teams 里**故意不映射**：宿主对它没有契约（它自己在 `.nav-type` 里硬编码 4px），我们那档是 3px，1px 差别肉眼无感，不值得为它写死一个字面量。
- 契约测试随之分化：**颜色令牌**的兜底必须是 `var(--mode-*)`（有镜像），**形状令牌**的兜底必须是字面量（`--mode-*` 只镜像颜色）；另加三条：形状引用的正是宿主那三个圆角、自带调色板的皮肤圆角次序自洽（xs≤sm≤md≤lg）、windows 的形状就是 4/8。门禁自测补了一条 fixture，明确"皮肤块可以声明尺寸令牌，只有亮色块不行"。
- **一个必须说的教训**：令牌改对了不等于看得见。改完先量真实控件，发现 `新建连接` 与会话搜索框在三套皮肤下**都是 4px**——因为整个共享壳里 260 处 `borderRadius` 只有 57 处走了令牌，其余是内联写死的。本次把**当前可见面**上的 24 处收敛成令牌（`styles.css` 11、`ProductChrome` 4、`CommandGrid` 4、`SessionManager` 2、`FlexLayoutAdapter` 3），收敛后再量：`新建连接` 与原版/Windows 是 4px、teams 是 **9px**，与宿主的 antd 控件一致。
- **遗留**：仍有约 179 处 `borderRadius` 字面量，集中在对话框与面板（`FilesPanel`、`ScriptEditorModal`、`HighlightRulesModal`、`SmartConnectModal`、`productSettingsStyles` 等），以及 `styles.css` 里 12 处**不在刻度上**的取值（2/5/7/9/11/12/13px）。要让形状在所有面上一致，需要一次与步骤 5 同规格的机械收敛（逐处打印 + 冻结表 + 逐面截图），并先决定那些离刻度值归到哪一档。

**前置证据（2026-09-14，为待确认 5 取数，尚未实施）**：把宿主 `ed88a1d` 的 `design-system.ts` 与我们 `shell-theme.css` 的尺寸令牌并排看，宿主**确实**提供了圆角/间距/字号/字体族，但两套刻度并不一一对应：

| 维度 | 宿主 `--ui-*` | 我们 | 对应关系 |
| --- | --- | --- | --- |
| 圆角 | `--ui-radius` 9px、`--ui-radius-card` 12px、`--ui-radius-block` 16px | xs 3、sm 4、md 6、lg 8、full 20、circle 50%（6 档） | **不齐**：宿主最小 9px，我们的 3/4/6 无对应档；映射只能把我们的小圆角整体抬到 9px |
| 间距 | `--ui-space-1` 8、`--ui-space-2` 12、`--ui-space-3` 16、`--ui-space-inner` 12、`--ui-space-block` 16、`--ui-block-padding-x/y` 24、`--ui-card-padding` 18 | `--space-N` = Npx 共 19 档 | **齐**：8/12/16/18/24 逐一对得上，可直接映射 |
| 字号 | eyebrow 11px、caption 12px、note 13.6px、body 16px、h3 18.7px、h2 24px、h1 `clamp(2rem,6vw,4rem)` | xs 11、sm 12、base 13、lg 14、md 16、xl 18、2xl 20 | **近似**：xs/sm/base/md 有 1px 内对应（11/12/13.6/16），lg(14)/xl(18)/2xl(20) 无对应 |
| 字体族 | `Inter, ui-sans-serif, system-ui, sans-serif` | 系统栈（`-apple-system, BlinkMacSystemFont, 'Segoe UI', …`，见 `frontend/src/style.css`） | **不映射**（见上面的范围修正）：字体属于人体工学，终端字体/字号已由 `TerminalConfig` 独立拥有 |
| 控件高度 | 无（32px 是 antd 默认值，不在契约里） | 我们自己定 | 无处可映射 |

据此的建议（**这是产品判断，不是技术结论，需你定**）：

- **建议做**：间距（8/12/16/18/24 一对一，改动可预测）。但注意间距也偏"人体工学"（主流把密度放在设置里而非主题），所以它更适合做成独立的"界面密度"设置，而不是塞进皮肤；若只想在皮肤里做一件事，就做圆角。
- **不做**：字号（属于人体工学，终端的字号已独立可调；界面字号若也要可调，另立设置项）。
- **建议不做**：圆角。宿主只有一个 9px 的"控件圆角"档，我们最小的三档（3/4/6px）没有对应值，映射等于把所有小圆角统一抬到 9px；徽标、输入框、按钮会同时变圆，是比换颜色大得多的观感变化，且和 6.4「不映射语义保持默认语言」的理由同源。
- **不做**：控件高度（宿主没有契约）。

若结论是"只做间距 + 字体族"，那是一次小改动，风险面可控；若连字号与圆角一起换，必须按上面第 2 条逐面截图复核。若结论是"皮肤只换颜色"，本步取消，步骤 7 即终态。

补充一条取数结论：宿主的尺寸类变量（`--ui-radius*`、`--ui-space-*`、`--ui-font-size-*`、`--ui-font-family`）**只在 `:root` 声明一次，暗色块不覆写**（`design-system.ts` 里逐条核对过声明位置），所以把它们纳入皮肤只会让几何跟随宿主那**一套**刻度，与明暗无关——不会破坏"皮肤/模式正交"。若决定做圆角，建议按**角色**对齐而不是按数值：控件 `--radius-sm`(4px) → `--ui-radius`(9px)、卡片 `--radius-md`(6px) → `--ui-radius-card`(12px)、弹窗 `--radius-lg`(8px) → `--ui-radius-block`(16px)，徽标 `--radius-xs`(3px) 暂不映射（宿主没有对应小档）。代价是控件圆角从 4px 跳到 9px，比颜色变化明显，必须逐面复核且先看工具栏/输入框/徽标这三处。

### 步骤 10：门禁收口与元验证

改动：三条门禁接入 `shared-shell.yml`；对比度断言覆盖每个皮肤；8.4 的验收清单固化为 PR 自查项；基线截图更新为当前状态。

验证（证明门禁本身有效，而不是碰巧全绿）：

1. 三次故意注入，每次都要求失败：往组件加一行 `color: '#ff0000'` → 裸色值门禁失败；在 `:root` 加一个颜色令牌但不加亮色覆写 → 双主题完整性失败；把某处文字改成对比度不足 → 对比度用例失败。三次注入后回滚，门禁恢复全绿。
2. 门禁在 CI 上确实被执行：在 PR 上看到该 step 的运行记录，而非仅本机可跑。
3. 步骤 1 的基线与当前状态的每一处差异，都已在对应步骤中被解释或消除。

**已做的一半（2026-09-14）**：样式门禁接进 `.github/workflows/shared-shell.yml` 的 `verify` 作业，紧跟 `frontend-shell` 的 typecheck/test 之后、`frontend` 回归之前，用两条 npm 脚本而不是裸 `node` 命令，让门禁入口只有一处定义：

- `npm --prefix frontend-shell run lint:style:self`（门禁规则自测，8 条）——放在前一条，规则本身被改坏时先报，避免"规则失效但仍全绿"；
- `npm --prefix frontend-shell run lint:style:all`（全量扫描，187 个文件）。为此在 `frontend-shell/package.json` 新增 `lint:style:all`：原有的 `lint:style` 只扫 `frontend-shell/src`（93 个文件，本机快跑用），CI 要连 `frontend/src` 一起扫，用 `node tools/checks/check-style-tokens.mjs` 的默认根集。

本机三条命令都已跑通（自测 8/8；全量 187 文件通过；局部 93 文件通过）。**原第 1 条（三次注入）与第 2 条（在 PR 上看到运行记录）仍未做**：前者在本仓已由门禁自测的 8 条用例覆盖了规则的注入验证（见 8.3 与步骤 5），但没有"往组件里塞红字"这三次端到端注入；后者要等一次带 PR 的运行，本仓改为单分支后由 `push: [main]` 触发，还没有留下 CI 运行记录。第 3 条（基线与现状逐处对齐）已在步骤 1–8 的登记里逐条交代。

### 步骤 11：第三套皮肤（windows）与皮肤预览（2026-09-14）

评审意见：原版之外再要一套风格（定为 Windows 风格），并要有简易预览；"跟随"不作为选项。
据此把皮肤从"一个 teams 映射"扩展成"两类别、三种皮肤"：

| 皮肤 | 类型 | 写法 | 覆盖范围 |
| --- | --- | --- | --- |
| `default` | 模式层自带 | 无皮肤块 | 暖白亮色 + 深色暗色 |
| `windows` | **自带调色板** | 亮色一块 + 暗色一块（`[data-theme="dark"][data-skin="windows"]`，权重更高） | 表面、边框、文字、强调、语义色与浅底/描边、遮罩、阴影、滚动条、对话框别名 |
| `teams` | **宿主映射** | 一个块覆盖两种模式（值指向 `--ui-*`） | 6.3 的 27 条 |

- **机制结论**：皮肤允许两类作者——宿主映射型只需一个块（宿主自己按模式换变量），自带调色板型必须按模式各写一块。
- **不覆盖的仍然不覆盖**：风险等级、严重度、状态徽标、chip、图标族、拖放态、星标在 windows 皮肤里也沿用我们自己的语言（宿主没有这些语义色，Windows 也没有官方对应的）。**但严重度必须别名**：`--severity-*` 的可读性取决于皮肤自己的底色，沿用模式层那份（为暖白底调的）在 windows 更深的底上掉到 4.4x。用 `var(--success)` 一类别名而不是复制色值，暗色块覆写本体后别名自然取到当模式的值。
- **调色板是被算出来的，不是挑出来的**：把 windows 两种模式并入既有的对比度矩阵（正文×9 层表面、状态色×4 面板、成对前景/底色），首轮 5 处不达标，按算出的下限调整（`--text-muted` 亮色 `#757575`→`#5c5c5c`、`--warning` 亮色 `#9d5d00`→`#855200`、`--text-muted` 暗色 `#9a9a9a`→`#adadad` 等）后 45 条全绿。这条同时说明：**自带调色板的皮肤必须整体过矩阵，不能只测它改过的那几个令牌**——它改了底色，就会影响它没改的令牌。
- **预览**：`skinSwatch(skin, root)` 把候选皮肤临时写到目标元素上、读真实计算值再还原（同一任务内完成，不绘制中间态）。不把色值抄进 TS：抄一份必然与 CSS 漂移，而预览的意义就是"它实际长什么样"。空值时不画（样式表没加载时宁可不画也不要四个空格子）。
- **入口按环境过滤**：`availableSkins()` 只在有宿主令牌时才列出宿主映射型皮肤（`hostTokensAvailable()`）。桌面壳与插件都接上了皮肤卡；桌面壳另在 `index.html` 首屏脚本里读皮肤键，避免选了自带调色板的皮肤后先闪一帧默认配色。
- **一条必须写下来的规则（第三种皮肤把它照出来了）**：**宿主映射型皮肤把明暗也一起映射了**。teams 的颜色来自宿主变量，而宿主按自己的模式换那批变量；若此时插件再"手动覆盖明暗"，就会出现"映射过的令牌是宿主暗色、没映射的仍是插件亮色"的半暗半亮。所以：选 teams 即等于明暗跟随宿主（切换时自动置回跟随，加载时按持久化的皮肤推导初值），并在皮肤为 teams 时拒绝手动改明暗并给出解释；想自己定明暗就选自带调色板的皮肤（原版 / Windows）。
- **构建改写的一个真缺陷**：`build.ts` 原先把 `:root` 改成 `:host` 用的是 `/:root\[([^\]]+)\]/`，遇到连续属性选择器会在第一个 `]` 收尾，产出 `:host([data-theme="dark"])[data-skin="windows"]` —— 而 `:host()` 之后不能再接简单选择器，这条规则**静默失效**，windows 的暗色块在插件里就是不生效。改成 `/:root((?:\[[^\]]+\])+)/` 后正确产出 `:host([data-theme="dark"][data-skin="windows"])`。这条无法靠单测发现（jsdom 不跑层叠），只能靠"在真宿主里读计算值"。

实测（真宿主，2 个模式 × 3 个皮肤，逐项读计算值）：

| 皮肤 | 暗色 | 亮色 |
| --- | --- | --- |
| default | `#1e1e1e` / `#2a2a2a` / `#fff` / accent `#007acc` | `#faf8f3` / `#fffdf8` / `#39362f` / accent `#855b23` |
| windows | `#202020` / `#2b2b2b` / `#ffffff` / accent `#60cdff` / **强调面文字 `#000000`** | `#f3f3f3` / `#ffffff` / `#1b1b1b` / accent `#0067c0` |
| teams | `#1c1c1c` / `#262626` / `#eeeeee` / accent `#af87ff` | `#fafafa` / `#fff` / `#18211c` / accent `#9470c4` |

`--text-on-accent` 在 windows 暗色下是黑色不是白色——Windows 11 暗色的强调色是浅蓝，强调面上的文字就是黑的，这是它自己的语言。

## 10. 待确认事项

1. 花园入口与快捷命令入口是否统一放在现有右侧工具栏——该决定同时影响入口选中态在皮肤下的规范。
2. 在 iCode Teams 中首次进入时的默认模式是否直接跟随宿主，还是先给出一次可见提示。**实施时按"直接跟随、不提示"落地**（宿主已经表达了明暗，再加一次询问是多余的）；若产品上更希望有一次可见告知，改的是 `ui.tsx` 首帧与 `app.tsx` 的初值，机制本身不用动。
3. 用户显式覆盖模式后，回到“跟随宿主”的入口放在哪里。**实施时放在插件工具栏明暗按钮右侧**（仅被覆盖时出现；`ProductToolbar` 的新属性是可选的，桌面壳不传所以看不到）。位置本身是易改的 UI 细节，若更希望放进设置页，把 `hostThemeFollow` 的渲染位置挪过去即可。
4. 宿主缺失语义（警告色、风险等级、严重度、状态徽标、图标族、滚动条）是否推动宿主纳入 `--ui-*` 契约，还是在皮肤内保持默认语言。
5. 令牌化后的圆角与密度是否允许 `default` 与 `teams` 使用不同取值，还是所有皮肤共用 OpsCopilot 自己的密度，仅换颜色。该结论决定第 9 步是否实施。
6. 是否引入像素级视觉回归工具（如 Playwright）纳入 CI。不引入时由第 9 节的令牌级断言加人工截图承担，代价是交互触发的面（拖拽、浮窗、溢出菜单）无法自动回归。
7. **插件包以哪个为准（已定）**：`main` 已包含 `plugins/teams-opscopilot`（`fe1f3f6` 之后），它随 main 演进，桌面与插件共用一份令牌层；`OpsCopilot-teams-plugin` / `OpsCopilot-shell-integration` / `OpsCopilot-workbench-architecture` 三个工作树此后只作历史参考。第 7、8 步的"插件 `ui.tsx` 不写死 `host.dataset.theme`"落在 `plugins/teams-opscopilot/src/ui.tsx`。
8. **宿主 revision（已解决）**：宿主仓库 `D:\dev\workspace-ai\icode-teams` 已把同事暂存的原生插件传输层提交为 `fed9cd9`（hostApi 6 的浏览器终端与文件通道），再并入 `origin/main`（`c9d99e9`，领先 26 个提交，含 `fda781f` 的深浅色主题）为 `1cfd32e`；10 个冲突文件按语义逐个合并，细节见该仓库的合并提交说明。宿主的 `--ui-*` 契约在这 26 个提交里**没有变化**（46 个名字全在），所以本仓的契约快照无需更新。宿主明暗的运行方式：`<head>` 内联脚本按 `localStorage['ui.theme']` 优先、否则 `prefers-color-scheme` 初始化 `<html data-theme>`，侧边栏 Switch 切换并写回 localStorage（只在浏览器本地，不落服务端）。该分支的推送与 PR 由宿主仓库负责人决定。

   另外宿主侧还有两处修复（都与主题无关，但影响插件在宿主里的呈现）：① `native-relay.ts` 的 `Promise.resolve(lease.close()).catch(() => {})` 被宿主的兜底门禁判为未登记的静默兜底，按仓内约定登记 `terminal-relay-lease-close` 并补了回归用例；② 合并后插件页塌成 225px——宿主在 `<main>` 内多了一层内容包裹 div，`.native-workspace` 的 `height:100%` 落在 auto 高度的父层上，补 `.native-main>div{height:100%}` 并把整高链条写进 `ui.test.ts` 的契约断言。第 7 步的"12 个面"复核里，这类"比例被压扁"的缺陷截图最容易漏掉，所以本仓的结论只对已量的几何负责。
9. 键盘焦点的自动化判据：jsdom 不算层叠，要自动化"Tab 后焦点可见"只有两条路——引入真浏览器测试基准（即第 6 条），或在门禁里加静态规则（例如 `outline: none` 必须与该文件内的 `:focus-visible` 覆写成对出现）。当前只有规则存在性断言。
10. **主题改动如何整合到插件（2026-09-13 实测，相关结论已修正一次）**：本仓库有 5 个工作树、4 条线：
    | 工作树 | 分支 | 相对 main | 内容 |
    | --- | --- | --- | --- |
    | `OpsCopilot` | `main` | — | 本次主题工作（未提交）+ 其他产品提交 |
    | `OpsCopilot-teams-plugin` | `feat/teams-plugin` | **1 领先 / 42 落后** | 单个提交 `8a26721`（121 文件，+8091/−2702）：Teams 插件包 + 把产品 UI 从桌面挪进壳的 `product/*` 层 |
    | `OpsCopilot-workbench-architecture` | `codex/workbench-architecture` | **118 领先 / 49 落后** | 另一条插件线（DSH 宿主，`plugins/dsh-opscopilot-shell`），且**没有**拿到 `15646dd`（shared-shell 恢复） |
    | `OpsCopilot-shell-integration` | `integration/shared-shell` | 0 领先 / 43 落后 | 已被 main 完全包含 → 陈旧指针 |
    | `.codex/worktrees/a6e9/OpsCopilot` | 游离 HEAD | — | 会话临时工作树 |

    共同祖先是 `7d16780`（quick command context menu safety），不是 `917d443`。
    - **单分支已是既定意图**：插件提交自带 `.github/workflows/shared-shell.yml`，触发条件是 `push: branches: [main]`，并在同一个提交上跑 `frontend-shell`（typecheck/test/build）+ `frontend`（test/build）+ `plugins/teams-opscopilot`（typecheck/test/integration/package）+ `go test/vet/build ./cmd/shellsidecar`。也就是说目标态是 main 同时装下共享壳、桌面与插件。该工作流目前只在插件分支上，收敛时要一并进 main（它同时也承接了第 10 步"门禁进 CI"的落点，本方案自己的 `lint:style` 应加进去）。
    - **合并面实测（`git merge-tree` 预演，非两棵树对比）**：把 `8a26721` 合到 main 上是 **7 个冲突文件**——`app.go`、`frontend-shell/src/ui/session/SessionManager.tsx`、`frontend/src/components/SettingsModal/SettingsModal.tsx`、`internal/shellsidecar/configs.go`、`pkg/config/store.go`、`pkg/sshclient/client.go`，以及一个结构性冲突 `pkg/sessionmanager/manager.go`（main 删除、插件修改）。`FlexLayoutAdapter.tsx`、`CommandGrid.tsx`、`QuickCommandPanel.tsx`、`AIConfigCard.tsx`、`frontend/src/App.tsx`、`go.mod` 等均自动合并成功。
    - **修正**：先前记的"插件提交有意删掉 12 个桌面专属 shell 文件"是错的两点对比读法。实测这 12 个文件（`ImportParts`、`ErrorBoundary`、`QuickCommandImportDialog`、`ConnectionPropertiesModal`、`XshellImportDialog`、`SessionTreeView/SessionContextMenu/treeModel/useSessionTree`）在分叉点 `917d443` 上**都不存在**，是 main 之后的 42 个提交新增的；插件提交一个文件都没删。
    - 唯一的结构性冲突是"同一个问题两套并行解法"：main 把会话持久化搬到了 `pkg/session` + `pkg/filetxn`/`internal/atomicfile`（`7bd3ad3` 删除了 `pkg/sessionmanager`），而插件提交是在旧的 `pkg/sessionmanager/manager.go` 上加原子写、baseline、`PreserveCredentials`。这需要按语义重新落到 main 的新位置，不是文本合并。
    - 需要拍板的是**方向**（不是技术细节）：插件提交把产品 UI 从 `frontend/` 挪进共享壳的 `product/*`（`Surface.tsx`、`ProductSidebar/ProductChrome/BottomBar`、`ProductShellSettingsPage`、`productSettingsStyles.ts` 等 17 个新文件），同时给桌面瘦身（`App.tsx` −369、`SettingsModal.tsx` −969、`Sidebar.tsx` −167）。桌面是否就此变成"薄宿主 + 共享壳"，决定了下面两件事：壳是否要保留 `ConnectionPropertiesModal`/`XshellImportDialog` 这些导出（桌面当前从包里导入它们），以及**遗留项"桌面旧设置页 253 处令牌化"是否应当直接取消**——那个文件在这个方向上正被 `Product*` 设置页取代。
    - 主题工作自身的落点不受影响：令牌层三件套（`shell-theme.css`、`settingsStyles.ts`、`appearance.ts`）**不在**上面 7 个冲突文件里，收敛后可直接落地；插件的 `productSettingsStyles.ts` 本来就 `import ... from './settingsStyles'`，会自动吃到令牌。唯一耦合仍是"`settingsStyles.ts` 与 `shell-theme.css` 必须同一次过去"（当前插件里 `--radius-sm` 读出为空即为证）。

    **收敛结果（2026-09-14）**：已合到 `main`（`fe1f3f6`，11 个冲突文件），插件包从此刻起随 main 一起演进——同事原来的担心（"在普通 main 上改皮肤，永远进不了插件包"）已消除。落地时按语义重新做了三件事：① 插件提交的共享存储语义（原子写、`filetxn`、`PreserveCredentials`、基线合并）移植到 main 的新位置 `pkg/connectionstore`（`pkg/sessionmanager` 已删除）；② `internal/shellsidecar` 的 RPC 对齐插件侧的口径（方法短名 `shell.configs.update/delete/rename`、list 信封键 `sessions`、输入字段 `host_key`/`root_password`）；③ 插件的 `app.tsx` 适配 main 的树形会话 API，同时保留 main 刚加出来的会话列表特性（导入入口等）。

    **挂载时又发现并解决的两个前置问题**：插件原先把 `hostApi` 声明为 `'3'`，其 `entry.ts` 要求宿主传 `config.host = {protocol, bundleId, version, artifactDirectory, dataDirectory}` —— 那是宿主 `demo/src/v2` 原型分支的契约，真实宿主对 `hostApi` 4/5/6 传的是 `config.dataDir`，所以插件无法挂载（宿主 400，错误由我们自己的守卫抛出，形如 `OpsCopilot requires the versioned Native host context`）。同事在工作树里留了未提交的迁移（`hostApi: '6'`、`uiApi: '1'`、`config.dataDir`、artifact 改为 base64 文件表），已一并整合；期间用"故意在守卫里回显收到的 config"做过一次探针，确认宿主对 `hostApi:'3'` 传的 `config` 是 `undefined`。另外宿主的旧版本号内容不可变（`retained bundle version cannot change its contents`），插件版本因此从 `0.1.0` 升到 `0.1.1`。
