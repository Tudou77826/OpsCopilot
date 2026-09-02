import { AppearanceConfig, Theme } from './appearanceTypes';

/** 默认主题：暗色（与历史版本一致，老用户配置无感） */
export const DEFAULT_THEME: Theme = 'dark';

/** localStorage 键名 —— 与 index.html 内联防 FOUC 脚本保持一致 */
export const THEME_STORAGE_KEY = 'opscopilot-theme';

/** 归一化主题值：仅接受 dark/light，其余回退默认 dark。对应后端 NormalizeAppearanceConfig。 */
export const normalizeTheme = (value?: string | null): Theme => {
    const v = (value ?? '').trim().toLowerCase();
    return v === 'light' || v === 'dark' ? v : DEFAULT_THEME;
};

/** 归一化整个 AppearanceConfig */
export const normalizeAppearanceConfig = (config?: Partial<AppearanceConfig> | null): AppearanceConfig => ({
    theme: normalizeTheme(config?.theme),
});

/** 持久化主题到 localStorage（供首屏内联脚本下次同步读取，防 FOUC） */
export const persistTheme = (theme: Theme) => {
    try {
        window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
        // 忽略隐私模式 / 配额等异常
    }
};

/** 读取 localStorage 中上次主题（首屏内联脚本也读同一个键） */
export const readPersistedTheme = (): Theme => {
    try {
        return normalizeTheme(window.localStorage.getItem(THEME_STORAGE_KEY));
    } catch {
        return DEFAULT_THEME;
    }
};
