import React from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import BottomBar, { BOTTOM_BAR_TIP_INTERVAL_MS, BOTTOM_BAR_TIPS } from './BottomBar';

const bridge = vi.hoisted(() => ({ announcements: vi.fn(), link: vi.fn() }));
vi.mock('../../../wailsjs/go/main/App', () => ({
    GetServiceAnnouncements: bridge.announcements,
    OpenServiceAnnouncement: bridge.link,
}));

describe('BottomBar', () => {
    beforeEach(() => {
        bridge.announcements.mockResolvedValue([]);
        bridge.link.mockResolvedValue(undefined);
    });
    afterEach(() => {
        vi.useRealTimers();
        vi.unstubAllGlobals();
    });

    it('shows the first terminal tip', async () => {
        render(<BottomBar onOpenServiceSettings={vi.fn()} />);
        await act(async () => { await Promise.resolve(); await Promise.resolve(); });
        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[0]);
    });

    it('rotates tips every five seconds', async () => {
        vi.useFakeTimers();
        vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
            matches: true,
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
        }));

        render(<BottomBar onOpenServiceSettings={vi.fn()} />);
        await act(async () => { await Promise.resolve(); await Promise.resolve(); });
        act(() => {
            vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS);
        });

        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[1]);
    });

    it('shows announcements in the same carousel as tips and opens their links', async () => {
        const now = Date.now();
        bridge.announcements.mockResolvedValue([
            { id: 'a', text: '版本公告', url: 'https://ops.internal/downloads', startsAt: new Date(now - 1000).toISOString(), endsAt: new Date(now + 60000).toISOString() },
            { id: 'b', text: '维护公告', url: '', startsAt: new Date(now - 1000).toISOString(), endsAt: new Date(now + 60000).toISOString() },
            { id: 'expired', text: '过期公告', url: '', startsAt: new Date(now - 60000).toISOString(), endsAt: new Date(now - 1000).toISOString() },
        ]);
        vi.useFakeTimers();
        vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
        render(<BottomBar onOpenServiceSettings={vi.fn()} />);
        await act(async () => { await Promise.resolve(); await Promise.resolve(); });

        expect(screen.getByText('公告')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '版本公告' })).toBeInTheDocument();
        expect(screen.queryByText('过期公告')).toBeNull();
        expect(screen.queryByTestId('bottom-bar-tip')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: '版本公告' }));
        expect(bridge.link).toHaveBeenCalledWith('https://ops.internal/downloads');

        fireEvent.mouseEnter(screen.getByTestId('bottom-bar-carousel'));
        act(() => { vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS); });
        expect(screen.getByRole('button', { name: '版本公告' })).toBeInTheDocument();
        fireEvent.mouseLeave(screen.getByTestId('bottom-bar-carousel'));
        act(() => { vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS); });
        expect(screen.queryByText('公告')).toBeNull();
        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[0]);
        act(() => { vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS); });
        expect(screen.getByText('公告')).toBeInTheDocument();
        expect(screen.getByText('维护公告')).toBeInTheDocument();
        expect(screen.queryByTestId('bottom-bar-tip')).toBeNull();
        act(() => { vi.advanceTimersByTime(BOTTOM_BAR_TIP_INTERVAL_MS); });
        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[1]);
    });
});
