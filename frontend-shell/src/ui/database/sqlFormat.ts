/**
 * Lightweight SQL formatter preserving string literals, quoted identifiers,
 * and comments while breaking top-level clauses cleanly.
 */

interface Token {
  type: 'word' | 'string' | 'comment' | 'punct' | 'space';
  raw: string;
  upper?: string;
}

const MAJOR_CLAUSES = new Set([
  'SELECT',
  'FROM',
  'WHERE',
  'SET',
  'VALUES',
  'HAVING',
  'LIMIT',
  'OFFSET',
  'UPDATE',
  'DELETE',
  'INSERT',
  'UNION',
  'EXCEPT',
  'INTERSECT',
  'RETURNING',
]);

const TWO_WORD_CLAUSES = new Set([
  'ORDER BY',
  'GROUP BY',
  'LEFT JOIN',
  'RIGHT JOIN',
  'INNER JOIN',
  'OUTER JOIN',
  'CROSS JOIN',
  'FULL JOIN',
  'INSERT INTO',
  'DELETE FROM',
  'UNION ALL',
  'ON DUPLICATE',
]);

const THREE_WORD_CLAUSES = new Set([
  'LEFT OUTER JOIN',
  'RIGHT OUTER JOIN',
  'FULL OUTER JOIN',
]);

const INDENT_CLAUSES = new Set(['AND', 'OR']);

const SQL_KEYWORDS = new Set([
  ...MAJOR_CLAUSES,
  ...INDENT_CLAUSES,
  'AS',
  'ASC',
  'BETWEEN',
  'BY',
  'CASE',
  'CROSS',
  'DESC',
  'DISTINCT',
  'ELSE',
  'END',
  'EXISTS',
  'FULL',
  'GROUP',
  'IN',
  'INNER',
  'INTO',
  'IS',
  'JOIN',
  'LEFT',
  'LIKE',
  'NOT',
  'NULL',
  'ON',
  'ORDER',
  'OUTER',
  'RIGHT',
  'THEN',
  'WHEN',
  'ALL',
]);

function tokenize(sql: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < sql.length) {
    const ch = sql[i];
    if (/\s/.test(ch)) {
      let j = i + 1;
      while (j < sql.length && /\s/.test(sql[j])) j++;
      tokens.push({ type: 'space', raw: sql.slice(i, j) });
      i = j;
      continue;
    }
    // Line comment: -- ... or # ...
    if ((ch === '-' && sql[i + 1] === '-') || ch === '#') {
      let j = sql.indexOf('\n', i);
      if (j < 0) j = sql.length;
      tokens.push({ type: 'comment', raw: sql.slice(i, j).trimEnd() });
      i = j;
      continue;
    }
    // Block comment: /* ... */
    if (ch === '/' && sql[i + 1] === '*') {
      let j = sql.indexOf('*/', i + 2);
      j = j < 0 ? sql.length : j + 2;
      tokens.push({ type: 'comment', raw: sql.slice(i, j) });
      i = j;
      continue;
    }
    // Quoted strings / identifiers
    if (ch === "'" || ch === '"' || ch === '`') {
      const quote = ch;
      let j = i + 1;
      while (j < sql.length) {
        if (sql[j] === '\\' && quote !== '`') {
          j += 2;
          continue;
        }
        if (sql[j] === quote) {
          if (sql[j + 1] === quote) {
            j += 2;
            continue;
          }
          j++;
          break;
        }
        j++;
      }
      tokens.push({ type: 'string', raw: sql.slice(i, j) });
      i = j;
      continue;
    }
    // Word / identifier / number
    if (/[a-zA-Z0-9_$]/.test(ch)) {
      let j = i + 1;
      while (j < sql.length && /[a-zA-Z0-9_$]/.test(sql[j])) j++;
      const raw = sql.slice(i, j);
      tokens.push({ type: 'word', raw, upper: raw.toUpperCase() });
      i = j;
      continue;
    }
    // Two-char operators
    const pair = sql.slice(i, i + 2);
    if (['<=', '>=', '<>', '!=', '||', '::'].includes(pair)) {
      tokens.push({ type: 'punct', raw: pair });
      i += 2;
      continue;
    }
    tokens.push({ type: 'punct', raw: ch });
    i++;
  }
  return tokens;
}

