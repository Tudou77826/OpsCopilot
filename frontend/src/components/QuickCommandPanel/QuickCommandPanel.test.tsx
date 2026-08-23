import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import QuickCommandPanel from './QuickCommandPanel';

describe('QuickCommandPanel', () => {
    beforeAll(() => {
        (window as any).go = {
            main: {
                App: {
                    LoadQuickCommands: vi.fn().mockResolvedValue([
                        { id: '1', name: 'List Files', content: 'ls -la', group: 'default' },
                        { id: '2', name: 'Check Disk', content: 'df -h', group: 'system' },
                    ]),
                    SaveQuickCommands: vi.fn(),
                }
            }
        };
    });

    it('renders panel when open', () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        expect(screen.getByTestId('quick-command-panel')).toBeInTheDocument();
        expect(screen.getByTestId('command-grid')).toBeInTheDocument();
    });

    it('renders collapsed when closed', () => {
        render(<QuickCommandPanel isOpen={false} onExecute={vi.fn()} />);
        const panel = screen.getByTestId('quick-command-panel');
        expect(panel).toBeInTheDocument();
        expect(screen.queryByText('List Files')).not.toBeInTheDocument();
    });

    it('executes command on click', async () => {
        const onExecute = vi.fn();
        render(<QuickCommandPanel isOpen={true} onExecute={onExecute} />);

        const cmd = await screen.findByText('List Files');
        fireEvent.click(cmd);
        expect(onExecute).toHaveBeenCalledWith('ls -la');
    });

    it('shows group strip with current group', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');
        expect(screen.getByTestId('group-strip')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-default')).toHaveTextContent('default');
    });

    it('opens add modal via add button', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');

        fireEvent.click(screen.getByTestId('command-add-btn'));
        expect(screen.getByTestId('command-edit-modal')).toBeInTheDocument();
        expect(screen.getByText('新建命令')).toBeInTheDocument();
    });

    it('filters commands by keyword within the current group', async () => {
        // default 分组下只有 "List Files"（ls -la）；"Check Disk" 属于 system 分组，不参与搜索（issue #56）
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');

        const input = screen.getByTestId('command-search').querySelector('input')!;

        // 命中 name：输入 list，仍显示 List Files
        fireEvent.change(input, { target: { value: 'list' } });
        expect(screen.getByText('List Files')).toBeInTheDocument();

        // 输入不匹配的关键字，当前分组命令消失
        fireEvent.change(input, { target: { value: 'nomatch' } });
        expect(screen.queryByText('List Files')).not.toBeInTheDocument();
    });
});

describe('QuickCommandPanel resizing', () => {
    beforeEach(() => {
        localStorage.removeItem('opscopilot-quickcmd-panel-height');
        localStorage.removeItem('opscopilot-quickcmd-strip-width');
    });

    it('resizes panel height by dragging the top handle and persists it', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');
        const handle = screen.getByTestId('quickcmd-height-handle');
        fireEvent.pointerDown(handle, { clientY: 500, pointerId: 1 });
        fireEvent.pointerMove(handle, { clientY: 400, pointerId: 1 }); // 上拖 100px
        fireEvent.pointerUp(handle, { clientY: 400, pointerId: 1 });
        const panel = screen.getByTestId('quick-command-panel');
        expect(panel).toHaveStyle({ height: '100px' });
        expect(localStorage.getItem('opscopilot-quickcmd-panel-height')).toBe('100');
    });

    it('resizes strip width by dragging the divider and persists it', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');
        const handle = screen.getByTestId('quickcmd-width-handle');
        fireEvent.pointerDown(handle, { clientX: 500, pointerId: 1 });
        fireEvent.pointerMove(handle, { clientX: 470, pointerId: 1 }); // 左拖 30px → 94px
        fireEvent.pointerUp(handle, { clientX: 470, pointerId: 1 });
        expect(screen.getByTestId('group-strip')).toHaveStyle({ width: '94px' });
        expect(localStorage.getItem('opscopilot-quickcmd-strip-width')).toBe('94');
    });

    it('clamps width drag to the allowed range', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');
        const handle = screen.getByTestId('quickcmd-width-handle');
        fireEvent.pointerDown(handle, { clientX: 500, pointerId: 1 });
        fireEvent.pointerMove(handle, { clientX: 900, pointerId: 1 }); // 右拖 400px，远超下限
        fireEvent.pointerUp(handle, { clientX: 900, pointerId: 1 });
        expect(screen.getByTestId('group-strip')).toHaveStyle({ width: '56px' });
    });
});

describe('new group end-to-end flow', () => {
    beforeEach(() => {
        localStorage.removeItem('opscopilot-quickcmd-panel-height');
        localStorage.removeItem('opscopilot-quickcmd-strip-width');
    });

    it('creates a group from the strip "+", lands in it, and shows the new command', async () => {
        render(<QuickCommandPanel isOpen={true} onExecute={vi.fn()} />);
        await screen.findByText('List Files');

        // 分组条 "+"：直接进入分组名输入模式，标题为"新建分组"
        fireEvent.click(screen.getByTestId('group-add-btn'));
        expect(screen.getByTestId('command-group-input')).toBeInTheDocument();
        expect(screen.getByText('新建分组')).toBeInTheDocument();

        // 不切换任何控件，直接填三个字段保存（修复前的"必须先切走再切回"已不需要）
        fireEvent.change(screen.getByTestId('command-group-input'), { target: { value: '数据库' } });
        fireEvent.change(screen.getByTestId('command-name-input'), { target: { value: '连接测试库' } });
        fireEvent.change(screen.getByTestId('command-content-textarea'), { target: { value: 'mysql -h db' } });
        fireEvent.click(screen.getByTestId('command-edit-save'));

        // 保存后自动落到新分组：命令立即可见，滚筒中出现新分组卡
        expect(await screen.findByText('连接测试库')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-数据库')).toBeInTheDocument();
    });
});
