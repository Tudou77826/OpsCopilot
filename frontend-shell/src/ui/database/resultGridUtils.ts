import type {
  DatabaseColumn,
  DatabaseResultSet,
  DatabaseStructure,
} from './types';

export type SQLDialect = 'mysql' | 'standard';

export function sqlQuoteIdent(name: string, dialect: SQLDialect = 'mysql') {
  if (dialect === 'standard') {
    return `"${name.replace(/"/g, '""')}"`;
  }
  return `\`${name.replace(/`/g, '``')}\``;
}

export function sqlLiteral(
  value: string | null,
  dialect: SQLDialect = 'mysql',
): string {
  if (value === null) return 'NULL';
  if (dialect === 'mysql') {
    return `'${value
      .replace(/\\/g, '\\\\')
      .replace(/'/g, "''")
      .replace(/\0/g, '\\0')
      .replace(/\r/g, '\\r')
      .replace(/\n/g, '\\n')}'`;
  }
  return `'${value.replace(/'/g, "''")}'`;
}

export function formatRowCSV(row: (string | null)[]): string {
  const escape = (v: string | null) =>
    v === null ? '' : `"${v.replace(/"/g, '""')}"`;
  return row.map(escape).join(',');
}

export function formatRowTSV(row: (string | null)[]): string {
  return row
    .map((v) =>
      v === null ? 'NULL' : v.replace(/\t/g, ' ').replace(/\r?\n/g, ' '),
    )
    .join('\t');
}

export function formatSelectionMatrix(
  rows: (string | null)[][],
  format: 'tsv' | 'csv',
): string {
  return rows
    .map((r) => (format === 'csv' ? formatRowCSV(r) : formatRowTSV(r)))
    .join(format === 'csv' ? '\r\n' : '\n');
}

export function formatRowJSON(
  columns: DatabaseColumn[],
  row: (string | null)[],
): string {
  const record: Record<string, string | null> = {};
  columns.forEach((col, i) => {
    const key = col.name || `column_${i + 1}`;
    record[key] = row[i] ?? null;
  });
  return JSON.stringify(record, null, 2);
}

export function formatRowInsertSQL(
  columns: DatabaseColumn[],
  row: (string | null)[],
  target?: { schema?: string; table?: string; dialect?: SQLDialect },
): string {
  const dialect = target?.dialect ?? 'mysql';
  const tableName = target?.table
    ? target.schema
      ? `${sqlQuoteIdent(target.schema, dialect)}.${sqlQuoteIdent(target.table, dialect)}`
      : sqlQuoteIdent(target.table, dialect)
    : sqlQuoteIdent('table_name', dialect);
  const cols = columns.map((c) => sqlQuoteIdent(c.name, dialect)).join(', ');
  const vals = columns.map((_, i) => sqlLiteral(row[i] ?? null, dialect)).join(', ');
  return `INSERT INTO ${tableName} (${cols}) VALUES (${vals});`;
}

export function tryFormatJSON(value: string | null): string | null {
  if (value === null) return null;
  const trimmed = value.trim();
  if (
    (!trimmed.startsWith('{') || !trimmed.endsWith('}')) &&
    (!trimmed.startsWith('[') || !trimmed.endsWith(']'))
  ) {
    return null;
  }
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return null;
  }
}

const INTEGER_RE = /^-?\d+$/;
const NUMBER_RE = /^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/;

export function compareCellValues(
  a: string | null | undefined,
  b: string | null | undefined,
): number {
  const aNull = a === null || a === undefined;
  const bNull = b === null || b === undefined;
  if (aNull && bNull) return 0;
  if (aNull) return 1;
  if (bNull) return -1;

  const ta = a.trim();
  const tb = b.trim();
  if (ta !== '' && tb !== '') {
    if (INTEGER_RE.test(ta) && INTEGER_RE.test(tb)) {
      try {
        const ba = BigInt(ta);
        const bb = BigInt(tb);
        return ba < bb ? -1 : ba > bb ? 1 : 0;
      } catch {
        // fall through to numeric/string comparison
      }
    }
    if (NUMBER_RE.test(ta) && NUMBER_RE.test(tb)) {
      const na = Number(ta);
      const nb = Number(tb);
      if (Number.isFinite(na) && Number.isFinite(nb)) {
        return na < nb ? -1 : na > nb ? 1 : 0;
      }
    }
  }
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
}

export function rowMatchesFilter(
  row: (string | null)[],
  rawQuery: string,
): boolean {
  const q = rawQuery.trim().toLowerCase();
  if (!q) return true;
  return row.some((cell) => {
    if (cell === null) return 'null'.includes(q);
    return cell.toLowerCase().includes(q);
  });
}

