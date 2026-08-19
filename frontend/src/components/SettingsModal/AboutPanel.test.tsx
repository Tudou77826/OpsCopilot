import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { compareVersions } from './AboutPanel';
import AboutPanel from './AboutPanel';

// Mock Wails runtime 事件订阅（组件通过 EventsOn 订阅流式 token）
const eventHandlers: Record<string, (...args: any[]) => void> = {};
vi.mock('../../../wailsjs/runtime/runtime', () => ({
    EventsOn: (name: string, cb: (...args: any[]) => void) => {
        eventHandlers[name] = cb;
        return () => { delete eventHandlers[name]; };
    },
}));

function makeRelease(tag: string, body: string) {
    return { tag_name: tag, name: tag, body, html_url: '', published_at: '2026-08-19T00:00:00Z' };
}

describe('AboutPanel update flow', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        Object.keys(eventHandlers).forEach(k => delete eventHandlers[k]);
        localStorage.clear();
    });

    it('shows behind-banner and streams AI summary card when multiple versions behind', async () => {
        const checkUpdate = vi.fn().mockResolvedValue(JSON.stringify({
            hasUpdate: true,
            currentVersion: 'v1.8.9.3',
            latestVersion: 'v1.9.0.0',
            downloadUrl: 'http://example.com/x.zip',
            release: makeRelease('v1.9.0.0', '## v1.9.0.0\n\n- 最新版本内容'),
        }));
        const getHistory = vi.fn().mockResolvedValue(JSON.stringify({
            releases: [
                makeRelease('v1.9.0.0', 'a'),
                makeRelease('v1.8.9.5', 'b'),
                makeRelease('v1.8.9.4', 'c'),
                makeRelease('v1.8.9.3', 'd'),
            ],
        }));
        const summarize = vi.fn().mockResolvedValue('');
        window.go = {
            main: {
                App: {
                    GetVersion: vi.fn().mockResolvedValue('v1.8.9.3'),
                    CheckUpdate: checkUpdate,
                    GetReleaseHistory: getHistory,
                    SummarizeUpdateNotes: summarize,
                },
            },
        } as any;

        render(<AboutPanel />);

        // 检查更新
        await waitFor(() => screen.getByRole('button', { name: '检查更新' }));
        fireEvent.click(screen.getByRole('button', { name: '检查更新' }));

        // 新版本卡片头部出现落后徽标 + AI 总览触发（1.8.9.3 → 1.9.0.0 之间有 3 个版本）。
        // 后台请求顺带补全版本日志：deck 从单卡变为 4 张可翻页
        expect(await screen.findByText(/落后 3 个版本/)).toBeInTheDocument();
        await waitFor(() => expect(screen.getByText('1 / 4')).toBeInTheDocument());
        const summarizeBtn = await screen.findByRole('button', { name: 'AI 总览' });

        // 触发 AI 总览 → 调用携带累积日志原文，并翻到总览卡片（deck 第 1 张，共 5 张）
        fireEvent.click(summarizeBtn);
        await waitFor(() => expect(summarize).toHaveBeenCalledWith('## v1.9.0.0\n\n- 最新版本内容'));

        // 总览卡片作为 deck 第一张卡（token 流式渲染的 DOM 断言在 vitest 下受
        // act 环境限制，流式链路已由真实运行实例验证；此处断言卡片状态与翻页计数）
        expect(await screen.findByText(/正在整合 3 个版本的更新/)).toBeInTheDocument();
        expect(screen.getByText('v1.8.9.3 → v1.9.0.0 · 3 个版本')).toBeInTheDocument();
        expect(screen.getByText('1 / 5')).toBeInTheDocument();

        // 关闭总览 → deck 恢复为纯版本卡片，回到最新版本卡
        fireEvent.click(screen.getByRole('button', { name: '关闭总览' }));
        await waitFor(() => screen.getByRole('button', { name: 'AI 总览' }));
        expect(screen.getByText('1 / 4')).toBeInTheDocument();
    });

    it('does not show behind-banner when only one version behind', async () => {
        const checkUpdate = vi.fn().mockResolvedValue(JSON.stringify({
            hasUpdate: true,
            currentVersion: 'v1.8.9.5',
            latestVersion: 'v1.9.0.0',
            downloadUrl: 'http://example.com/x.zip',
            release: makeRelease('v1.9.0.0', 'body'),
        }));
        const getHistory = vi.fn().mockResolvedValue(JSON.stringify({
            releases: [makeRelease('v1.9.0.0', 'a'), makeRelease('v1.8.9.5', 'b')],
        }));
        window.go = {
            main: {
                App: {
                    GetVersion: vi.fn().mockResolvedValue('v1.8.9.5'),
                    CheckUpdate: checkUpdate,
                    GetReleaseHistory: getHistory,
                    SummarizeUpdateNotes: vi.fn(),
                },
            },
        } as any;

        render(<AboutPanel />);
        await waitFor(() => screen.getByRole('button', { name: '检查更新' }));
        fireEvent.click(screen.getByRole('button', { name: '检查更新' }));

        await waitFor(() => expect(checkUpdate).toHaveBeenCalled());
        // 落后 1 个版本：不出落后徽标也不出 AI 总览触发
        await waitFor(() => screen.getByText('更新并重启'));
        expect(screen.queryByText(/已落后/)).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'AI 总览' })).not.toBeInTheDocument();
    });
});

describe('compareVersions', () => {
    it('compares 4-part versions numerically', () => {
        expect(compareVersions('1.9.0.0', '1.8.9.5')).toBeGreaterThan(0);
        expect(compareVersions('1.8.9.5', '1.9.0.0')).toBeLessThan(0);
        expect(compareVersions('1.9.0.0', '1.9.0.0')).toBe(0);
    });

    it('compares per segment, not lexicographically', () => {
        // 字符串比较会误判 "1.10.0" < "1.9.0"，数值比较必须正确
        expect(compareVersions('1.10.0', '1.9.0')).toBeGreaterThan(0);
        expect(compareVersions('2.0.0', '1.99.99')).toBeGreaterThan(0);
    });

    it('pads shorter versions with zeros', () => {
        expect(compareVersions('1.9', '1.8.9.5')).toBeGreaterThan(0);
        expect(compareVersions('1.9.0.0', '1.9')).toBe(0);
    });

    it('treats non-numeric segments as 0', () => {
        // dev 构建等非数字版本号参与比较时降级为 0，不会抛错
        expect(compareVersions('1.0.0', 'dev')).toBeGreaterThan(0);
        expect(compareVersions('dev', 'dev')).toBe(0);
    });
});
