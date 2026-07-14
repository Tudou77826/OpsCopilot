import React from 'react';
import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import BottomBar, { BOTTOM_BAR_TIP_INTERVAL_MS, BOTTOM_BAR_TIPS } from './BottomBar';

describe('BottomBar', () => {
    afterEach(() => {
        vi.useRealTimers();
        vi.unstubAllGlobals();
    });

    it('shows the first terminal tip', () => {
        render(<BottomBar />);
        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[0]);
    });

    it('rotates tips every five seconds', () => {
        vi.useFakeTimers();
        vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
            matches: true,
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
        }));

        render(<BottomBar />);
        act(() => {
            vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS);
        });

        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[1]);
    });
});
