import { render, screen } from '@testing-library/react';
import TerminalComponent from './Terminal';
import { Terminal } from 'xterm';
import { vi, describe, it, expect } from 'vitest';

const { TerminalMock } = vi.hoisted(() => {
    return {
        TerminalMock: vi.fn()
    }
});

// Mock xterm
vi.mock('xterm', () => {
  return {
    Terminal: class {
        constructor() {
            TerminalMock();
            return {
                open: vi.fn(),
                write: vi.fn(),
                dispose: vi.fn(),
                onData: vi.fn(),
                attachCustomKeyEventHandler: vi.fn(),
                getSelection: vi.fn(() => ''),
                clearSelection: vi.fn(),
                paste: vi.fn(),
                loadAddon: vi.fn(),
                onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                focus: vi.fn(),
                cols: 80,
                rows: 24,
                buffer: {
                    active: {
                        viewportY: 0,
                        baseY: 0,
                        cursorY: 0,
                        length: 0,
                        getLine: vi.fn(() => ({ translateToString: vi.fn(() => '') }))
                    }
                },
                registerMarker: vi.fn(() => ({ dispose: vi.fn() })),
                registerDecoration: vi.fn(() => ({ dispose: vi.fn() })),
            }
        }
    },
  };
});

// Mock xterm-addon-fit
vi.mock('xterm-addon-fit', () => {
  return {
    FitAddon: class {
        fit = vi.fn();
    }
  }
})

vi.mock('xterm-addon-search', () => {
    return {
        SearchAddon: class {
            findNext = vi.fn();
            findPrevious = vi.fn();
        }
    };
});

describe('TerminalComponent', () => {
  it('renders terminal container', () => {
    render(<TerminalComponent id="test-term" />);
    const element = screen.getByTestId('terminal-container-test-term');
    expect(element).toBeInTheDocument();
  });

  it('initializes xterm on mount', () => {
    render(<TerminalComponent id="test-term" />);
    expect(TerminalMock).toHaveBeenCalled();
  });
});
