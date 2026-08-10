import { render, screen, fireEvent } from '@testing-library/react';
import Sidebar from './Sidebar';
import { ToastProvider } from '../Toast/Toast';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';

describe('Sidebar Component', () => {
    // Mock scrollIntoView
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
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

    it('renders closed state correctly', () => {
        const { container } = render(
            <ToastProvider>
                <Sidebar
                    isOpen={false}
                    activeTab="troubleshoot"
                    onToggle={() => { }}
                    onConnect={() => { }}
                    activeTerminalId={null}
                    terminals={[]}
                />
            </ToastProvider>
        );
        // When closed, it should return a hidden div, not null
        expect(container.firstChild).not.toBeNull();
        expect(container.firstChild).toHaveStyle({ width: '0px' });
    });

    it('renders TroubleshootingPanel when activeTab is troubleshoot', () => {
        render(
            <ToastProvider>
                <Sidebar
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
                    isOpen={true}
                    activeTab="chat"
                    onToggle={() => { }}
                    onConnect={() => { }}
                    activeTerminalId={null}
                    terminals={[]}
                />
            </ToastProvider>
        );
        expect(screen.getByText('AI 问答')).toBeInTheDocument();
    });
});
