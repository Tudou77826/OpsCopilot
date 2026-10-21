import React from 'react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { DatabaseWorkspace } from './DatabaseWorkspace';
import type { DatabaseProfile, DatabaseRuntime } from './types';
import { resultCSV } from './ResultGrid';

vi.mock('./SQLEditor', () => ({
  SQLEditor: React.forwardRef(
    (
      {
        value,
        onChange,
        onRun,
        onRunScript,
      }: {
        value: string;
        onChange(v: string): void;
        onRun(): void;
        onRunScript(): void;
      },
      ref,
    ) => {
      const element = React.useRef<HTMLTextAreaElement>(null);
      React.useImperativeHandle(ref, () => ({
        value: () => value,
        selection: () =>
          value.slice(
            element.current?.selectionStart ?? 0,
            element.current?.selectionEnd ?? 0,
          ),
        cursorOffset: () => element.current?.selectionStart ?? 0,
        focus: vi.fn(),
      }));
      return (
        <textarea
          ref={element}
          aria-label="SQL 编辑器"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.ctrlKey && e.key === 'Enter') {
              e.preventDefault();
              if (e.shiftKey) onRunScript();
              else onRun();
            }
          }}
        />
      );
    },
  ),
}));
vi.mock('./ResultGrid', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ResultGrid: ({ result }: { result: { rows: unknown[] } }) => (
    <div aria-label="查询结果">{JSON.stringify(result.rows)}</div>
  ),
}));
const profile: DatabaseProfile = {
  id: 'p1',
  revision: 1,
  name: '订单库',
  host: 'localhost',
  port: 3306,
  user: 'ops',
  environment: '测试',
  engine: 'mysql',
  database: 'orders',
  gatewayId: '',
  tls: 'off',
  readOnly: true,
  passwordSet: true,
};
function runtime(): DatabaseRuntime {
  return {
    jdbcStatus: vi.fn(async () => ({
      available: true,
      message: '',
      drivers: [{id:'mysql-jdbc',name:'MySQL Connector/J',className:'com.mysql.cj.jdbc.Driver',urlTemplate:'jdbc:mysql://{host}:{port}/{database}',dialect:'mysql',version:'9.7',jars:[]}],
    })),
    importJDBC: vi.fn(),
    deleteJDBC: vi.fn(),
    chooseJDBCJars: vi.fn(),
    profiles: vi.fn(async () => [profile]),
    groups: vi.fn(async () => []),
    createGroup: vi.fn(),
    renameGroup: vi.fn(),
    deleteGroup: vi.fn(),
    save: vi.fn(),
    test: vi.fn(),
    remove: vi.fn(),
    gateways: vi.fn(async () => []),
    connect: vi.fn(async () => ({
      id: 's1',
      profileId: profile.id,
      version: '8.4',
      database: 'orders',
      readOnly: true,
    })),
    disconnect: vi.fn(async () => {}),
    schemas: vi.fn(async () => ['orders']),
    tables: vi.fn(async () => [
      { name: 'events', type: 'BASE TABLE', comment: '' },
    ]),
    structure: vi.fn(),
    generateSQL: vi.fn(
      async () => 'SELECT * FROM `orders`.`events` LIMIT 100;',
    ),
    browse: vi.fn(),
    query: vi.fn(async () => ({
      sets: [
        {
          statement: 'SELECT 1',
          columns: [{ name: 'id', type: 'BIGINT' }],
          rows: [['9007199254740993']],
          affectedRows: 0,
          truncated: false,
        },
      ],
      durationMs: 10,
      error: '',
      cancelled: false,
      disconnected: false,
    })),
    cancel: vi.fn(async () => true),
  };
}
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (
    this: HTMLDialogElement,
  ) {
    this.setAttribute('open', '');
  });
  HTMLDialogElement.prototype.close = vi.fn();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
