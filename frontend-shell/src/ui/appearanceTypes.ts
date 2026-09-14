// 外观模式（亮色/暗色主题）类型定义 —— 与后端 config.AppearanceConfig 手工镜像。
// 加字段时需同步后端 pkg/config/store.go 的 AppearanceConfig + NormalizeAppearanceConfig。

/** 主题模式：dark（默认）或 light */
export type Theme = 'dark' | 'light';

/**
 * 皮肤轴：与明暗模式正交，管"视觉语言从哪来"。
 *
 * - default：OpsCopilot 自带的视觉语言（暖白亮色 + 深色暗色）。
 * - windows：Windows 11 / Fluent 取向的自带调色板。它没有宿主，取值是本皮肤自己的字面量，
 *   因此按模式各写一块（亮色一块、暗色一块，后者多一个属性、权重更高）。
 * - teams：宿主映射型皮肤，取值全部来自 iCode Teams 的 --ui-* 契约，一个块就覆盖两种模式
 *   （宿主自己按模式换那批变量）。
 *
 * 字体与密度**不属于任何皮肤**：终端字体/字号是终端应用的核心体验，由 TerminalConfig 独立拥有；
 * 见 docs/theme-design.md 第 9 步。皮肤只存前端（localStorage 独立键），不进后端 AppearanceConfig。
 */
export type Skin = 'default' | 'teams' | 'windows';

/** 皮肤的展示信息。hostOnly 的皮肤在没有宿主令牌的环境里换不出任何差别，入口应按此过滤。 */
export interface SkinInfo { id: Skin; label: string; hint: string; hostOnly?: boolean }

export const SKINS: SkinInfo[] = [
    { id: 'default', label: 'OpsCopilot 原版', hint: '深色底 + 暖白亮色，OpsCopilot 自己的视觉语言' },
    { id: 'windows', label: 'Windows 风格', hint: 'Windows 11 / Fluent 取向：中性灰表面、蓝色强调、更轻的投影' },
    { id: 'teams', label: 'iCode Teams', hint: '颜色取自宿主公开的样式契约，宿主换主题时自动跟随', hostOnly: true },
];

/** 外观配置，对应后端 AppearanceConfig */
export interface AppearanceConfig {
    theme: Theme;
}
