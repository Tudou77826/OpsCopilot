import React from 'react';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { afterEach, describe, it, expect, vi } from 'vitest';
import SkinChoiceCard, { type SkinChoice } from './SkinChoiceCard';
import { ProductShellSettingsPage } from './ProductShellSettingsPage';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';
import { availableSkins } from '../appearance';

afterEach(cleanup);

const options: SkinChoice['options'] = [
    { id: 'default', label: 'OpsCopilot 原版', hint: '自己的视觉语言' },
    { id: 'windows', label: 'Windows 风格', hint: 'Fluent 取向' },
    { id: 'teams', label: 'iCode Teams', hint: '取自宿主契约' },
];
const skinChoice = (value: SkinChoice['value'], onChange = vi.fn()): SkinChoice => ({ value, options, onChange });

describe('皮肤选择卡', () => {
    it('每个皮肤一个选项，选中态跟随当前值', () => {
        render(<SkinChoiceCard skin={skinChoice('windows')} mode="dark" />);
        const radios = screen.getAllByRole('radio');
        expect(radios.map(r => r.textContent)).toEqual(['OpsCopilot 原版', 'Windows 风格', 'iCode Teams']);
        expect(radios[1].getAttribute('aria-checked')).toBe('true');
        expect(radios[0].getAttribute('aria-checked')).toBe('false');
    });

    it('换一个皮肤会把新值交给宿主，且不动主题', () => {
        const onChange = vi.fn();
        render(<SkinChoiceCard skin={skinChoice('default', onChange)} mode="dark" />);
        fireEvent.click(screen.getByRole('radio', { name: /Windows 风格/ }));
        expect(onChange).toHaveBeenCalledWith('windows');
        expect(onChange).toHaveBeenCalledTimes(1);
    });

    it('提示语带上各选项的取舍说明', () => {
        render(<SkinChoiceCard skin={skinChoice('teams')} mode="light" />);
        expect(screen.getByRole('radio', { name: /iCode Teams/ }).getAttribute('title')).toBe('取自宿主契约');
    });

    it('切模式会让预览重算（mode 参与依赖）', () => {
        const { rerender } = render(<SkinChoiceCard skin={skinChoice('windows')} mode="dark" />);
        rerender(<SkinChoiceCard skin={skinChoice('windows')} mode="light" />);
        // 色值来自真实 CSS，jsdom 不跑层叠，因此这里只断言预览容器没被拆掉；
        // 颜色本身在真实宿主里读计算值核对（见 docs/theme-design.md 第 9 步）。
        expect(screen.getAllByRole('radio').length).toBe(3);
    });
});

describe('皮肤入口的出现条件', () => {
    it('没有宿主令牌时只列自带皮肤，不列宿主映射型皮肤', () => {
        const html = document.documentElement;
        const saved = html.getAttribute('style');
        try {
            html.removeAttribute('style');
            expect(availableSkins().map(s => s.id)).toEqual(['default', 'windows']);
            html.style.setProperty('--ui-color-bg', '#fafafa');
            expect(availableSkins().map(s => s.id), '有宿主令牌时才多出 iCode Teams').toEqual(['default', 'windows', 'teams']);
        } finally {
            if (saved === null) html.removeAttribute('style');
            else html.setAttribute('style', saved);
        }
    });

    const page = (skin?: SkinChoice) => (
        <ProductShellSettingsPage
            activeTab="appearance"
            config={{ terminal: normalizeTerminalConfig(), highlight_rules: [], command_query_shortcut: 'Ctrl+K' }}
            setConfig={vi.fn()}
            theme="dark"
            onThemeChange={vi.fn()}
            highlightIssues={[]}
            skin={skin}
        />
    );

    it('注入皮肤时外观页出现皮肤卡', () => {
        render(page(skinChoice('default')));
        expect(screen.getByText('皮肤')).toBeTruthy();
        expect(screen.getByText('主题')).toBeTruthy();
    });

    it('不注入时（桌面壳不传）没有皮肤卡', () => {
        render(page());
        expect(screen.queryByText('皮肤')).toBeNull();
        expect(screen.getByText('主题')).toBeTruthy();
    });
});
