import { useEffect, useRef, useState } from "react";
import { IconButton, ResourceLibraryIcon } from '@opscopilot/shell-terminal/ui';
import type { AssistantCommand } from "./types";

export function CommandMenu({
  commands,
  query,
  onChoose,
  onManage,
}: {
  commands: AssistantCommand[];
  query: string;
  onChoose(command: AssistantCommand): void;
  onManage(): void;
}) {
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const matches = commands.filter((c) =>
    (c.name + " " + c.description).toLowerCase().includes(query.toLowerCase()),
  );
  useEffect(() => {
    const node = root.current;
    const anchor = node?.parentElement;
    if (!node || !anchor || !node.showPopover) return;
    // Native top-layer rendering keeps the menu above the scrollable footer
    // without changing its DOM ownership or stealing terminal keyboard input.
    node.setAttribute("popover", "manual");
    const position = () => {
      const rect = anchor.getBoundingClientRect();
      Object.assign(node.style, {
        left: `${rect.left + 12}px`,
        bottom: `${window.innerHeight - rect.top + 6}px`,
        width: `${Math.max(180, rect.width - 24)}px`,
      });
    };
    position();
    node.showPopover();
    const observer = new ResizeObserver(position);
    observer.observe(anchor);
    window.addEventListener("resize", position);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", position);
      if (node.isConnected) node.hidePopover();
    };
  }, []);
  useEffect(() => setActive(0), [query]);
  useEffect(() => {
    const handle = (event: KeyboardEvent) => {
      if (!root.current?.parentElement?.contains(event.target as Node)) return;
      if (event.isComposing) return;
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        setActive((old) =>
          Math.max(
            0,
            Math.min(
              matches.length - 1,
              old + (event.key === "ArrowDown" ? 1 : -1),
            ),
          ),
        );
      }
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (matches[active]) onChoose(matches[active]);
      }
    };
    // Capture before the textarea's ordinary send handler.
    document.addEventListener("keydown", handle, true);
    return () => document.removeEventListener("keydown", handle, true);
  }, [matches, active, onChoose]);
  return (
    <div ref={root} className="oc-command-menu" role="listbox" aria-label="快捷命令">
      <header>
        <span>快捷命令</span>
        <IconButton label="管理快捷命令" onClick={onManage}><ResourceLibraryIcon /></IconButton>
      </header>
      <div>
        {matches.length ? (
          matches.map((command, index) => (
            <button
              type="button"
              role="option"
              aria-selected={index === active}
              key={command.name}
              onMouseEnter={() => setActive(index)}
              onClick={() => onChoose(command)}
            >
              <strong>/{command.name}</strong>
              <span>{command.description}</span>
            </button>
          ))
        ) : (
          <p>
            {commands.length ? "没有匹配的命令" : "还没有快捷命令"}
            <button onClick={onManage}>去创建</button>
          </p>
        )}
      </div>
    </div>
  );
}
