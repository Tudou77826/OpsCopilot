import { describe, expect, it } from 'vitest';
import { formatSQL } from './sqlFormat';

describe('formatSQL', () => {
  it('formats SELECT/WHERE/AND/ORDER BY/LIMIT and preserves string literals', () => {
    const input =
      "select id, order_no, status from orders.order_events where status = 'from where' and amount > 10 order by id desc limit 20;";
    expect(formatSQL(input)).toBe(
      [
        'SELECT',
        '  id,',
        '  order_no,',
        '  status',
        'FROM orders.order_events',
        "WHERE status = 'from where'",
        '  AND amount > 10',
        'ORDER BY id DESC',
        'LIMIT 20;',
      ].join('\n'),
    );
  });

  it('separates multiple statements with a blank line', () => {
    expect(formatSQL('select 1; select 2;')).toBe('SELECT\n  1;\n\nSELECT\n  2;');
  });
});
