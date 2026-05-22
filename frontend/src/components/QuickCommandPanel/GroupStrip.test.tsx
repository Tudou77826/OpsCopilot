import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';
import GroupStrip from './GroupStrip';

describe('GroupStrip', () => {
    const defaultProps = {
        groups: ['default', 'nginx', 'docker'],
        selectedGroup: 'default',
        onSelectGroup: vi.fn(),
        onAddGroup: vi.fn(),
    };

    it('renders 3 stacked cards', () => {
        render(<GroupStrip {...defaultProps} />);
        expect(screen.getByTestId('group-strip')).toBeInTheDocument();
        // Only current (default) and next (nginx) visible since no prev
        expect(screen.getByTestId('group-item-default')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-nginx')).toBeInTheDocument();
    });

    it('highlights active card', () => {
        render(<GroupStrip {...defaultProps} />);
        const active = screen.getByTestId('group-item-default');
        expect(active).toBeInTheDocument();
    });

    it('clicks next card to switch group', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} onSelectGroup={onSelect} />);
        fireEvent.click(screen.getByTestId('group-item-nginx'));
        expect(onSelect).toHaveBeenCalledWith('nginx');
    });

    it('shows 3 cards when middle group selected', () => {
        render(<GroupStrip {...defaultProps} selectedGroup="nginx" />);
        expect(screen.getByTestId('group-item-default')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-nginx')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-docker')).toBeInTheDocument();
    });

    it('calls onAddGroup when add button is clicked', () => {
        const onAdd = vi.fn();
        render(<GroupStrip {...defaultProps} onAddGroup={onAdd} />);
        fireEvent.click(screen.getByTestId('group-add-btn'));
        expect(onAdd).toHaveBeenCalled();
    });
});
