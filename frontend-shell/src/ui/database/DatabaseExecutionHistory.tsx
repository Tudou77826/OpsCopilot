import type { DatabaseBlock } from "../../workspace/database";

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
}: {
  blocks: DatabaseBlock[];
  onSelect(block: DatabaseBlock): void;
}) {
  return (
    <details className="oc-db-ai-history">
      <summary>AI 执行记录 · {blocks.length} 条</summary>
      <ol>
        {blocks.map((block, index) => (
          <li key={block.id}>
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
