import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/react';
import HighlightRulesModal from './HighlightRulesModal';
import { HighlightRule } from '../Terminal/highlightTypes';

describe('HighlightRulesModal', () => {
    it('adds rule with risky pattern that cannot be enabled', () => {
        const onChange = vi.fn();
        const onClose = vi.fn();
        render(<HighlightRulesModal isOpen={true} rules={[]} onChange={onChange} onClose={onClose} />);

        // Add a new rule (button text is "+ 新建规则")
        fireEvent.click(screen.getByText('+ 新建规则'));

        // Verify the rule was added — name shows "新规则", status "已禁用"
        expect(screen.getByText('新规则')).toBeInTheDocument();
        expect(screen.getByText('已禁用')).toBeInTheDocument();

        // Enter a risky regex pattern (.+)+ — assessed as 'high' risk
        const regexInput = screen.getByPlaceholderText(/例如：\(\?i\)\\\\b\(error\|fail\)\\\\b/) as HTMLInputElement;
        fireEvent.change(regexInput, { target: { value: '(.+)+' } });

        // Risk badge appears (appears twice: badge + detail text)
        const riskLabels = screen.getAllByText('高风险');
        expect(riskLabels.length).toBeGreaterThanOrEqual(1);

        // The toggle checkbox is disabled because user hasn't acknowledged risk
        const checkboxes = screen.getAllByRole('checkbox');
        const toggleCheckbox = checkboxes[0] as HTMLInputElement;
        expect(toggleCheckbox.disabled).toBe(true);
        expect(toggleCheckbox.checked).toBe(false);

        // Save — button text is "保存更改"
        fireEvent.click(screen.getByText('保存更改'));
        expect(onChange).toHaveBeenCalledTimes(1);

        const saved = onChange.mock.calls[0][0] as HighlightRule[];
        expect(saved.length).toBe(1);
        expect(saved[0].is_enabled).toBe(false);
        expect(saved[0].pattern).toBe('(.+)+');
    });
});
