import { useEffect, useMemo, useRef, useState } from 'react';
import { TbArrowDown, TbArrowsMaximize, TbArrowUp } from 'react-icons/tb';
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import { DatabaseMenu, type DatabaseMenuItem } from './DatabaseMenu';
import {
  CellDetailDialog,
  type CellDetailTarget,
} from './CellDetailDialog';
import {
  compareCellValues,
  formatRowCSV,
  formatRowInsertSQL,
  formatRowJSON,
  formatRowTSV,
  formatSelectionMatrix,
  rowMatchesFilter,
  type SQLDialect,
} from './resultGridUtils';
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

interface IndexedRow {
  origIndex: number;
  cells: (string | null)[];
}

interface CellCoord {
  row: number;
  col: number;
}

interface SelectionRange {
  anchor: CellCoord;
  focus: CellCoord;
}

export interface ResultGridTarget {
  schema?: string;
  table?: string;
  dialect?: SQLDialect;
}

export interface CellEditRequest {
  origRowIndex: number;
  colIndex: number;
  columnName: string;
  oldValue: string | null;
  newValue: string | null;
}

function normalizeBounds(range: SelectionRange | null) {
  if (!range) return null;
  return {
    r1: Math.min(range.anchor.row, range.focus.row),
    r2: Math.max(range.anchor.row, range.focus.row),
    c1: Math.min(range.anchor.col, range.focus.col),
    c2: Math.max(range.anchor.col, range.focus.col),
  };
}

