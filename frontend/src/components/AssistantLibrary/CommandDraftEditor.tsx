import { useEffect, useRef } from "react";
import { TbX } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { CommandDraft, ConnectionChoice } from "./types";

// Keep editable prose in its own DOM spans. React only controls the typed blanks,
// so changing a blank never resets a caret in the surrounding instruction.
export function CommandDraftEditor({
  draft,
  connections,
  disabled,
  onChange,
  onSend,
  onCancel,
}: {
  draft: CommandDraft;
  connections: ConnectionChoice[];
  disabled: boolean;
  onChange(draft: CommandDraft): void;
  onSend(): void;
  onCancel(): void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const initial = useRef(
    draft.template.split(/(\{\{[a-zA-Z][a-zA-Z0-9_]*\}\})/g),
  );
  useEffect(() => {
    root.current
      ?.querySelector<HTMLInputElement | HTMLSelectElement>("input,select")
      ?.focus();
  }, []);
  function readProse() {
    const nodes = root.current?.querySelectorAll<HTMLElement>(
      "[data-prose],[data-field]",
    );
    if (!nodes) return;
    const template = Array.from(nodes, (node) =>
      node.dataset.field
        ? "{{" + node.dataset.field + "}}"
        : (node.innerText ?? node.textContent ?? ""),
    ).join("");
    onChange({ ...draft, template });
  }
  return (
    <div className="oc-command-draft">
      <header>
        <span>/{draft.command.name}</span>
        <IconButton
          label="退出快捷命令"
          onClick={onCancel}
          disabled={disabled}
        >
          <TbX size={18} />
        </IconButton>
      </header>
      <div
        ref={root}
        className="oc-command-sentence"
        aria-label="填写快捷指令"
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            if (!disabled) onSend();
          }
          if (event.key === "Tab") {
            const blanks = Array.from(
              root.current?.querySelectorAll<
                HTMLInputElement | HTMLSelectElement
              >("input,select") ?? [],
            );
            const index = blanks.indexOf(event.target as HTMLInputElement);
            const next = blanks[index + (event.shiftKey ? -1 : 1)];
            if (next) {
              event.preventDefault();
              next.focus();
            }
          }
        }}
      >
        {initial.current.map((part, index) => {
          if (!part.startsWith("{{"))
            return (
              <span
                key={index}
                data-prose
                tabIndex={-1}
                contentEditable={!disabled}
                suppressContentEditableWarning
                onInput={readProse}
                onPaste={(event) => {
                  event.preventDefault();
                  document.execCommand(
                    "insertText",
                    false,
                    event.clipboardData.getData("text/plain"),
                  );
                }}
              >
                {part}
              </span>
            );
          const key = part.slice(2, -2);
          const field = draft.command.fields.find((f) => f.key === key);
          if (!field) return null;
          const value = draft.values[key] ?? "";
          return (
            <span
              key={index}
              data-field={key}
              className={`oc-command-blank${value ? " is-filled" : ""}`}
            >
              {field.kind === "connection" ? (
                <select
                  aria-label={field.label}
                  disabled={disabled}
                  value={value}
                  required={field.required}
                  onChange={(e) =>
                    onChange({
                      ...draft,
                      values: { ...draft.values, [key]: e.target.value },
                    })
                  }
                >
                  <option value="">{field.label}</option>
                  {connections.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  aria-label={field.label}
                  placeholder={field.label}
                  value={value}
                  disabled={disabled}
                  required={field.required}
                  maxLength={2048}
                  style={{
                    width: `${Math.max(5, Math.min(30, Array.from(value || field.label).reduce((n, c) => n + (c.charCodeAt(0) > 255 ? 2 : 1), 0) + 1))}ch`,
                  }}
                  onChange={(e) =>
                    onChange({
                      ...draft,
                      values: { ...draft.values, [key]: e.target.value },
                    })
                  }
                />
              )}
            </span>
          );
        })}
      </div>
      <small>
        {draft.command.fields.length
          ? "填写高亮空位 · Tab 切换 · 正文可修改"
          : "正文可修改后发送"}
      </small>
    </div>
  );
}
