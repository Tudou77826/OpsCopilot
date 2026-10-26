import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react';

const STORAGE_KEY = 'opscopilot.companion.presentation.v1';
const DEFAULT_HEIGHT = 180;
const MIN_HEIGHT = 120;
const MAX_HEIGHT = 340;
type Preference = { expanded: boolean; height: number };

function readPreference(): Preference {
  try {
    const value = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null');
    return {
      expanded: value?.expanded === true,
      height: Number.isFinite(value?.height) ? Math.max(MIN_HEIGHT, Math.min(MAX_HEIGHT, value.height)) : DEFAULT_HEIGHT,
    };
  } catch { return { expanded: false, height: DEFAULT_HEIGHT }; }
}

/** Presentation preferences only: no pet state, growth or host calls. */
export function useCompanionPresentation(enabled: boolean, visible: boolean) {
  const root = useRef<HTMLElement>(null);
  const [preference, setPreference] = useState(readPreference);
  const [maximum, setMaximum] = useState(MAX_HEIGHT);
  const [previewHeight, setPreviewHeight] = useState<number>();
  const drag = useRef<{ pointer: number; y: number; height: number; element: HTMLElement }>();
  const minimum = Math.min(MIN_HEIGHT, maximum);
  const clamp = (height: number) => Math.round(Math.max(minimum, Math.min(maximum, height)));
  const height = clamp(previewHeight ?? preference.height);

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(preference)); } catch { /* Optional in private/restricted storage. */ }
  }, [preference]);

  useLayoutEffect(() => {
    const panel = root.current?.parentElement;
    if (!panel || !enabled || !visible) return;
    const update = () => {
      if (!panel.clientHeight) return; // Hidden panels must not erase the user's height.
      // Keep the composer and a useful portion of the conversation visible.
      // A narrow landscape should not grow into an empty portrait viewport.
      const aspectMaximum = panel.clientWidth > 0 ? Math.floor(panel.clientWidth * .7) : MAX_HEIGHT;
      setMaximum(Math.max(0, Math.min(MAX_HEIGHT, aspectMaximum, Math.floor(panel.clientHeight * .4), panel.clientHeight - 44 - 280)));
    };
    update();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(update);
    observer?.observe(panel);
    window.addEventListener('resize', update);
    return () => { observer?.disconnect(); window.removeEventListener('resize', update); };
  }, [enabled, visible]);

  function cancelDrag() {
    const current = drag.current;
    drag.current = undefined;
    setPreviewHeight(undefined);
    if (current?.element.hasPointerCapture?.(current.pointer)) current.element.releasePointerCapture(current.pointer);
  }
  useEffect(() => {
    if (!enabled || !visible || !preference.expanded) cancelDrag();
  }, [enabled, visible, preference.expanded]);
  useEffect(() => {
    window.addEventListener('blur', cancelDrag);
    return () => {
      window.removeEventListener('blur', cancelDrag);
      const current = drag.current;
      drag.current = undefined;
      if (current?.element.hasPointerCapture?.(current.pointer)) current.element.releasePointerCapture(current.pointer);
    };
  }, []);

  function onPointerDown(event: PointerEvent<HTMLElement>) {
    if (event.button !== 0 || !event.isPrimary) return;
    event.preventDefault();
    event.currentTarget.focus();
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { pointer: event.pointerId, y: event.clientY, height, element: event.currentTarget };
    setPreviewHeight(height);
  }
  function onPointerMove(event: PointerEvent<HTMLElement>) {
    const current = drag.current;
    if (!current || current.pointer !== event.pointerId) return;
    setPreviewHeight(clamp(current.height + event.clientY - current.y));
  }
  function onPointerUp(event: PointerEvent<HTMLElement>) {
    const current = drag.current;
    if (!current || current.pointer !== event.pointerId) return;
    const value = clamp(current.height + event.clientY - current.y);
    drag.current = undefined;
    setPreviewHeight(undefined);
    setPreference(previous => ({ ...previous, height: Math.max(MIN_HEIGHT, value) }));
    if (current.element.hasPointerCapture?.(current.pointer)) current.element.releasePointerCapture(current.pointer);
  }
  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    const step = event.shiftKey ? 40 : 10;
    const next = event.key === 'ArrowUp' ? height - step : event.key === 'ArrowDown' ? height + step :
      event.key === 'Home' ? minimum : event.key === 'End' ? maximum : undefined;
    if (next === undefined) return;
    event.preventDefault();
    setPreference(previous => ({ ...previous, height: Math.max(MIN_HEIGHT, clamp(next)) }));
  }
  return {
    root, height, minimum, maximum, expanded: preference.expanded, dragging: previewHeight !== undefined,
    toggle: () => setPreference(previous => ({ ...previous, expanded: !previous.expanded })),
    separator: {
      onPointerDown, onPointerMove, onPointerUp, onKeyDown,
      onPointerCancel: (event: PointerEvent<HTMLElement>) => { if (drag.current?.pointer === event.pointerId) cancelDrag(); },
      onLostPointerCapture: (event: PointerEvent<HTMLElement>) => { if (drag.current?.pointer === event.pointerId) cancelDrag(); },
      onDoubleClick: () => setPreference(previous => ({ ...previous, height: DEFAULT_HEIGHT })),
    },
  };
}
