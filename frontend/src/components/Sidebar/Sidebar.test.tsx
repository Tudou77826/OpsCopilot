import { render, screen, fireEvent } from '@testing-library/react';
import Sidebar from './Sidebar';
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
                }
            }
        };
    });

    it('renders closed state correctly', () => {
        const { container } = render(
            <Sidebar
                isOpen={false}
                activeTab="troubleshoot"
                onToggle={() => { }}
                onConnect={() => { }}
                activeTerminalId={null}
                terminals={[]}
            />
        );
        // When closed, it should return a hidden div, not null
        expect(container.firstChild).not.toBeNull();
        expect(container.firstChild).toHaveStyle({ width: '0px' });
    });

    it('renders TroubleshootingPanel when activeTab is troubleshoot', () => {
        render(
            <Sidebar
                isOpen={true}
                activeTab="troubleshoot"
                onToggle={() => { }}
                onConnect={() => { }}
                activeTerminalId={null}
                terminals={[]}
            />
        );
        // Assuming TroubleshootingPanel renders specific text
        expect(screen.getByText('开始排查')).toBeInTheDocument();
    });

    it('renders AIChatPanel when activeTab is chat', () => {
        render(
            <Sidebar
                isOpen={true}
                activeTab="chat"
                onToggle={() => { }}
                onConnect={() => { }}
                activeTerminalId={null}
                terminals={[]}
            />
        );
        expect(screen.getByText('AI 问答')).toBeInTheDocument();
    });
});
