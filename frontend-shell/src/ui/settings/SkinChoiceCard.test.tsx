import React from 'react';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { afterEach, describe, it, expect, vi } from 'vitest';
import SkinChoiceCard, { type SkinChoice } from './SkinChoiceCard';
import { ProductShellSettingsPage } from './ProductShellSettingsPage';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';

afterEach(cleanup);

const options: SkinChoice['options'] = [
    { id: 'teams', label: '跟随 iCode Teams', hint: '取自宿主契约' },
    { id: 'default', label: 'OpsCopilot 默认', hint: '用自己的视觉语言' },
];
const skinChoice = (value: SkinChoice['value'], onChange = vi.fn()): SkinChoice => ({ value, options, onChange });

describe('皮肤选择卡', () => {
    it('每个皮肤一个选项，选中态跟随当前值', () => {
        render(<SkinChoiceCard skin={skinChoice('teams')} />);
        const radios = screen.getAllByRole('radio');
        expect(radios.map(r => r.textContent)).toEqual(['跟随 iCode Teams', 'OpsCopilot 默认']);
        expect(radios[0].getAttribute('aria-checked')).toBe('true');
        expect(radios[1].getAttribute('aria-checked')).toBe('false');
    });

    it('换一个皮肤会把新值交给宿主，且不动主题', () => {
        const onChange = vi.fn();
        render(<SkinChoiceCard skin={skinChoice('teams', onChange)} />);
        fireEvent.click(screen.getByRole('radio', { name: 'OpsCopilot 默认' }));
        expect(onChange).toHaveBeenCalledWith('default');
        expect(onChange).toHaveBeenCalledTimes(1);
    });

    it('提示语带上各选项的取舍说明', () => {
        render(<SkinChoiceCard skin={skinChoice('default')} />);
        expect(screen.getByRole('radio', { name: '跟随 iCode Teams' }).getAttribute('title')).toBe('取自宿主契约');
        expect(screen.getByRole('radio', { name: 'OpsCopilot 默认' }).getAttribute('title')).toBe('用自己的视觉语言');
    });
});

describe('皮肤入口的出现条件', () => {
    const page = (skin?: SkinChoice) => (
        <ProductShellSettingsPage
            activeTab="appearance"
            config={{ terminal: normalizeTerminalConfig(), highlight_rules: [], command_query_shortcut: 'Ctrl+K' }}
            setConfig={vi.fn()}
            theme="light"
            onThemeChange={vi.fn()}
            highlightIssues={[]}
            skin={skin}
        />
    );

    it('注入皮肤时外观页出现皮肤卡', () => {
        render(page(skinChoice('teams')));
        expect(screen.getByText('皮肤')).toBeTruthy();
        expect(screen.getByText('主题')).toBeTruthy();
    });

    it('不注入时（桌面壳）没有皮肤卡——那里换皮肤是空操作', () => {
        render(page());
        expect(screen.queryByText('皮肤')).toBeNull();
        expect(screen.getByText('主题')).toBeTruthy();
    });
});