export function extractPrimaryKeys(structure?: DatabaseStructure): string[] {
  if (!structure) return [];
  const colSet = structure.columns;
  const keyIdx = colSet.columns.findIndex(
    (c) => c.name === '键' || c.name.toUpperCase() === 'COLUMN_KEY',
  );
  const nameIdx = colSet.columns.findIndex(
    (c) => c.name === '字段' || c.name.toUpperCase() === 'COLUMN_NAME',
  );
  if (keyIdx >= 0 && nameIdx >= 0) {
    const pks = colSet.rows
      .filter((r) => (r[keyIdx] ?? '').toUpperCase() === 'PRI' && r[nameIdx])
      .map((r) => r[nameIdx] as string);
    if (pks.length > 0) return pks;
  }

  const idxSet = structure.indexes;
  const idxNamePos = idxSet.columns.findIndex(
    (c) => c.name === '索引' || c.name.toUpperCase() === 'INDEX_NAME',
  );
  const idxColPos = idxSet.columns.findIndex(
    (c) => c.name === '字段' || c.name.toUpperCase() === 'COLUMN_NAME',
  );
  const nonUniquePos = idxSet.columns.findIndex(
    (c) => c.name === '非唯一' || c.name.toUpperCase() === 'NON_UNIQUE',
  );
  if (idxNamePos >= 0 && idxColPos >= 0) {
    const primaryCols = idxSet.rows
      .filter((r) => {
        const idxName = (r[idxNamePos] ?? '').toLowerCase();
        return (
          (idxName === 'primary' ||
            idxName.endsWith('_pkey') ||
            idxName.startsWith('pk_')) &&
          r[idxColPos]
        );
      })
      .map((r) => r[idxColPos] as string);
    if (primaryCols.length > 0) return Array.from(new Set(primaryCols));

    if (nonUniquePos >= 0) {
      const uniqueGroups = new Map<string, string[]>();
      for (const r of idxSet.rows) {
        const nu = (r[nonUniquePos] ?? '').toLowerCase();
        const idxName = r[idxNamePos] ?? '';
        const colName = r[idxColPos];
        if ((nu === '0' || nu === 'false') && idxName && colName) {
          const list = uniqueGroups.get(idxName) ?? [];
          list.push(colName);
          uniqueGroups.set(idxName, list);
        }
      }
      const firstUnique = uniqueGroups.values().next().value;
      if (firstUnique && firstUnique.length > 0) {
        return Array.from(new Set(firstUnique));
      }
    }
  }

  return [];
}

export interface BuiltUpdateSQL {
  sql: string;
  usedPrimaryKey: boolean;
  whereColumns: string[];
}

export interface RowPendingChange {
  origRowIndex: number;
  changes: Record<number, string | null>;
}

export interface BuiltBatchUpdateSQL {
  sql: string;
  statements: string[];
  rowCount: number;
  cellCount: number;
  usedPrimaryKey: boolean;
}

export function buildRowUpdateSQL(params: {
  schema: string;
  table: string;
  dialect: SQLDialect;
  changes: Record<number, string | null>;
  columns: DatabaseColumn[];
  originalRow: (string | null)[];
  primaryKeys: string[];
}): BuiltUpdateSQL {
  const {
    schema,
    table,
    dialect,
    changes,
    columns,
    originalRow,
    primaryKeys,
  } = params;
  const colIndexByName = new Map<string, number>();
  columns.forEach((c, i) => colIndexByName.set(c.name, i));

  const validPKs = primaryKeys.filter((pk) => colIndexByName.has(pk));
  const usedPrimaryKey = validPKs.length > 0;
  const whereColumns = usedPrimaryKey
    ? validPKs
    : columns.map((c) => c.name);

  const qualifiedTable = schema
    ? `${sqlQuoteIdent(schema, dialect)}.${sqlQuoteIdent(table, dialect)}`
    : sqlQuoteIdent(table, dialect);

  const assignments = Object.entries(changes)
    .map(([colIdxStr, newValue]) => {
      const col = columns[Number(colIdxStr)];
      if (!col) return null;
      return `${sqlQuoteIdent(col.name, dialect)} = ${sqlLiteral(newValue, dialect)}`;
    })
    .filter((v): v is string => v !== null);

  const predicates = whereColumns.map((colName) => {
    const idx = colIndexByName.get(colName)!;
    const orig = originalRow[idx] ?? null;
    if (orig === null) {
      return `${sqlQuoteIdent(colName, dialect)} IS NULL`;
    }
    return `${sqlQuoteIdent(colName, dialect)} = ${sqlLiteral(orig, dialect)}`;
  });

  const sql = `UPDATE ${qualifiedTable}\nSET ${assignments.join(', ')}\nWHERE ${predicates.join(' AND ')};`;
  return { sql, usedPrimaryKey, whereColumns };
}

