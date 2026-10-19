import { useMemo, useRef, useState } from 'react';
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { DatabaseResultSet } from './types';

export function resultCSV(result: DatabaseResultSet) {
  const escape = (v: string | null) =>
    v === null ? '' : `"${v.replace(/"/g, '""')}"`;
  return (
    '\uFEFF' +
    [
      result.columns.map((c) => escape(c.name)).join(','),
      ...result.rows.map((row) => row.map(escape).join(',')),
    ].join('\r\n')
  );
}
export function ResultGrid({ result }: { result: DatabaseResultSet }) {
  const parent = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState<{
    title: string;
    value: string;
  } | null>(null);
  const [notice, setNotice] = useState('');
  const columns = useMemo<ColumnDef<(string | null)[]>[]>(
    () =>
      result.columns.map((c, i) => ({
        id: String(i),
        accessorFn: (row) => row[i],
        size: 170,
        minSize: 80,
        maxSize: 700,
        header: () => (
          <span title={c.type}>
            {c.name}
            <small>{c.type}</small>
          </span>
        ),
        cell: (ctx) => {
          const value = ctx.getValue() as string | null;
          return value === null ? (
            <em>NULL</em>
          ) : value === '' ? (
            <em>空字符串</em>
          ) : (
            value
          );
        },
      })),
    [result.columns],
  );
  const table = useReactTable({
    data: result.rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
    columnResizeMode: 'onChange',
  });
  const rows = table.getRowModel().rows;
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 30,
    overscan: 12,
  });
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice('已复制');
    } catch {
      setNotice('复制失败，请打开单元格后手动复制');
    }
  }
  return (
    <div className="oc-db-grid-wrap">
      <div
        className="oc-db-grid"
        ref={parent}
        role="table"
        aria-label="查询结果"
        aria-rowcount={rows.length + 1}
      >
        <div
          className="oc-db-grid-head"
          role="row"
          style={{ width: table.getTotalSize() + 48 }}
        >
          <span className="oc-db-row-number" role="columnheader">
            #
          </span>
          {table.getHeaderGroups()[0]?.headers.map((h) => (
            <div role="columnheader" key={h.id} style={{ width: h.getSize() }}>
              {flexRender(h.column.columnDef.header, h.getContext())}
              <span
                className="oc-db-resizer"
                onMouseDown={h.getResizeHandler()}
                onTouchStart={h.getResizeHandler()}
              />
            </div>
          ))}
        </div>
        <div
          style={{
            height: virtual.getTotalSize(),
            position: 'relative',
            width: table.getTotalSize() + 48,
          }}
        >
          {virtual.getVirtualItems().map((item) => {
            const row = rows[item.index];
            return (
              <div
                role="row"
                aria-rowindex={item.index + 2}
                className="oc-db-grid-row"
                key={row.id}
                style={{
                  transform: `translateY(${item.start}px)`,
                  height: item.size,
                }}
              >
                <span className="oc-db-row-number" role="rowheader">
                  {item.index + 1}
                </span>
                {row.getVisibleCells().map((cell) => {
                  const value = cell.getValue() as string | null;
                  return (
                    <button
                      type="button"
                      role="cell"
                      key={cell.id}
                      style={{ width: cell.column.getSize() }}
                      title={value ?? 'NULL'}
                      onDoubleClick={() =>
                        setExpanded({
                          title: result.columns[Number(cell.column.id)].name,
                          value: value ?? 'NULL',
                        })
                      }
                      onKeyDown={(e) => {
                        if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
                          e.preventDefault();
                          void copy(value ?? 'NULL');
                        }
                        if (e.key === 'Enter')
                          setExpanded({
                            title: result.columns[Number(cell.column.id)].name,
                            value: value ?? 'NULL',
                          });
                      }}
                    >
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </button>
                  );
                })}
              </div>
            );
          })}
        </div>
        {rows.length === 0 && (
          <div className="oc-db-grid-empty">查询完成，没有返回数据</div>
        )}
      </div>
      {notice && (
        <span
          className="oc-db-copy-notice"
          role="status"
          onClick={() => setNotice('')}
        >
          {notice}
        </span>
      )}
      {expanded && (
        <div className="oc-db-cell-detail">
          <header>
            <strong>{expanded.title}</strong>
            <button onClick={() => void copy(expanded.value)}>复制</button>
            <button
              aria-label="关闭单元格详情"
              onClick={() => setExpanded(null)}
            >
              ×
            </button>
          </header>
          <textarea
            aria-label="单元格完整内容"
            readOnly
            value={expanded.value}
          />
        </div>
      )}
    </div>
  );
}
