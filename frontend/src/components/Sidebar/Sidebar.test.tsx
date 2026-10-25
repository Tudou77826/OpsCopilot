import { render, screen, fireEvent } from '@testing-library/react';
import Sidebar from './Sidebar';
import { wailsCompanionHost } from '../../shell-adapter/wailsCompanionHost';
import { wailsSocialHost } from '../../shell-adapter/wailsSocialHost';
import { ToastProvider } from '../Toast/Toast';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';

describe('Sidebar Component', () => {
    // Mock scrollIntoView
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
        window.matchMedia = vi.fn(() => ({matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()})) as any;
        // @ts-ignore
        window.go = {
            main: {
                App: {
                    GetSavedSessions: async () => [],
                    DeleteSavedSession: async () => '',
                    RenameSavedSession: async () => '',
                    UpdateSavedSession: async () => '',
                    CreateSavedFolder: async () => '',
                }
            }
        };
    });

    it.each(['troubleshoot', 'chat'] as const)('collapses the closed %s panel without retaining its minimum width', (activeTab) => {
        const { container } = render(
            <ToastProvider>
                <Sidebar
                    companionHost={wailsCompanionHost}
                    socialHost={wailsSocialHost}
                    companionEnabled={false}
                    onManageCompanion={() => {}}
                    isOpen={false}
                    activeTab={activeTab}
                    onToggle={() => { }}
                    onConnect={() => { }}
                    activeTerminalId={null}
                    terminals={[]}
                />
            </ToastProvider>
        );
        expect(container.firstChild).not.toBeNull();
        expect(container.firstChild).toHaveStyle({
            width: '0px',
            minWidth: '0',
            boxShadow: 'none',
            overflow: 'hidden',
        });
    });

    it('renders TroubleshootingPanel when activeTab is troubleshoot', () => {
        render(
            <ToastProvider>
                <Sidebar
                    companionHost={wailsCompanionHost}
                    socialHost={wailsSocialHost}
                    companionEnabled={false}
                    onManageCompanion={() => {}}
                    isOpen={true}
                    activeTab="troubleshoot"
                    onToggle={() => { }}
                    onConnect={() => { }}
                    activeTerminalId={null}
                    terminals={[]}
                />
            </ToastProvider>
        );
        expect(screen.getByText('从现象开始定位')).toBeInTheDocument();
    });

    it('renders AIChatPanel when activeTab is chat', () => {
        render(
            <ToastProvider>
                <Sidebar
                    companionHost={wailsCompanionHost}
                    socialHost={wailsSocialHost}
                    companionEnabled={false}
                    onManageCompanion={() => {}}
                    isOpen={true}
                    activeTab="chat"
                    onToggle={() => { }}
                    onConnect={() => { }}
                    activeTerminalId={null}
                    terminals={[]}
                />
            </ToastProvider>
        );
        expect(screen.queryByRole('heading',{name:'AI 助手'})).toBeNull();
        expect(screen.getByRole('toolbar',{name:'对话操作'})).toBeVisible();
        expect(screen.queryByText('勿打扰')).toBeNull();
        expect(screen.queryByLabelText('伙伴展示')).toBeNull();
        expect(screen.queryByText('开启养成')).toBeNull();
        expect(screen.getByPlaceholderText('输入问题…')).toBeVisible();
    });
});
