import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DatabaseExecutionHistory } from './DatabaseExecutionHistory';
import type { DatabaseBlock } from '../../workspace/database';

describe('SQL 结果定位', () => {
    it('按稳定记录 ID 展开目标 SQL，重复定位仍有效，不执行 SQL', () => {
        const onSelect = vi.fn();
        const blocks: DatabaseBlock[] = ['one','two'].map(id => ({id,sql:`select '${id}'`,purpose:id,status:'completed',at:'2026-10-24T00:00:00Z',result:{sets:[],durationMs:0,error:'',cancelled:false,disconnected:false}}));
        const view = render(<DatabaseExecutionHistory blocks={blocks} onSelect={onSelect} revealId="two" revealNonce="first"/>);
        expect(screen.getByText("select 'two'")).toBeVisible();
        expect(screen.getByText("select 'one'")).not.toBeVisible();
        expect(document.activeElement).toHaveAttribute('data-block-id','two');
        view.rerender(<DatabaseExecutionHistory blocks={blocks} onSelect={onSelect} revealId="one" revealNonce="second"/>);
        expect(document.activeElement).toHaveAttribute('data-block-id','one');
        expect(onSelect).not.toHaveBeenCalled();
    });
});
