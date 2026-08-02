// 外观模式（亮色/暗色主题）类型定义 —— 与后端 config.AppearanceConfig 手工镜像。
// 加字段时需同步后端 pkg/config/store.go 的 AppearanceConfig + NormalizeAppearanceConfig。

/** 主题模式：dark（默认）或 light */
export type Theme = 'dark' | 'light';

/** 外观配置，对应后端 AppearanceConfig */
export interface AppearanceConfig {
    theme: Theme;
}
