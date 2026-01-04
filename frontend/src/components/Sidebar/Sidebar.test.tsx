import { render, screen, fireEvent } from '@testing-library/react';
import Sidebar from './Sidebar';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';

describe('Sidebar Component', () => {
    // Mock scrollIntoView
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
    });

    it('renders closed state correctly', () => {
        render(<Sidebar isOpen={false} onToggle={() => {}} />);
        // When closed, it might just show a toggle button or be hidden depending on implementation.
        // Assuming it renders a toggle button at least.
        const toggleBtn = screen.getByRole('button', { name: /toggle sidebar/i });
        expect(toggleBtn).toBeInTheDocument();
    });

    it('renders open state with start button', () => {
        render(<Sidebar isOpen={true} onToggle={() => {}} />);
        expect(screen.getByText('AI 助手')).toBeInTheDocument();
        expect(screen.getByText('开始排查')).toBeInTheDocument();
    });

    it('switches to investigation mode when start is clicked', () => {
        const onStartMock = vi.fn();
        render(<Sidebar isOpen={true} onToggle={() => {}} onStart={onStartMock} />);
        
        const startBtn = screen.getByText('开始排查');
        fireEvent.click(startBtn);
        
        expect(onStartMock).toHaveBeenCalled();
        // Should show stop button and input area
        expect(screen.getByText('结束排查')).toBeInTheDocument();
        expect(screen.getByPlaceholderText(/输入问题/i)).toBeInTheDocument();
    });

    it('handles message input', () => {
        render(<Sidebar isOpen={true} onToggle={() => {}} />);
        
        // Enter investigation mode
        fireEvent.click(screen.getByText('开始排查'));
        
        const input = screen.getByPlaceholderText(/输入问题/i);
        fireEvent.change(input, { target: { value: 'Test query' } });
        
        const sendBtn = screen.getByText('发送');
        fireEvent.click(sendBtn);
        
        // Should verify message is added to list (assuming internal state or mock)
        // For now, we just check if input is cleared or message appears
        expect(screen.getByText('Test query')).toBeInTheDocument();
    });
});
