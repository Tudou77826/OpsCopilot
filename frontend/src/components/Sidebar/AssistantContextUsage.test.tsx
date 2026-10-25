import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AssistantContextUsage } from './AssistantContextUsage';

afterEach(() => { cleanup(); vi.useRealTimers(); });

describe('AssistantContextUsage', () => {
    it('distinguishes unknown usage from a known empty window', async () => {
        const host = {contextUsage: vi.fn().mockResolvedValue({known:false,used:0,size:0})};
        const {rerender} = render(<AssistantContextUsage runtime={host} busy={false}/>);
        expect(screen.getByRole('img', {name:'上下文用量未知'})).toHaveAttribute('title', expect.stringContaining('不代表 0%'));
        host.contextUsage.mockResolvedValue({known:true,used:0,size:1000});
        rerender(<AssistantContextUsage runtime={host} busy={false} revision="next"/>);
        await waitFor(() => expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow','0'));
    });
    it('shows real counts and preserves over-capacity values while clamping the ring', async () => {
        const host = {contextUsage:vi.fn().mockResolvedValue({known:true,used:1200,size:1000})};
        render(<AssistantContextUsage runtime={host} busy={false}/>);
        const meter = await screen.findByRole('meter', {name:'上下文占用 120%'});
        expect(meter).toHaveAttribute('aria-valuenow','100');
        expect(meter).toHaveAttribute('title',expect.stringContaining('1,200 / 1,000 tokens'));
        expect(meter).toHaveClass('is-high');
        expect(meter.querySelector('.ai-context-fill')).toHaveAttribute('stroke-dasharray','100 100');
    });
    it('updates during a turn without overlapping reads and accepts lower usage after compaction', async () => {
        vi.useFakeTimers();
        let complete!: (value: {known:boolean;used:number;size:number}) => void;
        const host = {contextUsage:vi.fn().mockImplementationOnce(() => new Promise(resolve => {complete=resolve;})).mockResolvedValue({known:true,used:20,size:100})};
        render(<AssistantContextUsage runtime={host} busy/>);
        await act(async () => {vi.advanceTimersByTime(3000);});
        expect(host.contextUsage).toHaveBeenCalledTimes(1);
        await act(async () => {complete({known:true,used:80,size:100});});
        expect(screen.getByRole('meter')).toHaveClass('is-warning');
        await act(async () => {vi.advanceTimersByTime(1000);});
        expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow','20');
    });
    it('ignores a late response from an old conversation/model', async () => {
        let previous!: (value: {known:boolean;used:number;size:number}) => void;
        const host = {contextUsage:vi.fn().mockImplementationOnce(() => new Promise(resolve => {previous=resolve;})).mockResolvedValue({known:true,used:5,size:100})};
        const {rerender} = render(<AssistantContextUsage runtime={host} busy={false} revision="old"/>);
        rerender(<AssistantContextUsage runtime={host} busy={false} revision="new"/>);
        await screen.findByRole('meter',{name:'上下文占用 5%'});
        await act(async () => {previous({known:true,used:90,size:100});});
        expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow','5');
    });
    it('does not invent a percentage for malformed usage or a failed read', async () => {
        const host = {contextUsage:vi.fn().mockResolvedValue({known:true,used:10,size:0})};
        const {rerender} = render(<AssistantContextUsage runtime={host} busy={false}/>);
        await act(async () => {});
        expect(screen.queryByRole('meter')).not.toBeInTheDocument();
        host.contextUsage.mockRejectedValue(Error('offline'));
        rerender(<AssistantContextUsage runtime={host} busy={false} revision="failed"/>);
        expect(await screen.findByRole('img',{name:'上下文用量暂不可用'})).toBeInTheDocument();
    });
});