export function buildUpdateSQL(params: {
  schema: string;
  table: string;
  dialect: SQLDialect;
  column: string;
  newValue: string | null;
  columns: DatabaseColumn[];
  originalRow: (string | null)[];
  primaryKeys: string[];
}): BuiltUpdateSQL {
  const colIdx = params.columns.findIndex((c) => c.name === params.column);
  return buildRowUpdateSQL({
    schema: params.schema,
    table: params.table,
    dialect: params.dialect,
    changes: colIdx >= 0 ? { [colIdx]: params.newValue } : {},
    columns: params.columns,
    originalRow: params.originalRow,
    primaryKeys: params.primaryKeys,
  });
}

export function buildBatchUpdateSQL(params: {
  schema: string;
  table: string;
  dialect: SQLDialect;
  pendingEdits: Record<number, Record<number, string | null>>;
  columns: DatabaseColumn[];
  rows: (string | null)[][];
  primaryKeys: string[];
}): BuiltBatchUpdateSQL {
  const statements: string[] = [];
  let cellCount = 0;
  let usedPrimaryKey = true;

  const rowIndices = Object.keys(params.pendingEdits)
    .map(Number)
    .sort((a, b) => a - b);

  for (const origRowIndex of rowIndices) {
    const changes = params.pendingEdits[origRowIndex];
    const origRow = params.rows[origRowIndex];
    if (!changes || !origRow) continue;
    const keys = Object.keys(changes);
    if (keys.length === 0) continue;
    cellCount += keys.length;
    const built = buildRowUpdateSQL({
      schema: params.schema,
      table: params.table,
      dialect: params.dialect,
      changes,
      columns: params.columns,
      originalRow: origRow,
      primaryKeys: params.primaryKeys,
    });
    if (!built.usedPrimaryKey) usedPrimaryKey = false;
    statements.push(built.sql);
  }

  return {
    sql: statements.join('\n\n'),
    statements,
    rowCount: statements.length,
    cellCount,
    usedPrimaryKey,
  };
}

export function applyPendingEditsToResult(
  result: DatabaseResultSet,
  pendingEdits: Record<number, Record<number, string | null>>,
): DatabaseResultSet {
  return {
    ...result,
    rows: result.rows.map((row, rIdx) => {
      const rowChanges = pendingEdits[rIdx];
      if (!rowChanges) return row;
      return row.map((cell, cIdx) =>
        Object.prototype.hasOwnProperty.call(rowChanges, cIdx)
          ? rowChanges[cIdx]
          : cell,
      );
    }),
  };
}

export function cloneResultWithUpdatedCell(
  result: DatabaseResultSet,
  origRowIndex: number,
  colIndex: number,
  newValue: string | null,
): DatabaseResultSet {
  return applyPendingEditsToResult(result, {
    [origRowIndex]: { [colIndex]: newValue },
  });
}

function unquoteIdent(raw: string): string {
  const trimmed = raw.trim();
  if (
    (trimmed.startsWith('`') && trimmed.endsWith('`')) ||
    (trimmed.startsWith('"') && trimmed.endsWith('"'))
  ) {
    const q = trimmed[0];
    return trimmed.slice(1, -1).replace(new RegExp(`${q}${q}`, 'g'), q);
  }
  return trimmed;
}

/**
 * Infers the single target table from a simple SELECT statement so users can
 * also inline-edit single-table query results (rejecting JOIN/UNION/GROUP BY).
 */
export function inferEditableTarget(
  statement: string | undefined,
  defaultSchema: string,
): { schema: string; table: string } | null {
  if (!statement) return null;
  const cleaned = statement
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .replace(/--[^\n]*/g, ' ')
    .trim();
  if (!/^select\b/i.test(cleaned)) return null;
  if (/\b(join|union|group\s+by|having|distinct)\b/i.test(cleaned)) return null;

  const fromMatch = cleaned.match(
    /\bfrom\s+((?:`[^`]+`|"[^"]+"|[a-zA-Z0-9_$]+)(?:\s*\.\s*(?:`[^`]+`|"[^"]+"|[a-zA-Z0-9_$]+))?)(?:\s+(?:as\s+)?(?:`[^`]+`|"[^"]+"|[a-zA-Z0-9_$]+))?\s*(?:$|;|where\b|order\b|limit\b|offset\b|fetch\b)/i,
  );
  if (!fromMatch) return null;

  const rawTarget = fromMatch[1];
  const parts = rawTarget.split(/\s*\.\s*/).map(unquoteIdent);
  if (parts.length === 2 && parts[0] && parts[1]) {
    return { schema: parts[0], table: parts[1] };
  }
  if (parts.length === 1 && parts[0]) {
    return { schema: defaultSchema, table: parts[0] };
  }
  return null;
}
