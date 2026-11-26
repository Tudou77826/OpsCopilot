import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react';
import { ResultGrid } from './ResultGrid';
import {
  applyPendingEditsToResult,
  buildBatchUpdateSQL,
  buildUpdateSQL,
  compareCellValues,
  extractPrimaryKeys,
  formatRowCSV,
  formatRowInsertSQL,
  formatRowJSON,
  formatRowTSV,
  inferEditableTarget,
  rowMatchesFilter,
  tryFormatJSON,
} from './resultGridUtils';
import type { DatabaseResultSet, DatabaseStructure } from './types';

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 30,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        start: index * 30,
        size: 30,
        key: index,
      })),
    scrollToIndex: vi.fn(),
  }),
}));

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (
    this: HTMLDialogElement,
  ) {
    this.setAttribute('open', '');
  });
  HTMLDialogElement.prototype.close = vi.fn(function (
    this: HTMLDialogElement,
  ) {
    this.removeAttribute('open');
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const sampleResult: DatabaseResultSet = {
  statement: 'SELECT id, name, payload FROM events',
  columns: [
    { name: 'id', type: 'BIGINT' },
    { name: 'name', type: 'VARCHAR' },
    { name: 'payload', type: 'JSON' },
  ],
  rows: [
    ['10', 'beta', '{"a":1,"b":2}'],
    ['2', 'alpha', null],
    ['100', 'gamma', 'plain text'],
  ],
  affectedRows: 0,
  truncated: false,
};

describe('resultGridUtils', () => {
  it('compares numbers, bigints, strings and NULLs consistently', () => {
    expect(compareCellValues('2', '10')).toBeLessThan(0);
    expect(
      compareCellValues('9007199254740993', '9007199254740992'),
    ).toBeGreaterThan(0);
    expect(compareCellValues(null, 'a')).toBeGreaterThan(0);
    expect(compareCellValues('a', null)).toBeLessThan(0);
    expect(compareCellValues(null, null)).toBe(0);
  });

  it('formats rows as CSV, TSV, JSON and INSERT SQL', () => {
    expect(formatRowCSV(['1', 'a"b', null])).toBe('"1","a""b",');
    expect(formatRowTSV(['1', 'line1\nline2', null])).toBe(
      '1\tline1 line2\tNULL',
    );
    expect(
      JSON.parse(formatRowJSON(sampleResult.columns, sampleResult.rows[1])),
    ).toEqual({
      id: '2',
      name: 'alpha',
      payload: null,
    });
    expect(
      formatRowInsertSQL(sampleResult.columns, ['2', "O'Brien", null], {
        schema: 'orders',
        table: 'events',
        dialect: 'mysql',
      }),
    ).toBe(
      "INSERT INTO `orders`.`events` (`id`, `name`, `payload`) VALUES ('2', 'O''Brien', NULL);",
    );
  });

  it('detects and pretty-prints JSON values only', () => {
    expect(tryFormatJSON('{"k":1}')).toBe('{\n  "k": 1\n}');
    expect(tryFormatJSON('hello')).toBeNull();
    expect(tryFormatJSON(null)).toBeNull();
  });

  it('matches filter text case-insensitively including NULL', () => {
    expect(rowMatchesFilter(['1', 'Alpha', null], 'alp')).toBe(true);
    expect(rowMatchesFilter(['1', 'Alpha', null], 'null')).toBe(true);
    expect(rowMatchesFilter(['1', 'Alpha', 'ok'], 'zzz')).toBe(false);
  });

  it('extracts primary keys and builds safe UPDATE SQL', () => {
    const structure: DatabaseStructure = {
      columns: {
        statement: '',
        columns: [
          { name: '字段', type: 'TEXT' },
          { name: '键', type: 'TEXT' },
        ],
        rows: [
          ['id', 'PRI'],
          ['name', ''],
        ],
        affectedRows: 0,
        truncated: false,
      },
      indexes: {
        statement: '',
        columns: [],
        rows: [],
        affectedRows: 0,
        truncated: false,
      },
      ddl: '',
    };
    const pks = extractPrimaryKeys(structure);
    expect(pks).toEqual(['id']);

    const built = buildUpdateSQL({
      schema: 'orders',
      table: 'events',
      dialect: 'mysql',
      column: 'name',
      newValue: 'updated',
      columns: sampleResult.columns,
      originalRow: sampleResult.rows[0],
      primaryKeys: pks,
    });
    expect(built.usedPrimaryKey).toBe(true);
    expect(built.sql).toBe(
      "UPDATE `orders`.`events`\nSET `name` = 'updated'\nWHERE `id` = '10';",
    );

    const fallback = buildUpdateSQL({
      schema: 'public',
      table: 'events',
      dialect: 'standard',
      column: 'name',
      newValue: null,
      columns: sampleResult.columns,
      originalRow: sampleResult.rows[1],
      primaryKeys: [],
    });
    expect(fallback.usedPrimaryKey).toBe(false);
    expect(fallback.sql).toBe(
      'UPDATE "public"."events"\nSET "name" = NULL\nWHERE "id" = \'2\' AND "name" = \'alpha\' AND "payload" IS NULL;',
    );
  });

  it('infers single editable table from simple SELECT and rejects joins/aggregates', () => {
    expect(
      inferEditableTarget('SELECT * FROM `orders`.`order_events` LIMIT 100', ''),
    ).toEqual({ schema: 'orders', table: 'order_events' });
    expect(
      inferEditableTarget('SELECT id, name FROM events WHERE id > 1;', 'orders'),
    ).toEqual({ schema: 'orders', table: 'events' });
    expect(
      inferEditableTarget('SELECT * FROM a JOIN b ON a.id = b.id', 'orders'),
    ).toBeNull();
    expect(
      inferEditableTarget('SELECT status, COUNT(*) FROM events GROUP BY status', 'orders'),
    ).toBeNull();
  });

  it('merges multiple edited cells in the same row into one UPDATE statement', () => {
    const batch = buildBatchUpdateSQL({
      schema: 'orders',
      table: 'events',
      dialect: 'mysql',
      pendingEdits: {
        0: { 1: 'beta-updated', 2: null },
        2: { 1: 'gamma-updated' },
      },
      columns: sampleResult.columns,
      rows: sampleResult.rows,
      primaryKeys: ['id'],
    });
    expect(batch.rowCount).toBe(2);
    expect(batch.cellCount).toBe(3);
    expect(batch.usedPrimaryKey).toBe(true);
    expect(batch.statements[0]).toBe(
      "UPDATE `orders`.`events`\nSET `name` = 'beta-updated', `payload` = NULL\nWHERE `id` = '10';",
    );
    expect(batch.statements[1]).toBe(
      "UPDATE `orders`.`events`\nSET `name` = 'gamma-updated'\nWHERE `id` = '100';",
    );

    const applied = applyPendingEditsToResult(sampleResult, {
      0: { 1: 'beta-updated', 2: null },
    });
    expect(applied.rows[0]).toEqual(['10', 'beta-updated', null]);
    expect(applied.rows[1]).toEqual(sampleResult.rows[1]);
  });
});

describe('ResultGrid interactions', () => {
  it('supports cell selection, column sorting and row filtering', () => {
    render(<ResultGrid result={sampleResult} />);

    // Select a cell via mouseDown
    const betaCell = screen.getByRole('cell', { name: 'beta' });
    fireEvent.mouseDown(betaCell, { button: 0 });
    fireEvent.mouseUp(window);
    expect(betaCell).toHaveAttribute('aria-selected', 'true');

    // Click id header to sort ascending numerically (2, 10, 100)
    const idSortBtn = screen.getByRole('button', { name: /^id\s*BIGINT$/i });
    fireEvent.click(idSortBtn);
    const cellsAfterAsc = screen
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[0].textContent);
    expect(cellsAfterAsc).toEqual(['2', '10', '100']);

    // Click again to sort descending (100, 10, 2)
    fireEvent.click(idSortBtn);
    const cellsAfterDesc = screen
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[0].textContent);
    expect(cellsAfterDesc).toEqual(['100', '10', '2']);
  });

  it('supports clicking # corner to select all and mouse drag-selecting multiple cells', () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    render(<ResultGrid result={sampleResult} />);

    // Click # corner to select all cells
    const selectAllBtn = screen.getByRole('columnheader', { name: '全选所有行' });
    fireEvent.click(selectAllBtn);
    expect(
      screen
        .getAllByRole('cell')
        .every((cell) => cell.getAttribute('aria-selected') === 'true'),
    ).toBe(true);

    // Drag-select a 2x2 block: (row 0, col 0) -> (row 1, col 1)
    const startCell = screen.getByRole('cell', { name: '10' });
    const endCell = screen.getByRole('cell', { name: 'alpha' });
    fireEvent.mouseDown(startCell, { button: 0 });
    fireEvent.mouseEnter(endCell);
    fireEvent.mouseUp(window);

    expect(startCell).toHaveAttribute('aria-selected', 'true');
    expect(endCell).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('cell', { name: 'gamma' })).toHaveAttribute(
      'aria-selected',
      'false',
    );

    // Ctrl+C copies the 2x2 matrix as TSV
    fireEvent.keyDown(endCell, { key: 'c', ctrlKey: true });
    expect(writeText).toHaveBeenCalledWith('10\tbeta\n2\talpha');
  });

  it('filters rows when filterText is provided', () => {
    const onFiltered = vi.fn();
    render(
      <ResultGrid
        result={sampleResult}
        filterText="alpha"
        onFilteredCountChange={onFiltered}
      />,
    );
    expect(screen.getAllByRole('row')).toHaveLength(2); // header + 1 matched row
    expect(onFiltered).toHaveBeenCalledWith(1, 3);
  });

  it('opens centered CellDetailDialog on double click with JSON formatting toggle', () => {
    render(<ResultGrid result={sampleResult} />);
    const jsonCell = screen.getByRole('cell', { name: '{"a":1,"b":2}' });
    fireEvent.doubleClick(jsonCell);

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('payload')).toBeTruthy();
    const preview = within(dialog).getByLabelText('单元格完整内容');
    // JSON pretty-print is enabled by default for valid JSON
    expect(preview.textContent).toBe('{\n  "a": 1,\n  "b": 2\n}');

    // Toggle JSON formatting off
    fireEvent.click(within(dialog).getByRole('button', { name: /JSON 格式化/ }));
    expect(preview.textContent).toBe('{"a":1,"b":2}');
  });

  it('opens context menu and copies row as JSON', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    render(
      <ResultGrid
        result={sampleResult}
        target={{ schema: 'orders', table: 'events', dialect: 'mysql' }}
      />,
    );
    const betaCell = screen.getByRole('cell', { name: 'beta' });
    fireEvent.contextMenu(betaCell);

    fireEvent.click(screen.getByRole('menuitem', { name: '复制整行 (JSON)' }));
    expect(writeText).toHaveBeenCalledWith(
      JSON.stringify(
        { id: '10', name: 'beta', payload: '{"a":1,"b":2}' },
        null,
        2,
      ),
    );
  });

  it('supports double-click staging edits, blur staging, dirty highlight and expand icon', () => {
    const onStageCellEdit = vi.fn();
    render(
      <ResultGrid
        result={sampleResult}
        canEdit={true}
        pendingEdits={{ 0: { 1: 'beta-staged' } }}
        onStageCellEdit={onStageCellEdit}
      />,
    );
    const stagedCell = screen.getByRole('cell', { name: 'beta-staged' });
    expect(stagedCell.className).toContain('is-cell-dirty');

    // Expand button inside cell opens detail dialog even in editable mode
    fireEvent.click(within(stagedCell).getByRole('button', { name: /查看 name 详情/ }));
    expect(screen.getByRole('dialog')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '关闭弹窗' }));

    // Double click enters inline editing directly and Enter stages edit
    fireEvent.doubleClick(stagedCell);
    const input = screen.getByLabelText('编辑 name');
    fireEvent.change(input, { target: { value: 'beta-2' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onStageCellEdit).toHaveBeenCalledWith(0, 1, 'beta-2');

    // Blur after editing also stages edit
    fireEvent.doubleClick(stagedCell);
    const input2 = screen.getByLabelText('编辑 name');
    fireEvent.change(input2, { target: { value: 'beta-blur' } });
    fireEvent.blur(input2);
    expect(onStageCellEdit).toHaveBeenLastCalledWith(0, 1, 'beta-blur');
  });
});
