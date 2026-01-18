import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import MonitoringPanel from './MonitoringPanel';

function mockGo(app: Record<string, any>) {
    // @ts-ignore
    window.go = { main: { App: app } };
}

describe('MonitoringPanel', () => {
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
    });

    beforeEach(() => {
        vi.useRealTimers();
        mockGo({});
    });

    it('shows empty state when no sessions', () => {
        render(<MonitoringPanel activeTerminalId={null} terminals={[]} />);
        expect(screen.getByText(/请先建立 SSH 连接/i)).toBeInTheDocument();
    });

    it('lists java processes and supports filtering', async () => {
        const list = JSON.stringify([
            { pid: 100, user: 'root', etime: '00:01', cmd: 'java -jar a.jar' },
            { pid: 200, user: 'app', etime: '00:02', cmd: 'java -jar b.jar' },
        ]);
        mockGo({
            GetJavaMonitorSnapshot: vi.fn(),
            GetJavaThreadStateCounts: vi.fn(),
            GetJavaTopCPUThreads: vi.fn(),
            ListJavaProcesses: vi.fn().mockResolvedValue(list),
        });

        render(<MonitoringPanel activeTerminalId="s1" terminals={[{ id: 's1', title: 'server1' }]} />);

        await waitFor(() => {
            expect(screen.getByText(/PID 100/)).toBeInTheDocument();
            expect(screen.getByText(/PID 200/)).toBeInTheDocument();
        });

        const input = screen.getByPlaceholderText(/筛选 Java 进程/i);
        fireEvent.change(input, { target: { value: 'b.jar' } });
        expect(screen.queryByText(/PID 100/)).not.toBeInTheDocument();
        expect(screen.getByText(/PID 200/)).toBeInTheDocument();
    });

    it('auto refreshes snapshot and triggers thread-state auto sampling once on sustained high CPU', async () => {
        const list = JSON.stringify([{ pid: 100, user: 'root', etime: '00:01', cmd: 'java -jar a.jar' }]);

        const snapshots = [
            // initial click sample
            JSON.stringify({
                pid: 100,
                tools: { jcmd: false, jstat: false, jps: false },
                process: { pid: 100, cpu: '90.0', etime: '00:01', threads: 10, fd_count: 20, fd_limit: 1000, vm_rss_kb: 1024 * 200, cmd: 'java' },
                jvm: { vm_version: { command: '', output: '' }, heap_info: { command: '', output: '' }, gcutil_once: { command: '', output: '' } },
                host: { uptime: { command: '', output: '' }, meminfo: { command: '', output: '' } }
            }),
            JSON.stringify({
                pid: 100,
                tools: { jcmd: false, jstat: false, jps: false },
                process: { pid: 100, cpu: '90.0', etime: '00:01', threads: 10, fd_count: 20, fd_limit: 1000, vm_rss_kb: 1024 * 200, cmd: 'java' },
                jvm: { vm_version: { command: '', output: '' }, heap_info: { command: '', output: '' }, gcutil_once: { command: '', output: '' } },
                host: { uptime: { command: '', output: '' }, meminfo: { command: '', output: '' } }
            }),
            JSON.stringify({
                pid: 100,
                tools: { jcmd: false, jstat: false, jps: false },
                process: { pid: 100, cpu: '90.0', etime: '00:01', threads: 10, fd_count: 20, fd_limit: 1000, vm_rss_kb: 1024 * 200, cmd: 'java' },
                jvm: { vm_version: { command: '', output: '' }, heap_info: { command: '', output: '' }, gcutil_once: { command: '', output: '' } },
                host: { uptime: { command: '', output: '' }, meminfo: { command: '', output: '' } }
            }),
        ];
        const GetJavaMonitorSnapshot = vi.fn()
            .mockResolvedValueOnce(snapshots[0])
            .mockResolvedValueOnce(snapshots[1])
            .mockResolvedValueOnce(snapshots[2]);

        const GetJavaThreadStateCounts = vi.fn().mockResolvedValue(JSON.stringify({
            runnable: 1, blocked: 0, waiting: 0, timed_waiting: 0, new: 0, terminated: 0, unknown: 0
        }));

        mockGo({
            ListJavaProcesses: vi.fn().mockResolvedValue(list),
            GetJavaMonitorSnapshot,
            GetJavaThreadStateCounts,
            GetJavaTopCPUThreads: vi.fn(),
        });

        render(<MonitoringPanel activeTerminalId="s1" terminals={[{ id: 's1', title: 'server1' }]} />);

        await waitFor(() => expect(screen.getByText(/PID 100/)).toBeInTheDocument());

        vi.useFakeTimers();
        fireEvent.click(screen.getByText(/PID 100/));

        await Promise.resolve();
        await Promise.resolve();
        expect(GetJavaMonitorSnapshot).toHaveBeenCalledTimes(1);

        // default interval is 5s; advance two intervals => total 3 samples
        await vi.advanceTimersByTimeAsync(5_100);
        await Promise.resolve();
        await vi.advanceTimersByTimeAsync(5_100);
        await Promise.resolve();
        expect(GetJavaMonitorSnapshot).toHaveBeenCalledTimes(3);

        await Promise.resolve();
        expect(GetJavaThreadStateCounts).toHaveBeenCalledTimes(1);
        expect(screen.getByText(/已自动采样一次线程状态/i)).toBeInTheDocument();
    }, 15_000);
});

