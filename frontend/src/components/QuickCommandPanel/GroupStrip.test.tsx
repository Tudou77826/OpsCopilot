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

    it('renders all group cards in the drum', () => {
        render(<GroupStrip {...defaultProps} />);
        expect(screen.getByTestId('group-strip')).toBeInTheDocument();
        // 滚筒把所有分组都渲染在 DOM 中，远近只是透明度/透视差异
        expect(screen.getByTestId('group-item-default')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-nginx')).toBeInTheDocument();
        expect(screen.getByTestId('group-item-docker')).toBeInTheDocument();
    });

    it('renders front card with group name', () => {
        render(<GroupStrip {...defaultProps} />);
        const active = screen.getByTestId('group-item-default');
        expect(active).toHaveTextContent('default');
    });

    it('clicks next card to switch group', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} onSelectGroup={onSelect} />);
        fireEvent.click(screen.getByTestId('group-item-nginx'));
        expect(onSelect).toHaveBeenCalledWith('nginx');
    });

    it('does not switch when clicking the front card', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} onSelectGroup={onSelect} />);
        fireEvent.click(screen.getByTestId('group-item-default'));
        expect(onSelect).not.toHaveBeenCalled();
    });

    it('switches group by wheel (deltaY accumulates past threshold)', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} onSelectGroup={onSelect} />);
        fireEvent.wheel(screen.getByTestId('group-strip'), { deltaY: 120 });
        expect(onSelect).toHaveBeenCalledWith('nginx');
    });

    it('switches to previous group by wheel up', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} selectedGroup="nginx" onSelectGroup={onSelect} />);
        fireEvent.wheel(screen.getByTestId('group-strip'), { deltaY: -120 });
        expect(onSelect).toHaveBeenCalledWith('default');
    });

    it('does not switch by wheel beyond the last group', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} selectedGroup="docker" onSelectGroup={onSelect} />);
        fireEvent.wheel(screen.getByTestId('group-strip'), { deltaY: 120 });
        expect(onSelect).not.toHaveBeenCalled();
    });

    it('reversing wheel direction switches immediately without offsetting old accumulation', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} selectedGroup="nginx" onSelectGroup={onSelect} />);
        const strip = screen.getByTestId('group-strip');
        // 向下滚了 40（未达阈值，留在累计器），紧接着反向 -60：
        // 应立即切到上一组，而不是先抵消旧的 +40
        fireEvent.wheel(strip, { deltaY: 40 });
        expect(onSelect).not.toHaveBeenCalled();
        fireEvent.wheel(strip, { deltaY: -60 });
        expect(onSelect).toHaveBeenCalledWith('default');
    });

    it('wheeling past the boundary does not block the reverse direction afterwards', () => {
        const onSelect = vi.fn();
        render(<GroupStrip {...defaultProps} selectedGroup="docker" onSelectGroup={onSelect} />);
        const strip = screen.getByTestId('group-strip');
        // 在最后一组继续向下滚（累计被边界清零），随后向上滚应立即生效
        fireEvent.wheel(strip, { deltaY: 120 });
        fireEvent.wheel(strip, { deltaY: 120 });
        expect(onSelect).not.toHaveBeenCalled();
        fireEvent.wheel(strip, { deltaY: -120 });
        expect(onSelect).toHaveBeenCalledWith('nginx');
    });

    it('calls onAddGroup when add button is clicked', () => {
        const onAdd = vi.fn();
        render(<GroupStrip {...defaultProps} onAddGroup={onAdd} />);
        fireEvent.click(screen.getByTestId('group-add-btn'));
        expect(onAdd).toHaveBeenCalled();
    });
});