export function formatSQL(input: string): string {
  const rawTokens = tokenize(input).filter((t) => t.type !== 'space');
  if (rawTokens.length === 0) return input;

  let out = '';
  let depth = 0;
  let needSpace = false;
  let inSelectList = false;

  const indent = (extra = 0) => '  '.repeat(Math.max(0, depth + extra));
  const newline = (extra = 0) => {
    if (extra === 0 && out.endsWith('\n\n')) {
      needSpace = false;
      return;
    }
    out = out.trimEnd() + '\n' + indent(extra);
    needSpace = false;
  };

  for (let i = 0; i < rawTokens.length; i++) {
    const t = rawTokens[i];
    const next = rawTokens[i + 1];
    const next2 = rawTokens[i + 2];

    if (t.type === 'comment') {
      if (out.length > 0 && !out.endsWith('\n')) out += '\n' + indent();
      out += t.raw;
      newline();
      continue;
    }

    if (t.type === 'punct') {
      if (t.raw === ';') {
        out = out.trimEnd() + ';';
        inSelectList = false;
        if (next) {
          out += '\n\n' + indent();
          needSpace = false;
        }
        continue;
      }
      if (t.raw === ',') {
        out = out.trimEnd() + ',';
        if (depth === 0 && inSelectList) {
          newline(1);
        } else {
          needSpace = true;
        }
        continue;
      }
      if (t.raw === '(') {
        if (needSpace && !out.endsWith('(') && !out.endsWith('.')) out += ' ';
        out += '(';
        depth++;
        needSpace = false;
        continue;
      }
      if (t.raw === ')') {
        depth = Math.max(0, depth - 1);
        out = out.trimEnd() + ')';
        needSpace = true;
        continue;
      }
      if (t.raw === '.') {
        out = out.trimEnd() + '.';
        needSpace = false;
        continue;
      }
      if (needSpace && !out.endsWith('(') && !out.endsWith('.')) out += ' ';
      out += t.raw;
      needSpace = true;
      continue;
    }

    if (t.type === 'word' && depth === 0) {
      const three =
        next?.type === 'word' && next2?.type === 'word'
          ? `${t.upper} ${next.upper} ${next2.upper}`
          : '';
      if (THREE_WORD_CLAUSES.has(three)) {
        inSelectList = false;
        if (out.trim().length > 0) newline();
        out += three;
        needSpace = true;
        i += 2;
        continue;
      }
      const two =
        next?.type === 'word' ? `${t.upper} ${next.upper}` : '';
      if (TWO_WORD_CLAUSES.has(two)) {
        inSelectList = false;
        if (out.trim().length > 0) newline();
        out += two;
        needSpace = true;
        i += 1;
        continue;
      }
      if (t.upper === 'JOIN' || MAJOR_CLAUSES.has(t.upper!)) {
        inSelectList = t.upper === 'SELECT' || t.upper === 'SET';
        if (out.trim().length > 0) newline();
        out += t.upper!;
        if (inSelectList && next && next.raw !== '*') {
          newline(1);
        } else {
          needSpace = true;
        }
        continue;
      }
      if (INDENT_CLAUSES.has(t.upper!)) {
        newline(1);
        out += t.upper!;
        needSpace = true;
        continue;
      }
    }

    if (needSpace && !out.endsWith('(') && !out.endsWith('.')) {
      out += ' ';
    }
    if (t.type === 'word' && SQL_KEYWORDS.has(t.upper!)) {
      out += t.upper!;
    } else {
      out += t.raw;
    }
    needSpace = true;
  }

  return out.trimEnd();
}
