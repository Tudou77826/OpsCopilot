import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { usePortalRoot } from '../Surface';

export interface DatabaseMenuItem {
  label: string;
  disabled?: boolean;
  danger?: boolean;
  separator?: boolean;
  action(): void;
}
export function DatabaseMenu({
  x,
  y,
  items,
  onClose,
  label = '数据库对象操作',
}: {
  x: number;
  y: number;
  items: DatabaseMenuItem[];
  onClose(): void;
  label?: string;
}) {
  const root = usePortalRoot();
  const ref = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ left: x, top: y });
  useLayoutEffect(() => {
    const rect = ref.current?.getBoundingClientRect();
    if (rect)
      setPosition({
        left: Math.max(4, Math.min(x, window.innerWidth - rect.width - 4)),
        top: Math.max(4, Math.min(y, window.innerHeight - rect.height - 4)),
      });
    ref.current
      ?.querySelector<HTMLButtonElement>('button:not(:disabled)')
      ?.focus();
  }, [x, y]);
  useEffect(() => {
    const outside = (e: Event) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    const close = () => onClose();
    window.addEventListener('pointerdown', outside);
    window.addEventListener('blur', close);
    window.addEventListener('resize', close);
    return () => {
      window.removeEventListener('pointerdown', outside);
      window.removeEventListener('blur', close);
      window.removeEventListener('resize', close);
    };
  }, [onClose]);
  return createPortal(
    <div
      ref={ref}
      className="oc-db-menu"
      role="menu"
      aria-label={label}
      style={position}
      onContextMenu={(e) => e.preventDefault()}
      onKeyDown={(e) => {
        e.stopPropagation();
        const buttons = Array.from(
          ref.current?.querySelectorAll<HTMLButtonElement>(
            'button:not(:disabled)',
          ) ?? [],
        );
        const index = buttons.indexOf(
          document.activeElement as HTMLButtonElement,
        );
        if (e.key === 'Escape' || e.key === 'Tab') {
          e.preventDefault();
          onClose();
          return;
        }
        const next =
          e.key === 'ArrowDown'
            ? (index + 1) % buttons.length
            : e.key === 'ArrowUp'
              ? (index - 1 + buttons.length) % buttons.length
              : e.key === 'Home'
                ? 0
                : e.key === 'End'
                  ? buttons.length - 1
                  : -1;
        if (next >= 0) {
          e.preventDefault();
          buttons[next]?.focus();
        }
      }}
    >
      {items.map((item) => (
        <div key={item.label}>
          {item.separator && (
            <div role="separator" className="oc-db-menu-divider" />
          )}
          <button
            role="menuitem"
            type="button"
            disabled={item.disabled}
            className={item.danger ? 'is-danger' : ''}
            onClick={() => {
              onClose();
              item.action();
            }}
          >
            {item.label}
          </button>
        </div>
      ))}
    </div>,
    root,
  );
}
