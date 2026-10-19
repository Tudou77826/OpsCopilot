import { useRef } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { ExplorerSplitter } from './ExplorerSplitter';

const key = 'opscopilot.database.explorerWidth';
afterEach(() => {
  cleanup();
  localStorage.removeItem(key);
  vi.restoreAllMocks();
});
function Fixture() {
  const container = useRef<HTMLElement>(null);
  return <section ref={container} data-testid="workspace"><ExplorerSplitter container={container} /></section>;
}
function measure(width: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: 0, y: 0, top: 0, left: 0, right: width, bottom: 700,
    width, height: 700, toJSON: () => ({}),
  });
}
it('remembers adjusted width across remounts and restores the default on double click', () => {
  measure(960);
  const view = render(<Fixture />);
  const splitter = screen.getByRole('separator', {name: '调整连接列表宽度'});
  fireEvent.keyDown(splitter, {key: 'ArrowRight'});
  expect(splitter).toHaveAttribute('aria-valuenow', '286');
  expect(screen.getByTestId('workspace').style.getPropertyValue('--oc-db-explorer-width')).toBe('286px');
  expect(localStorage.getItem(key)).toBe('286');
  view.unmount();
  render(<Fixture />);
  const restored = screen.getByRole('separator');
  expect(restored).toHaveAttribute('aria-valuenow', '286');
  fireEvent.doubleClick(restored);
  expect(restored).toHaveAttribute('aria-valuenow', '270');
  expect(localStorage.getItem(key)).toBeNull();
});
it('keeps the query area available when the workspace gets narrower', () => {
  measure(960);
  render(<Fixture />);
  const splitter = screen.getByRole('separator');
  fireEvent.keyDown(splitter, {key: 'End'});
  expect(splitter).toHaveAttribute('aria-valuenow', '520');
  vi.mocked(HTMLElement.prototype.getBoundingClientRect).mockReturnValue({width: 500} as DOMRect);
  fireEvent(window, new Event('resize'));
  expect(splitter).toHaveAttribute('aria-valuenow', '220');
  fireEvent.keyDown(splitter, {key: 'Home'});
  expect(splitter).toHaveAttribute('aria-valuenow', '160');
});

it('captures dragging, ignores other pointers and stops resizing after cancellation', () => {
  measure(960);
  render(<Fixture />);
  const splitter = screen.getByRole('separator');
  splitter.setPointerCapture = vi.fn();
  function pointer(type: string, x: number, id = 1) {
    const event = new MouseEvent(type, {bubbles: true, button: 0, clientX: x});
    Object.defineProperty(event, 'pointerId', {value: id});
    fireEvent(splitter, event);
  }
  pointer('pointerdown', 270);
  expect(splitter.setPointerCapture).toHaveBeenCalledWith(1);
  pointer('pointermove', 450, 2);
  expect(splitter).toHaveAttribute('aria-valuenow', '270');
  pointer('pointermove', 350);
  expect(splitter).toHaveAttribute('aria-valuenow', '350');
  pointer('pointercancel', 350, 2);
  pointer('pointermove', 370);
  expect(splitter).toHaveAttribute('aria-valuenow', '370');
  pointer('pointercancel', 370);
  expect(localStorage.getItem(key)).toBe('370');
  pointer('pointermove', 450);
  expect(splitter).toHaveAttribute('aria-valuenow', '370');
});
