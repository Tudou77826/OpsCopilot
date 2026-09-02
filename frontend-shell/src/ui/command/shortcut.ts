/**
 * 快捷键匹配工具（宿主无关）。
 * 从 Wails App.tsx 的内联实现提取；事件 → "Ctrl+K" 形式字符串，与设置项存储格式一致。
 */

export const isEditableTarget = (target: EventTarget | null): boolean => {
    const el = target as HTMLElement | null;
    if (!el) return false;
    if (el.classList?.contains('xterm-helper-textarea')) return false;
    if (el.closest?.('.xterm')) return false;
    const tag = el.tagName?.toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
    if ((el as unknown as HTMLElement).isContentEditable) return true;
    return false;
};

export const eventToShortcut = (e: KeyboardEvent): string => {
    const parts: string[] = [];
    if (e.ctrlKey) parts.push('Ctrl');
    if (e.altKey) parts.push('Alt');
    if (e.shiftKey) parts.push('Shift');
    if (e.metaKey) parts.push('Meta');

    if (e.key === 'Control' || e.key === 'Alt' || e.key === 'Shift' || e.key === 'Meta') return '';
    const mainKey = e.key.length === 1 ? e.key.toUpperCase() : e.key;
    parts.push(mainKey);
    return parts.join('+');
};

export const matchesShortcut = (e: KeyboardEvent, shortcut: string): boolean => {
    const normalized = (shortcut || '').trim();
    if (!normalized) return false;
    return eventToShortcut(e).toLowerCase() === normalized.toLowerCase();
};
