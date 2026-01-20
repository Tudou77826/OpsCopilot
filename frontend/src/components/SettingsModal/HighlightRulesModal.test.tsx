import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/react';
import HighlightRulesModal from './HighlightRulesModal';
import { HighlightRule } from '../Terminal/highlightTypes';

describe('HighlightRulesModal', () => {
    it('adds rule and disables risky pattern', () => {
        const onChange = vi.fn();
        const onClose = vi.fn();
        render(<HighlightRulesModal isOpen={true} rules={[]} onChange={onChange} onClose={onClose} />);

        fireEvent.click(screen.getByText('+ 添加规则'));
        const inputs = screen.getAllByPlaceholderText('#RRGGBB');
        expect(inputs.length).toBeGreaterThan(0);

        const regexInput = screen.getByPlaceholderText(/例如：\(\?i\)\\\\b\(error\|fail\|fatal\)\\\\b/) as HTMLInputElement;
        fireEvent.change(regexInput, { target: { value: '(.+)+' } });

        const enable = screen.getByText('禁用').closest('label')!.querySelector('input') as HTMLInputElement;
        fireEvent.click(enable);
        expect(enable.checked).toBe(false);

        fireEvent.click(screen.getByText('保存'));
        expect(onChange).toHaveBeenCalledTimes(1);
        const saved = onChange.mock.calls[0][0] as HighlightRule[];
        expect(saved.length).toBe(1);
        expect(saved[0].is_enabled).toBe(false);
    });
});
