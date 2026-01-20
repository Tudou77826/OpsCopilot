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
});
