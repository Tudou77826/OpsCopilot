import type { DatabaseBlock } from "../../workspace/database";
import { useEffect, useRef } from 'react';

const labels: Record<string, string> = {
  not_executed: "未执行",
  awaiting_approval: "等待确认",
  preparing: "准备执行",
  running: "执行中",
  completed: "已完成",
  failed: "失败",
  cancelled: "已停止",
};
export function DatabaseExecutionHistory({
  blocks,
  onSelect,
  revealId, revealNonce,
}: {
  blocks: DatabaseBlock[];
  onSelect(block: DatabaseBlock): void;
  revealId?: string; revealNonce?: string;
}) {
  const root = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    if (!revealId || !revealNonce || !root.current) return;
    const row = Array.from(root.current.querySelectorAll<HTMLElement>('[data-block-id]')).find(item => item.dataset.blockId === revealId);
    if (!row) return;
    root.current.open = true;
    const detail = row.querySelector('details');
    if (detail) detail.open = true;
    row.scrollIntoView?.({block:'nearest'});
    row.focus({preventScroll:true});
  }, [revealId, revealNonce]);
  return (
    <details className="oc-db-ai-history" ref={root}>
      <summary>AI 执行记录 · {blocks.length} 条</summary>
      <ol>
        {blocks.map((block, index) => (
          <li key={block.id} data-block-id={block.id} tabIndex={-1}>
            <details>
              <summary>
                {index + 1}. {block.purpose}
              </summary>
              <pre>{block.sql}</pre>
            </details>
            <small>{labels[block.status] || block.status}</small>
            <button onClick={() => onSelect(block)}>查看结果</button>
          </li>
        ))}
      </ol>
    </details>
  );
}