export function ResultGrid({
  result,
  filterText = '',
  onFilteredCountChange,
  target,
  canEdit = false,
  pendingEdits,
  onStageCellEdit,
  onRevertCellEdit,
}: {
  result: DatabaseResultSet;
  filterText?: string;
  onFilteredCountChange?: (filtered: number, total: number) => void;
  target?: ResultGridTarget;
  canEdit?: boolean;
  pendingEdits?: Record<number, Record<number, string | null>>;
  onStageCellEdit?: (
    origRowIndex: number,
    colIndex: number,
    newValue: string | null,
  ) => void;
  onRevertCellEdit?: (origRowIndex: number, colIndex: number) => void;
}) {
  const parent = useRef<HTMLDivElement>(null);
  const noticeTimer = useRef<number | null>(null);
  const dragMode = useRef<'none' | 'cell' | 'row'>('none');
  const skipBlurCommit = useRef(false);
  const [sorting, setSorting] = useState<SortingState>([]);
  const [selection, setSelection] = useState<SelectionRange | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [expanded, setExpanded] = useState<
    (CellDetailTarget & { initialEditing?: boolean }) | null
  >(null);
  const [inlineEdit, setInlineEdit] = useState<{
    origRowIndex: number;
    colIndex: number;
    value: string;
    isNull: boolean;
  } | null>(null);
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    viewRowIndex: number;
    origRowIndex: number;
    colIndex: number;
    cells: (string | null)[];
  } | null>(null);
  const [notice, setNotice] = useState('');

  useEffect(() => {
    const endDrag = () => {
      if (dragMode.current !== 'none') {
        dragMode.current = 'none';
        setIsDragging(false);
      }
    };
    window.addEventListener('mouseup', endDrag);
    return () => {
      window.removeEventListener('mouseup', endDrag);
      if (noticeTimer.current !== null) window.clearTimeout(noticeTimer.current);
    };
  }, []);

  function showNotice(text: string) {
    setNotice(text);
    if (noticeTimer.current !== null) window.clearTimeout(noticeTimer.current);
    noticeTimer.current = window.setTimeout(() => {
      setNotice('');
      noticeTimer.current = null;
    }, 2200);
  }

  async function copyText(value: string, label = '已复制') {
    try {
      await navigator.clipboard.writeText(value);
      showNotice(label);
    } catch {
      showNotice('复制失败，请打开单元格后手动复制');
    }
  }

  const indexedRows = useMemo<IndexedRow[]>(() => {
    const all = result.rows.map((baseCells, origIndex) => {
      const rowEdits = pendingEdits?.[origIndex];
      const cells = rowEdits
        ? baseCells.map((cell, cIdx) =>
            Object.prototype.hasOwnProperty.call(rowEdits, cIdx)
              ? rowEdits[cIdx]
              : cell,
          )
        : baseCells;
      return { origIndex, cells };
    });
    if (!filterText.trim()) return all;
    return all.filter((item) => rowMatchesFilter(item.cells, filterText));
  }, [result.rows, pendingEdits, filterText]);

  useEffect(() => {
    onFilteredCountChange?.(indexedRows.length, result.rows.length);
  }, [indexedRows.length, result.rows.length, onFilteredCountChange]);

  const columns = useMemo<ColumnDef<IndexedRow>[]>(
    () =>
      result.columns.map((c, i) => ({
        id: String(i),
        accessorFn: (row) => row.cells[i] ?? null,
        size: 160,
        minSize: 72,
        maxSize: 700,
        sortingFn: (rowA, rowB, columnId) => {
          const a = rowA.getValue(columnId) as string | null;
          const b = rowB.getValue(columnId) as string | null;
          return compareCellValues(a, b);
        },
        header: () => (
          <span className="oc-db-col-label" title={c.type ? `${c.name} (${c.type})` : c.name}>
            <strong>{c.name}</strong>
            {c.type && <small>{c.type}</small>}
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
    data: indexedRows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    columnResizeMode: 'onChange',
  });

  const rows = table.getRowModel().rows;
  const colCount = result.columns.length;
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 28,
    overscan: 12,
  });

  const bounds = normalizeBounds(selection);
  const isAllSelected = Boolean(
    bounds &&
      rows.length > 0 &&
      colCount > 0 &&
      bounds.r1 === 0 &&
      bounds.r2 === rows.length - 1 &&
      bounds.c1 === 0 &&
      bounds.c2 === colCount - 1,
  );
  const isMultiCell = Boolean(
    bounds && (bounds.r2 > bounds.r1 || bounds.c2 > bounds.c1),
  );

  function extractSelectedMatrix(): (string | null)[][] {
    if (!bounds) return [];
    const matrix: (string | null)[][] = [];
    for (let r = bounds.r1; r <= bounds.r2 && r < rows.length; r++) {
      const cells = rows[r].original.cells;
      const slice: (string | null)[] = [];
      for (let c = bounds.c1; c <= bounds.c2 && c < colCount; c++) {
        slice.push(cells[c] ?? null);
      }
      matrix.push(slice);
    }
    return matrix;
  }

  function copyCurrentSelection(format: 'tsv' | 'csv' = 'tsv') {
    if (!bounds) return;
    if (!isMultiCell) {
      const val = rows[bounds.r1]?.original.cells[bounds.c1] ?? null;
      void copyText(val ?? 'NULL', '已复制单元格');
      return;
    }
    const matrix = extractSelectedMatrix();
    const rowNum = matrix.length;
    const colNum = matrix[0]?.length ?? 0;
    void copyText(
      formatSelectionMatrix(matrix, format),
      `已复制 ${rowNum}×${colNum} 选区 (${format.toUpperCase()})`,
    );
  }

  function toggleSelectAll() {
    if (rows.length === 0 || colCount === 0) return;
    if (isAllSelected) {
      setSelection(null);
      return;
    }
    setSelection({
      anchor: { row: 0, col: 0 },
      focus: { row: rows.length - 1, col: colCount - 1 },
    });
  }

  function moveFocus(nextRow: number, nextCol: number, extend: boolean) {
    if (rows.length === 0 || colCount === 0) return;
    const clampedRow = Math.max(0, Math.min(rows.length - 1, nextRow));
    const clampedCol = Math.max(0, Math.min(colCount - 1, nextCol));
    setSelection((prev) =>
      extend && prev
        ? { anchor: prev.anchor, focus: { row: clampedRow, col: clampedCol } }
        : {
            anchor: { row: clampedRow, col: clampedCol },
            focus: { row: clampedRow, col: clampedCol },
          },
    );
    virtual.scrollToIndex(clampedRow, { align: 'auto' });
    window.requestAnimationFrame(() => {
      const btn = parent.current?.querySelector<HTMLElement>(
        `[data-cell="${clampedRow}:${clampedCol}"]`,
      );
      btn?.focus({ preventScroll: false });
    });
  }

  function openDetail(
    viewRowIndex: number,
    origRowIndex: number,
    colIndex: number,
    value: string | null,
    initialEditing = false,
  ) {
    const col = result.columns[colIndex];
    if (!col) return;
    setExpanded({
      rowNumber: viewRowIndex + 1,
      origRowIndex,
      colIndex,
      columnName: col.name,
      columnType: col.type,
      value,
      initialEditing,
    });
  }

  function startInlineEdit(
    viewRowIndex: number,
    origRowIndex: number,
    colIndex: number,
    value: string | null,
  ) {
    if (!canEdit || !onStageCellEdit) return;
    if (value !== null && value.includes('\n')) {
      openDetail(viewRowIndex, origRowIndex, colIndex, value, true);
      return;
    }
    skipBlurCommit.current = false;
    setInlineEdit({
      origRowIndex,
      colIndex,
      value: value ?? '',
      isNull: value === null,
    });
  }

  function commitInlineEdit(
    origRowIndex: number,
    colIndex: number,
    currentDisplayValue: string | null,
  ) {
    if (!inlineEdit) return;
    const nextValue = inlineEdit.isNull ? null : inlineEdit.value;
    setInlineEdit(null);
    if (nextValue === currentDisplayValue) return;
    onStageCellEdit?.(origRowIndex, colIndex, nextValue);
  }

  const menuItems: DatabaseMenuItem[] = contextMenu
    ? (() => {
        const col = result.columns[contextMenu.colIndex];
        const cellValue = contextMenu.cells[contextMenu.colIndex] ?? null;
        const items: DatabaseMenuItem[] = [
          {
            label: '查看单元格详情',
            action: () =>
              openDetail(
                contextMenu.viewRowIndex,
                contextMenu.origRowIndex,
                contextMenu.colIndex,
                cellValue,
              ),
          },
        ];
        if (canEdit && onStageCellEdit) {
          items.push({
            label: '编辑单元格…',
            action: () =>
              startInlineEdit(
                contextMenu.viewRowIndex,
                contextMenu.origRowIndex,
                contextMenu.colIndex,
                cellValue,
              ),
          });
          const isDirty = Boolean(
            pendingEdits?.[contextMenu.origRowIndex] &&
              Object.prototype.hasOwnProperty.call(
                pendingEdits[contextMenu.origRowIndex],
                contextMenu.colIndex,
              ),
          );
          if (isDirty && onRevertCellEdit) {
            items.push({
              label: '撤销此格修改',
              action: () =>
                onRevertCellEdit(
                  contextMenu.origRowIndex,
                  contextMenu.colIndex,
                ),
            });
          }
        }
        if (isMultiCell) {
          items.push(
            {
              label: '复制选区 (TSV)',
              separator: true,
              action: () => copyCurrentSelection('tsv'),
            },
            {
              label: '复制选区 (CSV)',
              action: () => copyCurrentSelection('csv'),
            },
          );
        }
        items.push(
          {
            label: '复制单元格',
            separator: !isMultiCell,
            action: () => void copyText(cellValue ?? 'NULL', '已复制单元格'),
          },
          {
            label: '复制列名',
            disabled: !col,
            action: () => col && void copyText(col.name, '已复制列名'),
          },
          {
            label: '复制整行 (TSV)',
            separator: true,
            action: () =>
              void copyText(formatRowTSV(contextMenu.cells), '已复制整行 (TSV)'),
          },
          {
            label: '复制整行 (CSV)',
            action: () =>
              void copyText(formatRowCSV(contextMenu.cells), '已复制整行 (CSV)'),
          },
          {
            label: '复制整行 (JSON)',
            action: () =>
              void copyText(
                formatRowJSON(result.columns, contextMenu.cells),
                '已复制整行 (JSON)',
              ),
          },
          {
            label: '复制为 INSERT SQL',
            action: () =>
              void copyText(
                formatRowInsertSQL(result.columns, contextMenu.cells, target),
                '已复制 INSERT SQL',
              ),
          },
        );
        return items;
      })()
    : [];

  return (
    <div className={`oc-db-grid-wrap${isDragging ? ' is-dragging' : ''}`}>
      <div
        className="oc-db-grid"
        ref={parent}
        role="table"
        aria-label="查询结果"
        aria-rowcount={rows.length + 1}
        onKeyDown={(e) => {
          if (inlineEdit) return;
          if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') {
            if (rows.length > 0 && colCount > 0) {
              e.preventDefault();
              setSelection({
                anchor: { row: 0, col: 0 },
                focus: { row: rows.length - 1, col: colCount - 1 },
              });
            }
          }
        }}
      >
        <div
          className="oc-db-grid-head"
          role="row"
          style={{ width: table.getTotalSize() + 44 }}
        >
          <button
            type="button"
            className={`oc-db-row-number oc-db-select-all${isAllSelected ? ' is-active' : ''}`}
            role="columnheader"
            aria-label={isAllSelected ? '取消全选' : '全选所有行'}
            title={isAllSelected ? '取消全选' : '点击全选 (Ctrl+A)'}
            onClick={toggleSelectAll}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c' && bounds) {
                e.preventDefault();
                copyCurrentSelection('tsv');
              }
            }}
          >
            #
          </button>
          {table.getHeaderGroups()[0]?.headers.map((h) => {
            const sortDir = h.column.getIsSorted();
            return (
              <div
                role="columnheader"
                aria-sort={
                  sortDir === 'asc'
                    ? 'ascending'
                    : sortDir === 'desc'
                      ? 'descending'
                      : 'none'
                }
                key={h.id}
                style={{ width: h.getSize() }}
                className={sortDir ? 'is-sorted' : ''}
              >
                <button
                  type="button"
                  className="oc-db-col-sort"
                  onClick={h.column.getToggleSortingHandler()}
                  title="点击切换排序（升序 / 降序 / 还原）"
                >
                  {flexRender(h.column.columnDef.header, h.getContext())}
                  {sortDir === 'asc' && (
                    <TbArrowUp className="oc-db-sort-icon" size={12} aria-hidden />
                  )}
                  {sortDir === 'desc' && (
                    <TbArrowDown className="oc-db-sort-icon" size={12} aria-hidden />
                  )}
                </button>
                <span
                  className="oc-db-resizer"
                  onClick={(e) => e.stopPropagation()}
                  onMouseDown={h.getResizeHandler()}
                  onTouchStart={h.getResizeHandler()}
                />
              </div>
            );
          })}
        </div>
        <div
          style={{
            height: virtual.getTotalSize(),
            position: 'relative',
            width: table.getTotalSize() + 44,
          }}
        >
          {virtual.getVirtualItems().map((item) => {
            const row = rows[item.index];
            const origRowIndex = row.original.origIndex;
            const isRowInRange = Boolean(
              bounds && item.index >= bounds.r1 && item.index <= bounds.r2,
            );
            const isFullRowInRange = Boolean(
              isRowInRange &&
                bounds &&
                bounds.c1 === 0 &&
                bounds.c2 === colCount - 1,
            );
            return (
              <div
                role="row"
                aria-rowindex={item.index + 2}
                aria-selected={isRowInRange}
                className={`oc-db-grid-row${isRowInRange ? ' is-row-selected' : ''}${isFullRowInRange ? ' is-full-row-selected' : ''}`}
                key={row.id}
                style={{
                  transform: `translateY(${item.start}px)`,
                  height: item.size,
                }}
              >
                <button
                  type="button"
                  className="oc-db-row-number"
                  role="rowheader"
                  aria-label={`选中第 ${item.index + 1} 行`}
                  onMouseDown={(e) => {
                    if (e.button !== 0 || colCount === 0) return;
                    dragMode.current = 'row';
                    setIsDragging(true);
                    if (e.shiftKey && selection) {
                      setSelection({
                        anchor: { row: selection.anchor.row, col: 0 },
                        focus: { row: item.index, col: colCount - 1 },
                      });
                    } else {
                      setSelection({
                        anchor: { row: item.index, col: 0 },
                        focus: { row: item.index, col: colCount - 1 },
                      });
                    }
                  }}
                  onMouseEnter={() => {
                    if (dragMode.current === 'row' && colCount > 0) {
                      setSelection((prev) =>
                        prev
                          ? {
                              anchor: { row: prev.anchor.row, col: 0 },
                              focus: { row: item.index, col: colCount - 1 },
                            }
                          : null,
                      );
                    }
                  }}
                  onKeyDown={(e) => {
                    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') {
                      e.preventDefault();
                      copyCurrentSelection('tsv');
                    }
                  }}
                >
                  {item.index + 1}
                </button>
                {row.getVisibleCells().map((cell, colIndex) => {
                  const value = cell.getValue() as string | null;
                  const isInRange = Boolean(
                    bounds &&
                      item.index >= bounds.r1 &&
                      item.index <= bounds.r2 &&
                      colIndex >= bounds.c1 &&
                      colIndex <= bounds.c2,
                  );
                  const isFocusCell = Boolean(
                    selection &&
                      selection.focus.row === item.index &&
                      selection.focus.col === colIndex,
                  );
                  const isDirtyCell = Boolean(
                    pendingEdits?.[origRowIndex] &&
                      Object.prototype.hasOwnProperty.call(
                        pendingEdits[origRowIndex],
                        colIndex,
                      ),
                  );
                  const isEditingThis =
                    inlineEdit?.origRowIndex === origRowIndex &&
                    inlineEdit?.colIndex === colIndex;

                  if (isEditingThis) {
                    return (
                      <div
                        role="cell"
                        key={cell.id}
                        className="oc-db-inline-edit"
                        style={{ width: cell.column.getSize() }}
                        onBlur={(e) => {
                          if (e.currentTarget.contains(e.relatedTarget as Node)) {
                            return;
                          }
                          if (skipBlurCommit.current) {
                            skipBlurCommit.current = false;
                            return;
                          }
                          commitInlineEdit(origRowIndex, colIndex, value);
                        }}
                      >
                        <input
                          autoFocus
                          spellCheck={false}
                          aria-label={`编辑 ${result.columns[colIndex]?.name ?? ''}`}
                          readOnly={inlineEdit.isNull}
                          placeholder={inlineEdit.isNull ? 'NULL' : ''}
                          value={inlineEdit.isNull ? '' : inlineEdit.value}
                          onChange={(e) => {
                            if (inlineEdit.isNull) return;
                            setInlineEdit((prev) =>
                              prev ? { ...prev, value: e.target.value } : null,
                            );
                          }}
                          onKeyDown={(e) => {
                            e.stopPropagation();
                            if (e.key === 'Enter') {
                              e.preventDefault();
                              skipBlurCommit.current = true;
                              commitInlineEdit(origRowIndex, colIndex, value);
                            } else if (e.key === 'Escape') {
                              e.preventDefault();
                              skipBlurCommit.current = true;
                              setInlineEdit(null);
                              moveFocus(item.index, colIndex, false);
                            }
                          }}
                        />
                        <button
                          type="button"
                          className={`oc-db-inline-null${inlineEdit.isNull ? ' is-active' : ''}`}
                          title="切换 NULL"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() =>
                            setInlineEdit((prev) =>
                              prev ? { ...prev, isNull: !prev.isNull } : null,
                            )
                          }
                        >
                          NULL
                        </button>
                      </div>
                    );
                  }

                  return (
                    <div
                      role="cell"
                      tabIndex={0}
                      key={cell.id}
                      data-cell={`${item.index}:${colIndex}`}
                      aria-selected={isInRange}
                      className={`oc-db-grid-cell${isInRange ? ' is-cell-in-range' : ''}${isFocusCell ? ' is-cell-selected' : ''}${isDirtyCell ? ' is-cell-dirty' : ''}`}
                      style={{ width: cell.column.getSize() }}
                      title={value ?? 'NULL'}
                      onMouseDown={(e) => {
                        if (e.button !== 0) return;
                        dragMode.current = 'cell';
                        setIsDragging(true);
                        if (e.shiftKey && selection) {
                          setSelection({
                            anchor: selection.anchor,
                            focus: { row: item.index, col: colIndex },
                          });
                        } else {
                          setSelection({
                            anchor: { row: item.index, col: colIndex },
                            focus: { row: item.index, col: colIndex },
                          });
                        }
                      }}
                      onMouseEnter={() => {
                        if (dragMode.current === 'cell') {
                          setSelection((prev) =>
                            prev
                              ? {
                                  anchor: prev.anchor,
                                  focus: { row: item.index, col: colIndex },
                                }
                              : null,
                          );
                        }
                      }}
                      onClick={(e) => {
                        if (e.shiftKey) return;
                        if (!selection) {
                          setSelection({
                            anchor: { row: item.index, col: colIndex },
                            focus: { row: item.index, col: colIndex },
                          });
                        }
                      }}
                      onDoubleClick={() => {
                        setSelection({
                          anchor: { row: item.index, col: colIndex },
                          focus: { row: item.index, col: colIndex },
                        });
                        if (canEdit && onStageCellEdit) {
                          startInlineEdit(
                            item.index,
                            origRowIndex,
                            colIndex,
                            value,
                          );
                        } else {
                          openDetail(item.index, origRowIndex, colIndex, value);
                        }
                      }}
                      onContextMenu={(e) => {
                        e.preventDefault();
                        if (!isInRange) {
                          setSelection({
                            anchor: { row: item.index, col: colIndex },
                            focus: { row: item.index, col: colIndex },
                          });
                        }
                        setContextMenu({
                          x: e.clientX,
                          y: e.clientY,
                          viewRowIndex: item.index,
                          origRowIndex,
                          colIndex,
                          cells: row.original.cells,
                        });
                      }}
                      onKeyDown={(e) => {
                        if (
                          (e.ctrlKey || e.metaKey) &&
                          e.key.toLowerCase() === 'c'
                        ) {
                          e.preventDefault();
                          copyCurrentSelection('tsv');
                          return;
                        }
                        if (e.key === ' ' || e.key === 'Enter') {
                          e.preventDefault();
                          openDetail(item.index, origRowIndex, colIndex, value);
                          return;
                        }
                        if (e.key === 'F2' && canEdit && onStageCellEdit) {
                          e.preventDefault();
                          startInlineEdit(
                            item.index,
                            origRowIndex,
                            colIndex,
                            value,
                          );
                          return;
                        }
                        if (
                          (e.shiftKey && e.key === 'F10') ||
                          e.key === 'ContextMenu'
                        ) {
                          e.preventDefault();
                          const rect = e.currentTarget.getBoundingClientRect();
                          setContextMenu({
                            x: rect.left + 12,
                            y: rect.bottom,
                            viewRowIndex: item.index,
                            origRowIndex,
                            colIndex,
                            cells: row.original.cells,
                          });
                          return;
                        }
                        if (e.key === 'ArrowDown') {
                          e.preventDefault();
                          moveFocus(item.index + 1, colIndex, e.shiftKey);
                        } else if (e.key === 'ArrowUp') {
                          e.preventDefault();
                          moveFocus(item.index - 1, colIndex, e.shiftKey);
                        } else if (e.key === 'ArrowRight') {
                          e.preventDefault();
                          moveFocus(item.index, colIndex + 1, e.shiftKey);
                        } else if (e.key === 'ArrowLeft') {
                          e.preventDefault();
                          moveFocus(item.index, colIndex - 1, e.shiftKey);
                        }
                      }}
                    >
                      <span className="oc-db-cell-text">
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext(),
                        )}
                      </span>
                      <button
                        type="button"
                        tabIndex={-1}
                        className="oc-db-cell-expand"
                        aria-label={`查看 ${result.columns[colIndex]?.name ?? ''} 详情`}
                        title="查看完整内容 (Space)"
                        onMouseDown={(e) => e.stopPropagation()}
                        onDoubleClick={(e) => e.stopPropagation()}
                        onClick={(e) => {
                          e.stopPropagation();
                          setSelection({
                            anchor: { row: item.index, col: colIndex },
                            focus: { row: item.index, col: colIndex },
                          });
                          openDetail(item.index, origRowIndex, colIndex, value);
                        }}
                      >
                        <TbArrowsMaximize size={14} />
                      </button>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
        {rows.length === 0 && (
          <div className="oc-db-grid-empty">
            {result.rows.length > 0 && filterText.trim()
              ? `没有匹配「${filterText.trim()}」的行`
              : '查询完成，没有返回数据'}
          </div>
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

      {contextMenu && (
        <DatabaseMenu
          x={contextMenu.x}
          y={contextMenu.y}
          label="查询结果操作"
          items={menuItems}
          onClose={() => setContextMenu(null)}
        />
      )}

      {expanded && (
        <CellDetailDialog
          cell={expanded}
          canEdit={canEdit && !!onStageCellEdit}
          initialEditing={expanded.initialEditing}
          onEditSubmit={(newValue) => {
            const targetCell = expanded;
            setExpanded(null);
            onStageCellEdit?.(
              targetCell.origRowIndex,
              targetCell.colIndex,
              newValue,
            );
          }}
          onClose={() => setExpanded(null)}
        />
      )}
    </div>
  );
}
