import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll } from 'vitest';
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
});
