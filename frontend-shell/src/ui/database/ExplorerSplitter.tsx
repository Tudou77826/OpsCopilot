import { useEffect, useRef, useState, type RefObject } from 'react';

const storageKey = 'opscopilot.database.explorerWidth';

function savedWidth(): number | null {
  try {
    const value = Number(localStorage.getItem(storageKey));
    return Number.isFinite(value) && value >= 120 && value <= 520 ? value : null;
  } catch {
    return null;
  }
}

export function ExplorerSplitter({ container }: { container: RefObject<HTMLElement> }) {
  const [preferred, setPreferred] = useState(savedWidth);
  const [available, setAvailable] = useState(960);
  const drag = useRef<{ id: number; x: number; width: number } | null>(null);
  const maximum = Math.min(520, Math.max(120, available - 280));
  const minimum = Math.min(160, maximum);
  const defaultWidth = available < 600 ? 180 : available < 900 ? 225 : 270;
  const clamp = (value: number) => Math.round(Math.min(maximum, Math.max(minimum, value)));
  const width = clamp(preferred ?? defaultWidth);
  const latestWidth = useRef(width);
  latestWidth.current = width;

  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const measure = () => {
      const next = element.getBoundingClientRect().width;
      if (next > 0) setAvailable(next);
    };
    measure();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    observer?.observe(element);
    window.addEventListener('resize', measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', measure);
      element.style.removeProperty('--oc-db-explorer-width');
    };
  }, [container]);

  useEffect(() => {
    container.current?.style.setProperty('--oc-db-explorer-width', `${width}px`);
  }, [container, width]);

  function persist(value: number | null) {
    try {
      if (value === null) localStorage.removeItem(storageKey);
      else localStorage.setItem(storageKey, String(value));
    } catch { /* Resizing remains available when storage is disabled. */ }
  }
  function finish() {
    if (!drag.current) return;
    drag.current = null;
    persist(latestWidth.current);
  }
  function reset() {
    drag.current = null;
    setPreferred(null);
    persist(null);
  }

  return (
    <div
      className="oc-db-explorer-splitter"
      role="separator"
      aria-label="调整连接列表宽度"
      aria-orientation="vertical"
      aria-valuemin={minimum}
      aria-valuemax={maximum}
      aria-valuenow={width}
      aria-valuetext={`${width} 像素`}
      tabIndex={0}
      title="拖动调整宽度，双击恢复默认"
      onDoubleClick={reset}
      onPointerDown={(e) => {
        if (e.button !== 0 || drag.current) return;
        e.preventDefault();
        e.currentTarget.focus();
        drag.current = { id: e.pointerId, x: e.clientX, width };
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        const active = drag.current;
        if (!active || active.id !== e.pointerId) return;
        const value = clamp(active.width + e.clientX - active.x);
        latestWidth.current = value;
        setPreferred(value);
      }}
      onPointerUp={(e) => {
        if (drag.current?.id === e.pointerId) finish();
      }}
      onPointerCancel={(e) => {
        if (drag.current?.id === e.pointerId) finish();
      }}
      onLostPointerCapture={(e) => {
        if (drag.current?.id === e.pointerId) finish();
      }}
      onKeyDown={(e) => {
        const next = e.key === 'ArrowLeft' ? width - 16
          : e.key === 'ArrowRight' ? width + 16
          : e.key === 'Home' ? minimum
          : e.key === 'End' ? maximum : undefined;
        if (next === undefined) return;
        e.preventDefault();
        e.stopPropagation();
        const value = clamp(next);
        setPreferred(value);
        persist(value);
      }}
    />
  );
}
