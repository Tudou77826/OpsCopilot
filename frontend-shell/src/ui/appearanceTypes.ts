// 外观模式（亮色/暗色主题）类型定义 —— 与后端 config.AppearanceConfig 手工镜像。
// 加字段时需同步后端 pkg/config/store.go 的 AppearanceConfig + NormalizeAppearanceConfig。

/** 主题模式：dark（默认）或 light */
export type Theme = 'dark' | 'light';

/**
 * 皮肤轴：与明暗模式正交。default = OpsCopilot 自带的视觉语言；
 * teams = 把颜色、圆角、密度、字体映射到 iCode Teams 宿主的 --ui-* 契约（见 docs/theme-design.md 第 6 节）。
 *
 * 皮肤只存前端（localStorage 独立键）：由宿主决定用哪个皮肤，桌面壳始终 default，
 * 因此不进后端 AppearanceConfig，也不需要配套的 Go 侧往返。
 */
export type Skin = 'default' | 'teams';

/** 外观配置，对应后端 AppearanceConfig */
export interface AppearanceConfig {
    theme: Theme;
}
