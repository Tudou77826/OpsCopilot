import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import AIChatPanel from './AIChatPanel';

// Mock Wails runtime calls
const mockAskAI = vi.fn();

window.go = {
    main: {
        App: {
            AskCompanionAI: mockAskAI,
        }
    }
} as any;

describe('AIChatPanel', () => {
    it('uses the injected assistant runtime and releases its status subscription', async () => {
        const release = vi.fn();
        const runtime = {ask:vi.fn(async () => '适配器的回答'),subscribeStatus:vi.fn(() => release)};
        render(<AIChatPanel runtime={runtime}/>);
        fireEvent.change(screen.getByPlaceholderText(/输入问题/i),{target:{value:'通过伙伴提问'}});
        fireEvent.click(screen.getByTitle('发送'));
        await screen.findByText('适配器的回答');
        expect(runtime.ask).toHaveBeenCalledWith('通过伙伴提问');
        expect(mockAskAI).not.toHaveBeenCalled();
        await waitFor(() => expect(release).toHaveBeenCalledOnce());
    });
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
    });

    beforeEach(() => {
        mockAskAI.mockReset();
        window.go = { main: { App: { AskCompanionAI: mockAskAI } } } as any;
    });

    it('renders empty chat state correctly', () => {
        render(<AIChatPanel />);
        expect(screen.getByPlaceholderText(/输入问题/i)).toBeInTheDocument();
        expect(screen.queryByTestId('message-item')).not.toBeInTheDocument();
        expect(screen.queryByText('COMPANION ASSISTANT')).not.toBeInTheDocument();
        expect(screen.queryByText('和伙伴一起探索运维知识')).not.toBeInTheDocument();
    });

    it('sends message and displays user input', async () => {
        mockAskAI.mockResolvedValue('Hello User');
        
        render(<AIChatPanel />);
        const input = screen.getByPlaceholderText(/输入问题/i);
        fireEvent.change(input, { target: { value: 'Hi AI' } });
        fireEvent.keyDown(input, { key: 'Enter' });

        // Check user message
        expect(await screen.findByText('Hi AI')).toBeInTheDocument();
        expect(input).toHaveValue('');
    });

    it('displays AI response correctly', async () => {
        mockAskAI.mockResolvedValue('This is AI response');
        
        render(<AIChatPanel />);
        const input = screen.getByPlaceholderText(/输入问题/i);
        fireEvent.change(input, { target: { value: 'Question' } });
        fireEvent.click(screen.getByTitle('发送'));

        // Check AI response
        expect(await screen.findByText('This is AI response')).toBeInTheDocument();
    });

    it('clears chat when clicking New Chat button', async () => {
        mockAskAI.mockResolvedValue('Response');
        
        render(<AIChatPanel />);
        
        // Add a message first
        const input = screen.getByPlaceholderText(/输入问题/i);
        fireEvent.change(input, { target: { value: 'Question' } });
        fireEvent.click(screen.getByTitle('发送'));
        
        await screen.findByText('Question');

        // Click New Chat
        fireEvent.click(screen.getByText('+ 新建对话'));

        // Should be empty
        expect(screen.queryByText('Question')).not.toBeInTheDocument();
    });
});
