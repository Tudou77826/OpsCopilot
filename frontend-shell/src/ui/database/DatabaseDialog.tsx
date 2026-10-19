import {
  useEffect,
  useId,
  useRef,
  type FormEventHandler,
  type ReactNode,
  type RefObject,
} from 'react';
import { TbX } from 'react-icons/tb';

/** Shared modal frame: only the body scrolls; actions and feedback stay visible. */
export function DatabaseDialog({
  title,
  subtitle,
  children,
  navigation,
  feedback,
  actions,
  onClose,
  onSubmit,
  busy = false,
  visible = true,
  compact = false,
  formRef,
  noValidate = false,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  navigation?: ReactNode;
  feedback?: ReactNode;
  actions: ReactNode;
  onClose(): void;
  onSubmit?: FormEventHandler<HTMLFormElement>;
  busy?: boolean;
  visible?: boolean;
  compact?: boolean;
  formRef?: RefObject<HTMLFormElement>;
  noValidate?: boolean;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const lastFocused = useRef<HTMLElement | null>(null);
  const titleId = useId();
  useEffect(() => {
    const element = dialog.current;
    if (!visible) return;
    const previous = document.activeElement;
    const resumeFocus = lastFocused.current;
    element?.showModal();
    if (resumeFocus && element?.contains(resumeFocus)) {
      resumeFocus.focus();
    } else {
      element?.querySelector<HTMLElement>('[data-db-initial-focus]')?.focus();
    }
    return () => {
      if (
        document.activeElement instanceof HTMLElement &&
        element?.contains(document.activeElement)
      ) {
        lastFocused.current = document.activeElement;
      }
      element?.close();
      if (
        previous instanceof HTMLElement &&
        previous.isConnected &&
        !element?.contains(previous)
      )
        previous.focus();
    };
  }, [visible]);
  return (
    <dialog
      ref={dialog}
      hidden={!visible}
      className={`oc-db-dialog oc-db-modal${compact ? ' oc-db-modal-compact' : ''}`}
      aria-labelledby={titleId}
      onFocusCapture={(e) => {
        if (e.target instanceof HTMLElement) lastFocused.current = e.target;
      }}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (
          !(e.target instanceof HTMLButtonElement) ||
          e.target.getAttribute('role') !== 'tab'
        )
          return;
        const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End'];
        if (!keys.includes(e.key)) return;
        e.preventDefault();
        const tabs = Array.from(
          e.target.parentElement?.querySelectorAll<HTMLButtonElement>(
            '[role="tab"]:not(:disabled)',
          ) ?? [],
        );
        const index = tabs.indexOf(e.target);
        const next =
          e.key === 'Home'
            ? 0
            : e.key === 'End'
              ? tabs.length - 1
              : (index + (e.key === 'ArrowRight' ? 1 : -1) + tabs.length) %
                tabs.length;
        tabs[next]?.click();
        tabs[next]?.focus();
      }}
    >
      <form
        ref={formRef}
        noValidate={noValidate}
        onSubmit={(e) => {
          e.preventDefault();
          if (!busy) onSubmit?.(e);
        }}
      >
        <header className="oc-db-modal-header">
          <div>
            <h3 id={titleId}>{title}</h3>
            {subtitle && <p>{subtitle}</p>}
          </div>
          <button
            type="button"
            aria-label={`关闭${title}`}
            title="关闭"
            disabled={busy}
            onClick={onClose}
          >
            <TbX size={18} />
          </button>
        </header>
        {navigation}
        <div className="oc-db-modal-body">{children}</div>
        <footer className="oc-db-modal-footer">
          {feedback && <div className="oc-db-modal-feedback">{feedback}</div>}
          <div className="oc-db-modal-actions">{actions}</div>
        </footer>
      </form>
    </dialog>
  );
}
