import { AppearanceConfig, Skin, Theme } from './appearanceTypes';

/** 默认主题：暗色（与历史版本一致，老用户配置无感） */
export const DEFAULT_THEME: Theme = 'dark';

/** localStorage 键名 —— 与 index.html 内联防 FOUC 脚本保持一致 */
export const THEME_STORAGE_KEY = 'opscopilot-theme';

/** 默认皮肤：OpsCopilot 自带视觉语言 */
export const DEFAULT_SKIN: Skin = 'default';

/**
 * 皮肤键与主题键**分开**：宿主用什么皮肤、用户偏好明暗，是两件独立的事。
 * 合成一个键会让"换宿主皮肤"顺带丢掉用户的明暗选择。
 */
export const SKIN_STORAGE_KEY = 'opscopilot-skin';

/** 归一化主题值：仅接受 dark/light，其余回退默认 dark。对应后端 NormalizeAppearanceConfig。 */
export const normalizeTheme = (value?: string | null): Theme => {
    const v = (value ?? '').trim().toLowerCase();
    return v === 'light' || v === 'dark' ? v : DEFAULT_THEME;
};

/** 归一化皮肤值：仅接受 default/teams，其余回退默认 default。 */
export const normalizeSkin = (value?: string | null): Skin => {
    const v = (value ?? '').trim().toLowerCase();
    return v === 'teams' || v === 'default' ? v : DEFAULT_SKIN;
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

/**
 * 当前生效主题：以 <html data-theme> 为准。
 * 供无法拿到 theme 属性的组件（如终端配色数据、高亮规则默认值）取值，
 * 避免它们各自回退到 DEFAULT_THEME，造成「界面亮色、终端暗色」这类错位。
 */
export const currentTheme = (): Theme => {
    if (typeof document === 'undefined') return DEFAULT_THEME;
    return normalizeTheme(document.documentElement.dataset.theme);
};

/** 持久化皮肤到 localStorage（独立键，不动主题键） */
export const persistSkin = (skin: Skin) => {
    try {
        window.localStorage.setItem(SKIN_STORAGE_KEY, normalizeSkin(skin));
    } catch {
        // 忽略隐私模式 / 配额等异常
    }
};

/** 读取 localStorage 中上次皮肤 */
export const readPersistedSkin = (): Skin => {
    try {
        return normalizeSkin(window.localStorage.getItem(SKIN_STORAGE_KEY));
    } catch {
        return DEFAULT_SKIN;
    }
};

/**
 * 当前生效皮肤：以 data-skin 为准。
 * 与 data-theme 写在同一元素上：桌面壳是 <html>，插件是 shadow host——
 * 皮肤块经插件构建改写为 :host([data-skin="teams"])，属性必须落在同一个宿主元素上才匹配得到。
 */
export const currentSkin = (): Skin => {
    if (typeof document === 'undefined') return DEFAULT_SKIN;
    return normalizeSkin(document.documentElement.dataset.skin);
};

/** 应用皮肤。root 供插件传入 shadow host；桌面壳用默认的 <html>。是否持久化由调用方决定。 */
export const applySkin = (skin: Skin, root?: HTMLElement): Skin => {
    const normalized = normalizeSkin(skin);
    const target = root ?? (typeof document === 'undefined' ? undefined : document.documentElement);
    if (target) target.dataset.skin = normalized;
    return normalized;
};

/**
 * 主题是否跟随宿主。第三个前端键，与主题键、皮肤键都分开：
 * 跟着宿主时展示的是宿主的明暗，用户手动切过之后才展示他自己的选择，
 * 所以"是否跟随"与"选了什么"必须能分别记住，否则恢复跟随时会丢掉用户原来的偏好。
 */
export type ThemeFollow = 'host' | 'manual';

export const THEME_FOLLOW_STORAGE_KEY = 'opscopilot-theme-follow';

/** 默认跟随宿主：宿主已经表达了明暗，插件没有理由再加一次选择。 */
export const DEFAULT_THEME_FOLLOW: ThemeFollow = 'host';

/** 归一化跟随值：仅接受 host/manual，其余回退默认 host。 */
export const normalizeThemeFollow = (value?: string | null): ThemeFollow => {
    const v = (value ?? '').trim().toLowerCase();
    return v === 'manual' || v === 'host' ? v : DEFAULT_THEME_FOLLOW;
};

/** 持久化是否跟随宿主（独立键，不动主题键与皮肤键） */
export const persistThemeFollow = (value: ThemeFollow) => {
    try {
        window.localStorage.setItem(THEME_FOLLOW_STORAGE_KEY, normalizeThemeFollow(value));
    } catch {
        // 忽略隐私模式 / 配额等异常
    }
};

/** 读取是否跟随宿主 */
export const readPersistedThemeFollow = (): ThemeFollow => {
    try {
        return normalizeThemeFollow(window.localStorage.getItem(THEME_FOLLOW_STORAGE_KEY));
    } catch {
        return DEFAULT_THEME_FOLLOW;
    }
};

/**
 * 宿主当前的明暗：读外层文档根元素的 data-theme，**没有就返回 undefined**。
 * 返回 undefined 而不是回退到 DEFAULT_THEME 是关键——宿主没有明暗（旧构建、非宿主环境）
 * 与"宿主说是暗色"是两件事，后者才允许改写插件自己的取值。
 */
export const hostTheme = (): Theme | undefined => {
    if (typeof document === 'undefined') return undefined;
    const raw = (document.documentElement.dataset.theme ?? '').trim().toLowerCase();
    return raw === 'dark' || raw === 'light' ? raw : undefined;
};

/**
 * 监听宿主明暗的运行期变化。只在宿主真的改了 data-theme 时回调，
 * 返回的函数用于解除监听（React effect 清理）。
 */
export const observeHostTheme = (listener: (theme: Theme) => void): (() => void) => {
    if (typeof document === 'undefined' || typeof MutationObserver === 'undefined') return () => {};
    const observer = new MutationObserver(() => {
        const next = hostTheme();
        if (next) listener(next);
    });
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    return () => observer.disconnect();
};
