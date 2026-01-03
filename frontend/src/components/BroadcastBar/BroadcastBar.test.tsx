import { render, screen, fireEvent } from '@testing-library/react';
import BroadcastBar from './BroadcastBar';
import { vi, describe, it, expect } from 'vitest';

describe('BroadcastBar', () => {
    it('renders input and button', () => {
        render(<BroadcastBar onBroadcast={vi.fn()} />);
        expect(screen.getByPlaceholderText(/Type command/i)).toBeInTheDocument();
        expect(screen.getByText('Send')).toBeInTheDocument();
    });

    it('calls onBroadcast with command when submitted', () => {
        const onBroadcast = vi.fn();
        render(<BroadcastBar onBroadcast={onBroadcast} />);

        const input = screen.getByPlaceholderText(/Type command/i);
        fireEvent.change(input, { target: { value: 'echo test' } });
        
        fireEvent.click(screen.getByText('Send'));

        expect(onBroadcast).toHaveBeenCalledWith('echo test');
    });

    it('clears input after submission', () => {
        const onBroadcast = vi.fn();
        render(<BroadcastBar onBroadcast={onBroadcast} />);

        const input = screen.getByPlaceholderText(/Type command/i) as HTMLInputElement;
        fireEvent.change(input, { target: { value: 'ls -la' } });
        fireEvent.click(screen.getByText('Send'));

        expect(input.value).toBe('');
    });

    it('does not submit empty command', () => {
        const onBroadcast = vi.fn();
        render(<BroadcastBar onBroadcast={onBroadcast} />);

        fireEvent.click(screen.getByText('Send'));
        expect(onBroadcast).not.toHaveBeenCalled();
    });
});
