import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/react';
import SearchPanel from './SearchPanel';

describe('SearchPanel', () => {
    it('does not render when hidden', () => {
        const { container } = render(
            <SearchPanel
                visible={false}
                query=""
                onQueryChange={() => {}}
                onClose={() => {}}
                onNext={() => {}}
                onPrev={() => {}}
                caseSensitive={false}
                onCaseSensitiveChange={() => {}}
                regexMode={false}
                onRegexModeChange={() => {}}
            />
        );
        expect(container.firstChild).toBeNull();
    });

    it('navigates on Enter/Shift+Enter and supports buttons', () => {
        const onClose = vi.fn();
        const onNext = vi.fn();
        const onPrev = vi.fn();

        const { getByPlaceholderText } = render(
            <SearchPanel
                visible={true}
                query="err"
                onQueryChange={() => {}}
                onClose={onClose}
                onNext={onNext}
                onPrev={onPrev}
                caseSensitive={false}
                onCaseSensitiveChange={() => {}}
                regexMode={false}
                onRegexModeChange={() => {}}
                matchText="1"
            />
        );

        const input = getByPlaceholderText('搜索…');
        fireEvent.keyDown(input, { key: 'Enter' });
        expect(onNext).toHaveBeenCalledTimes(1);

        fireEvent.keyDown(input, { key: 'Enter', shiftKey: true });
        expect(onPrev).toHaveBeenCalledTimes(1);

        fireEvent.click(screen.getByText('▶'));
        expect(onNext).toHaveBeenCalledTimes(2);

        fireEvent.click(screen.getByText('◀'));
        expect(onPrev).toHaveBeenCalledTimes(2);

        fireEvent.keyDown(input, { key: 'Escape' });
        expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('resets position to bottom-left when resetKey changes', () => {
        const baseProps = {
            visible: true,
            query: '',
            onQueryChange: () => {},
            onClose: () => {},
            onNext: () => {},
            onPrev: () => {},
            caseSensitive: false,
            onCaseSensitiveChange: () => {},
            regexMode: false,
            onRegexModeChange: () => {},
        };

        const { container, rerender } = render(<SearchPanel {...baseProps} resetKey={0} />);
        const wrap = container.firstChild as HTMLElement;

        // 初始位置：左下角
        expect(wrap.style.left).toBe('12px');
        expect(wrap.style.bottom).toBe('12px');

        // 模拟拖拽：mouseDown 后 mouseMove 改变位置（deltaX=100, deltaY=100）
        fireEvent.mouseDown(wrap, { clientX: 100, clientY: 100 });
        fireEvent.mouseMove(window, { clientX: 200, clientY: 200 });
        fireEvent.mouseUp(window);
        expect(wrap.style.left).toBe('112px');

        // resetKey 变化 → 重置回左下角
        rerender(<SearchPanel {...baseProps} resetKey={1} />);
        expect(wrap.style.left).toBe('12px');
        expect(wrap.style.bottom).toBe('12px');
    });
});