async function open(host: DatabaseRuntime) {
  render(<DatabaseWorkspace runtime={host} />);
  fireEvent.doubleClick(await screen.findByText('订单库'));
  await screen.findByLabelText('SQL 编辑器');
}
describe('database workbench', () => {
  it('separates cursor SQL, selected SQL and whole-script execution including shortcuts', async () => {
    const host = runtime();
    await open(host);
    const sql = 'SELECT 1;\nSELECT 2;';
    const area = screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement;
    fireEvent.change(area, { target: { value: sql } });
    area.setSelectionRange(13, 13);
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await waitFor(() =>
      expect(host.query).toHaveBeenLastCalledWith(
        expect.objectContaining({ sql, mode: 'statement', cursorOffset: 13 }),
      ),
    );
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '执行 SQL' })).toBeEnabled(),
    );
    area.setSelectionRange(10, sql.length);
    fireEvent.keyDown(area, { key: 'Enter', ctrlKey: true });
    await waitFor(() =>
      expect(host.query).toHaveBeenLastCalledWith(
        expect.objectContaining({ sql: 'SELECT 2;', mode: 'selection' }),
      ),
    );
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '执行脚本' })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole('button', { name: '执行脚本' }));
    await waitFor(() =>
      expect(host.query).toHaveBeenLastCalledWith(
        expect.objectContaining({ sql, mode: 'script' }),
      ),
    );
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '执行脚本' })).toBeEnabled(),
    );
    fireEvent.keyDown(area, { key: 'Enter', ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(host.query).toHaveBeenCalledTimes(4));
    expect(vi.mocked(host.query).mock.calls[3][0]).toMatchObject({
      sql,
      mode: 'script',
    });
  });
  it('renames and copies an inactive query without executing or changing its connection', async () => {
    const host = runtime();
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT retained_sql' },
    });
    fireEvent.click(screen.getByRole('button', { name: '新建 SQL 查询' }));
    fireEvent.contextMenu(
      screen.getByRole('button', { name: '查询 1 · 订单库' }),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '重命名查询…' }));
    fireEvent.change(screen.getByLabelText('查询名称'), {
      target: { value: '订单核对' },
    });
    fireEvent.click(
      within(screen.getByRole('dialog', { name: '重命名查询' })).getByRole(
        'button',
        { name: '保存' },
      ),
    );
    expect(
      screen.getByRole('button', { name: '订单核对 · 订单库' }),
    ).toBeTruthy();
    expect(screen.getByLabelText('SQL 编辑器')).toHaveProperty('value', '');
    fireEvent.keyDown(
      screen.getByRole('button', { name: '订单核对 · 订单库' }),
      { key: 'F10', shiftKey: true },
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '复制查询' }));
    expect(screen.getByLabelText('SQL 编辑器')).toHaveProperty(
      'value',
      'SELECT retained_sql',
    );
    expect(
      screen.getByRole('button', { name: '订单核对 副本 · 订单库' }),
    ).toBeTruthy();
    expect(host.query).not.toHaveBeenCalled();
  });
  it('closes tabs to the right of the context target and leaves a running query open', async () => {
    const host = runtime();
    let finish!: (value: Awaited<ReturnType<DatabaseRuntime['query']>>) => void;
    host.query = vi.fn(
      () =>
        new Promise<Awaited<ReturnType<DatabaseRuntime['query']>>>((resolve) => {
          finish = resolve;
        }),
    );
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT slow_query' },
    });
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await waitFor(() => expect(host.query).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole('button', { name: '新建 SQL 查询' }));
    fireEvent.click(screen.getByRole('button', { name: '新建 SQL 查询' }));
    fireEvent.contextMenu(
      screen.getByRole('button', { name: '查询 2 · 订单库' }),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '关闭右侧' }));
    expect(
      screen.queryByRole('button', { name: '查询 3 · 订单库' }),
    ).toBeNull();
    expect(
      screen.getByRole('button', { name: '查询 2 · 订单库' }),
    ).toBeTruthy();
    fireEvent.contextMenu(
      screen.getByRole('button', { name: '查询 1 · 订单库' }),
    );
    expect(screen.getByRole('menuitem', { name: '关闭当前' })).toBeDisabled();
    fireEvent.click(screen.getByRole('menuitem', { name: '关闭全部' }));
    expect(
      screen.queryByRole('button', { name: '查询 2 · 订单库' }),
    ).toBeNull();
    expect(
      screen.getByRole('button', { name: '查询 1 · 订单库' }),
    ).toBeTruthy();
    finish({
      sets: [],
      durationMs: 1,
      error: '',
      cancelled: false,
      disconnected: false,
    });
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: '关闭查询 1 · 订单库' }),
      ).toBeEnabled(),
    );
    fireEvent.contextMenu(
      screen.getByRole('button', { name: '查询 1 · 订单库' }),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '关闭全部' }));
    expect(
      screen.queryByRole('button', { name: '查询 1 · 订单库' }),
    ).toBeNull();
    expect(host.cancel).not.toHaveBeenCalled();
  });
  it('shows and copies every line of connection errors in the main area, then retries successfully', async () => {
    const host = runtime();
    const message =
      'JDBC 操作失败（SQLState 08001 代码 0）\n建立连接失败\njava.net.ConnectException: Connection refused\n' +
      '驱动诊断详情\n'.repeat(60) +
      '最后一行原因';
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
    vi.mocked(host.connect).mockRejectedValueOnce(new Error(message));
    render(<DatabaseWorkspace runtime={host} />);
    fireEvent.doubleClick(await screen.findByText('订单库'));
    const panel = await screen.findByRole('region', {
      name: '数据库连接失败详情',
    });
    expect(within(panel).getByRole('alert').textContent).toContain(message);
    expect(within(panel).getByText('localhost:3306')).toBeTruthy();
    fireEvent.click(
      within(panel).getByRole('button', { name: '复制错误详情' }),
    );
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(message));
    fireEvent.click(within(panel).getByRole('button', { name: '修改连接' }));
    await screen.findByRole('dialog', { name: '编辑连接' });
    fireEvent.click(screen.getByRole('button', { name: '取消' }));
    fireEvent.click(within(panel).getByRole('button', { name: '重试连接' }));
    await screen.findByLabelText('SQL 编辑器');
    expect(
      screen.queryByRole('region', { name: '数据库连接失败详情' }),
    ).toBeNull();
    expect(
      screen.queryByRole('button', { name: '连接失败 · 订单库' }),
    ).toBeNull();
    expect(host.connect).toHaveBeenCalledTimes(2);
  });
  it('keeps failures with their own connection and preserves queries when another connection fails', async () => {
    const host = runtime();
    host.profiles = vi.fn(async () => [
      profile,
      { ...profile, id: 'p2', name: '库存库' },
    ]);
    const successfulConnect = host.connect;
    host.connect = vi.fn((id) =>
      id === 'p2'
        ? Promise.reject(new Error('库存库不可达'))
        : successfulConnect(id),
    );
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT preserved_draft' },
    });
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await screen.findByText(/9007199254740993/);
    fireEvent.doubleClick(
      within(screen.getByLabelText('数据库连接与对象')).getByText('库存库'),
    );
    await screen.findByRole('region', { name: '数据库连接失败详情' });
    expect(screen.getByRole('alert').textContent).toBe('库存库不可达');
    fireEvent.click(screen.getByRole('button', { name: '查询 1 · 订单库' }));
    expect(screen.getByLabelText('SQL 编辑器')).toHaveProperty(
      'value',
      'SELECT preserved_draft',
    );
    expect(screen.getByLabelText('查询结果').textContent).toContain(
      '9007199254740993',
    );
    expect(screen.queryByRole('alert')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: '连接失败 · 库存库' }));
    expect(screen.getByRole('alert').textContent).toBe('库存库不可达');
    expect(host.connect).toHaveBeenCalledTimes(2);
  });
  it('identifies metadata initialization failures and closes the partial session', async () => {
    const host = runtime();
    host.schemas = vi.fn(async () => {
      throw new Error('permission denied: list schemas');
    });
    render(<DatabaseWorkspace runtime={host} />);
    fireEvent.doubleClick(await screen.findByText('订单库'));
    const panel = await screen.findByRole('region', {
      name: '数据库连接失败详情',
    });
    expect(within(panel).getByText('读取数据库列表')).toBeTruthy();
    expect(within(panel).getByRole('alert').textContent).toContain(
      'permission denied: list schemas',
    );
    expect(host.disconnect).toHaveBeenCalledWith('s1');
  });
  it('does not let a late retry replace a query the user returned to', async () => {
    const host = runtime();
    let finish!: (
      session: Awaited<ReturnType<DatabaseRuntime['connect']>>,
    ) => void;
    host.profiles = vi.fn(async () => [
      profile,
      { ...profile, id: 'p2', name: '库存库' },
    ]);
    vi.mocked(host.connect)
      .mockImplementationOnce(async () => ({
        id: 's1',
        profileId: 'p1',
        version: '8.4',
        database: 'orders',
        readOnly: true,
      }))
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      );
    await open(host);
    fireEvent.doubleClick(
      within(screen.getByLabelText('数据库连接与对象')).getByText('库存库'),
    );
    fireEvent.click(screen.getByRole('button', { name: '查询 1 · 订单库' }));
    finish({
      id: 's2',
      profileId: 'p2',
      version: '8.4',
      database: 'orders',
      readOnly: true,
    });
    await waitFor(() => expect(host.schemas).toHaveBeenCalledWith('s2'));
    await waitFor(() =>
      expect(
        within(screen.getByLabelText('数据库连接与对象')).getAllByRole(
          'button',
          { name: 'events' },
        ),
      ).toHaveLength(2),
    );
    expect(
      screen.queryByRole('button', { name: '查询 1 · 库存库' }),
    ).toBeNull();
    expect(screen.getByLabelText('SQL 编辑器')).toBeTruthy();
  });
  it('binds execution to the selected connection/schema and preserves exact cell values', async () => {
    const host = runtime();
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT id FROM events' },
    });
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await screen.findByText(/9007199254740993/);
    expect(host.query).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: 's1',
        database: 'orders',
        sql: 'SELECT id FROM events',
        limit: 1000,
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: '新建 SQL 查询' }));
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT 2' },
    });
    fireEvent.click(screen.getByRole('button', { name: '查询 1 · 订单库' }));
    expect(
      (screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement).value,
    ).toBe('SELECT id FROM events');
    expect(screen.getByLabelText('查询结果').textContent).toContain(
      '9007199254740993',
    );
  });
  it('keeps drafts across reconnect and invalidates the session after cancellation', async () => {
    const host = runtime();
    let resolve!: (
      value: Awaited<ReturnType<DatabaseRuntime['query']>>,
    ) => void;
    host.query = vi.fn(
      () =>
        new Promise<Awaited<ReturnType<DatabaseRuntime['query']>>>((r) => {
          resolve = r;
        }),
    );
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT SLEEP(20)' },
    });
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await waitFor(() => expect(host.query).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole('button', { name: '停止' }));
    expect(host.cancel).toHaveBeenCalledOnce();
    resolve({
      sets: [],
      durationMs: 12,
      error: '查询已停止',
      cancelled: true,
      disconnected: true,
    });
    await screen.findByRole('heading', { name: '订单库' });
    fireEvent.click(
      screen.getAllByRole('button', { name: '连接数据库' }).at(-1)!,
    );
    await screen.findByLabelText('SQL 编辑器');
    expect(
      (screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement).value,
    ).toBe('SELECT SLEEP(20)');
  });
  it('closes owned sessions when the workspace unmounts', async () => {
    const host = runtime();
    await open(host);
    cleanup();
    expect(host.disconnect).toHaveBeenCalledWith('s1');
  });
  it('loads the default object tree after connecting and delegates table paging to the backend', async () => {
    const host = runtime();
    host.browse = vi.fn(async () => ({
      sets: [
        {
          statement: '',
          columns: [{ name: 'id', type: 'INT' }],
          rows: [['1']],
          affectedRows: 0,
          truncated: true,
        },
      ],
      durationMs: 1,
      error: '',
      cancelled: false,
      disconnected: false,
    }));
    await open(host);
    fireEvent.doubleClick(
      await screen.findByRole('button', { name: 'events' }),
    );
    await screen.findByText(/\[\["1"\]\]/);
    expect(host.browse).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: 's1',
        database: 'orders',
        table: 'events',
        offset: 0,
        limit: 100,
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: '下一页' }));
    await waitFor(() =>
      expect(host.browse).toHaveBeenLastCalledWith(
        expect.objectContaining({ offset: 100 }),
      ),
    );
    expect(host.query).not.toHaveBeenCalled();
  });
  it('keeps both connection trees and never retargets a query when selecting another connection', async () => {
    const host = runtime();
    const second = { ...profile, id: 'p2', name: '库存库', group: '业务系统' };
    host.profiles = vi.fn(async () => [profile, second]);
    host.connect = vi.fn(async (id) => ({
      id: 'session-' + id,
      profileId: id,
      version: '8.4',
      database: 'orders',
      readOnly: true,
    }));
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT order_id FROM orders' },
    });
    fireEvent.doubleClick(screen.getByText('库存库'));
    await screen.findByRole('button', { name: '查询 1 · 库存库' });
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT item_id FROM stock' },
    });
    const tree = screen.getByLabelText('数据库连接与对象');
    expect(
      within(tree).getAllByRole('button', { name: 'events' }),
    ).toHaveLength(2);
    fireEvent.click(within(tree).getByText('订单库'));
    expect(
      (screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement).value,
    ).toBe('SELECT item_id FROM stock');
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await waitFor(() =>
      expect(host.query).toHaveBeenLastCalledWith(
        expect.objectContaining({
          sessionId: 'session-p2',
          sql: 'SELECT item_id FROM stock',
        }),
      ),
    );
    fireEvent.click(screen.getByRole('button', { name: '查询 1 · 订单库' }));
    expect(
      (screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement).value,
    ).toBe('SELECT order_id FROM orders');
    fireEvent.click(screen.getByRole('button', { name: '执行 SQL' }));
    await waitFor(() =>
      expect(host.query).toHaveBeenLastCalledWith(
        expect.objectContaining({
          sessionId: 'session-p1',
          sql: 'SELECT order_id FROM orders',
        }),
      ),
    );
  });
  it('generates a statement for the right-clicked connection without executing it', async () => {
    const host = runtime();
    await open(host);
    const table = await screen.findByRole('button', { name: 'events' });
    fireEvent.contextMenu(table);
    fireEvent.click(screen.getByRole('menuitem', { name: '生成 UPDATE' }));
    await waitFor(() =>
      expect(host.generateSQL).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: 's1',
          database: 'orders',
          table: 'events',
          kind: 'update',
        }),
      ),
    );
    await screen.findByRole('button', { name: '查询 2 · 订单库' });
    expect(host.query).not.toHaveBeenCalled();
    expect(host.browse).not.toHaveBeenCalled();
    expect(
      (screen.getByLabelText('SQL 编辑器') as HTMLTextAreaElement).value,
    ).toContain('SELECT * FROM');
  });
  it('keeps identically named objects from different connections in separate tabs', async () => {
    const host = runtime();
    host.profiles = vi.fn(async () => [
      profile,
      { ...profile, id: 'p2', name: '库存库' },
    ]);
    host.connect = vi.fn(async (id) => ({
      id: 'session-' + id,
      profileId: id,
      version: '8.4',
      database: 'orders',
      readOnly: true,
    }));
    host.browse = vi.fn(async (request) => ({
      sets: [
        {
          statement: '',
          columns: [{ name: 'id', type: 'TEXT' }],
          rows: [[request.sessionId]],
          affectedRows: 0,
          truncated: false,
        },
      ],
      durationMs: 1,
      error: '',
      cancelled: false,
      disconnected: false,
    }));
    await open(host);
    fireEvent.doubleClick(
      within(screen.getByLabelText('数据库连接与对象')).getByRole('button', {
        name: 'events',
      }),
    );
    await screen.findByText(/session-p1/);
    fireEvent.doubleClick(
      within(screen.getByLabelText('数据库连接与对象')).getByText('库存库'),
    );
    await screen.findByRole('button', { name: '查询 1 · 库存库' });
    const tables = within(
      screen.getByLabelText('数据库连接与对象'),
    ).getAllByRole('button', { name: 'events' });
    fireEvent.doubleClick(tables[1]);
    await screen.findByText(/session-p2/);
    fireEvent.click(screen.getByRole('button', { name: 'events · 订单库' }));
    expect(screen.getByLabelText('查询结果').textContent).toContain(
      'session-p1',
    );
  });
  it('moves a saved connection into a group without exposing its password or reconnecting', async () => {
    const host = runtime();
    host.save = vi.fn(async (input) => ({ ...input, revision: 2 }));
    await open(host);
    fireEvent.contextMenu(
      within(screen.getByLabelText('数据库连接与对象')).getByText('订单库'),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '移动到分组…' }));
    fireEvent.change(screen.getByLabelText('分组名称'), {
      target: { value: '业务系统' },
    });
    fireEvent.click(screen.getByRole('button', { name: '移动' }));
    await screen.findByRole('button', { name: '业务系统，1 个连接' });
    expect(host.save).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'p1', revision: 1, group: '业务系统' }),
    );
    expect(vi.mocked(host.save).mock.calls[0][0].password).toBeUndefined();
    expect(host.disconnect).not.toHaveBeenCalled();
  });
  it('manages persistent empty groups and moves connections to root without disturbing an open query', async () => {
    const host = runtime();
    let profiles = [{ ...profile, group: '业务系统' }];
    let groups = ['业务系统'];
    host.profiles = vi.fn(async () => [...profiles]);
    host.groups = vi.fn(async () => [...groups]);
    host.createGroup = vi.fn(async (name) => {
      groups.push(name);
    });
    host.renameGroup = vi.fn(async (oldName, name) => {
      groups = groups.map((g) => (g === oldName ? name : g));
      profiles = profiles.map((p) =>
        p.group === oldName
          ? { ...p, group: name, revision: p.revision + 1 }
          : p,
      );
    });
    host.deleteGroup = vi.fn(async (name) => {
      groups = groups.filter((g) => g !== name);
      profiles = profiles.map((p) =>
        p.group === name ? { ...p, group: '', revision: p.revision + 1 } : p,
      );
    });
    host.save = vi.fn(async (input) => {
      const next = {
        ...input,
        group: input.group ?? '',
        revision: input.revision + 1,
      };
      profiles = profiles.map((p) => (p.id === next.id ? next : p));
      return next;
    });
    await open(host);
    fireEvent.change(screen.getByLabelText('SQL 编辑器'), {
      target: { value: 'SELECT draft_not_to_lose' },
    });
    fireEvent.click(screen.getByRole('button', { name: '对象树选项' }));
    fireEvent.click(screen.getByRole('menuitem', { name: '新建分组…' }));
    fireEvent.change(screen.getByLabelText('分组名称'), {
      target: { value: '演练连接' },
    });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));
    await screen.findByRole('button', { name: '演练连接，0 个连接' });
    expect(screen.getByText('空分组')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '管理分组 演练连接' }));
    fireEvent.click(screen.getByRole('menuitem', { name: '重命名分组…' }));
    fireEvent.change(screen.getByLabelText('分组名称'), {
      target: { value: '排查分组' },
    });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await screen.findByRole('button', { name: '排查分组，0 个连接' });
    fireEvent.contextMenu(
      within(screen.getByLabelText('数据库连接与对象')).getByText('订单库'),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '移动到分组…' }));
    fireEvent.change(screen.getByLabelText('分组名称'), {
      target: { value: '排查分组' },
    });
    fireEvent.click(screen.getByRole('button', { name: '移动' }));
    await screen.findByRole('button', { name: '排查分组，1 个连接' });
    fireEvent.click(screen.getByRole('button', { name: '管理分组 排查分组' }));
    fireEvent.click(screen.getByRole('menuitem', { name: '删除分组…' }));
    expect(screen.getByRole('dialog').textContent).toContain(
      '1 个连接将移回顶层',
    );
    fireEvent.click(screen.getByRole('button', { name: '删除分组' }));
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: '管理分组 排查分组' }),
      ).toBeNull(),
    );
    expect(
      within(screen.getByLabelText('数据库连接与对象')).getByText('订单库'),
    ).toBeTruthy();
    expect(screen.getByLabelText('SQL 编辑器')).toHaveProperty(
      'value',
      'SELECT draft_not_to_lose',
    );
    expect(profiles[0].group).toBe('');
    expect(host.connect).toHaveBeenCalledTimes(1);
    expect(host.disconnect).not.toHaveBeenCalled();
    expect(host.query).not.toHaveBeenCalled();
    expect(host.createGroup).toHaveBeenCalledWith('演练连接');
    expect(host.renameGroup).toHaveBeenCalledWith('演练连接', '排查分组');
    expect(host.deleteGroup).toHaveBeenCalledWith('排查分组');
  });
  it('copies connection settings into a new draft without copying stored credentials', async () => {
    const host = runtime();
    await open(host);
    fireEvent.contextMenu(
      within(screen.getByLabelText('数据库连接与对象')).getByText('订单库'),
    );
    fireEvent.click(screen.getByRole('menuitem', { name: '复制连接配置' }));
    await screen.findByRole('dialog', { name: '添加数据库连接' });
    await screen.findByLabelText('密码');
    expect(
      (screen.getByLabelText('密码') as HTMLInputElement).placeholder,
    ).toBe('数据库密码');
    expect(screen.queryByLabelText('清空已保存的密码')).toBeNull();
    expect(
      (screen.getByLabelText('数据库地址') as HTMLInputElement).value,
    ).toBe('localhost');
    expect(host.save).not.toHaveBeenCalled();
  });
  it('filters loaded objects and hides system databases until explicitly enabled', async () => {
    const host = runtime();
    host.schemas = vi.fn(async () => ['orders', 'information_schema']);
    await open(host);
    const tree = screen.getByLabelText('数据库连接与对象');
    expect(within(tree).queryByText('information_schema')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: '对象树选项' }));
    fireEvent.click(screen.getByRole('menuitem', { name: '显示系统数据库' }));
    expect(within(tree).getByText('information_schema')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('搜索数据库或表'), {
      target: { value: 'events' },
    });
    expect(within(tree).getByRole('button', { name: 'events' })).toBeTruthy();
    expect(within(tree).queryByText('information_schema')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: '清除' }));
    expect(within(tree).getByText('information_schema')).toBeTruthy();
  });
  it('exports CSV without losing decimal text or escaping multiline cells incorrectly', () => {
    const csv = resultCSV({
      statement: '',
      columns: [{ name: 'value', type: 'TEXT' }],
      rows: [['12345678901234567890.1234567890'], ['a,"b"\nnext'], [null]],
      affectedRows: 0,
      truncated: false,
    });
    expect(csv).toContain('"12345678901234567890.1234567890"');
    expect(csv).toContain('"a,""b""\nnext"');
    expect(csv.endsWith('\r\n')).toBe(true);
  });
  it('AI 的完整页签保留多块 SQL 与结果，交还后可继续人工查询',async()=>{
    const host=runtime();const onEdit=vi.fn();const onPresented=vi.fn();
    const result={sets:[{statement:'SELECT 1;',columns:[{name:'n',type:'INTEGER'}],rows:[['1']],affectedRows:0,truncated:false}],durationMs:1,error:'',cancelled:false,disconnected:false};
    const first={id:'ai-db',profile,session:{id:'independent',profileId:profile.id,version:'8.4',database:'orders',readOnly:false},schemas:['orders'],sql:'-- AI：核对连接\nSELECT 1;',blocks:[{id:'block1',purpose:'核对连接',sql:'SELECT 1;',at:'2026-10-21',status:'completed',result}],ownerTurn:'turn',pending:'receipt1',connected:true,dismissed:false,revision:1};
    const renderManaged=(workspace:typeof first)=><DatabaseWorkspace runtime={host} managed={{workspace,editing:false,onEdit,onPresented}}/>;
    const page=render(renderManaged(first));
    await waitFor(()=>expect(onPresented).toHaveBeenCalledWith(first.sql));
    expect(host.query).not.toHaveBeenCalled();expect(host.connect).not.toHaveBeenCalled();
    const next={...first,revision:2,pending:'receipt2',sql:first.sql+'\n\n-- AI：检查下一项\nSELECT 2;',blocks:[...first.blocks,{...first.blocks[0],id:'block2',sql:'SELECT 2;',purpose:'检查下一项'}]};
    page.rerender(renderManaged(next));
    await waitFor(()=>expect((screen.getByRole('textbox',{name:'SQL 编辑器'}) as HTMLTextAreaElement).value).toBe(next.sql));
    expect(host.query).not.toHaveBeenCalled();
    page.rerender(renderManaged({...next,revision:3,pending:'',ownerTurn:''}));
    const input=screen.getByRole('textbox',{name:'SQL 编辑器'}) as HTMLTextAreaElement;
    fireEvent.change(input,{target:{value:next.sql+'\nSELECT 3;'}});
    expect(onEdit).toHaveBeenCalledWith(next.sql+'\nSELECT 3;');
    input.setSelectionRange(input.value.length,input.value.length);
    fireEvent.click(screen.getByRole('button',{name:'执行 SQL'}));
    await waitFor(()=>expect(host.query).toHaveBeenCalledOnce());
    expect(host.query).toHaveBeenCalledWith(expect.objectContaining({sessionId:'independent',sql:next.sql+'\nSELECT 3;',mode:'statement'}));
    await waitFor(()=>expect(screen.getByLabelText('查询结果').textContent).toContain('9007199254740993'));
    page.rerender(renderManaged({...next,revision:4,pending:'',ownerTurn:'',blocks:[...next.blocks.slice(0,-1),{...next.blocks[1],status:'cancelled',result:{...result,sets:[]}}]}));
    expect(screen.getByLabelText('查询结果').textContent).toContain('9007199254740993');
    page.unmount();expect(host.disconnect).not.toHaveBeenCalled();
  });
});
