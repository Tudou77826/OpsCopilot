import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { ConnectionDialog, emptyProfile } from './ConnectionDialog';
import type { DatabaseRuntime } from './types';

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (
    this: HTMLDialogElement,
  ) {
    this.setAttribute('open', '');
  });
  HTMLDialogElement.prototype.close = vi.fn();
});
afterEach(cleanup);
const driver = { id: 'mysql-jdbc', name: 'MySQL Connector/J', className: 'com.mysql.cj.jdbc.Driver', urlTemplate: 'jdbc:mysql://{host}:{port}/{database}', dialect: 'mysql', version: '9.7', jars: [] };
const jdbcStatus = vi.fn(async () => ({ available: true, message: '运行组件已就绪', drivers: [driver] }));
async function setup(saved = false) {
  const profile = {
    ...emptyProfile(), driverId: driver.id, port: 3306,
    host: 'db.internal',
    user: 'ops',
    ...(saved ? { id: 'saved', passwordSet: true } : {}),
  };
  const runtime = {
    jdbcStatus,
    gateways: vi.fn(async () => []),
    test: vi.fn(async () => 'MySQL 8.4.8'),
    save: vi.fn(async (p) => ({ ...p, id: 'saved' })),
  } as unknown as DatabaseRuntime;
  const onSaved = vi.fn();
  render(
    <ConnectionDialog
      profile={profile}
      runtime={runtime}
      onSaved={onSaved}
      onClose={vi.fn()}
    />,
  );
  await screen.findByText(driver.name,{exact:true});
  return { runtime, onSaved };
}
describe('connection dialog', () => {
  it('rejects a saved connection whose imported driver no longer exists', async () => {
    const runtime = {jdbcStatus, gateways:vi.fn(async()=>[]),test:vi.fn(),save:vi.fn()} as unknown as DatabaseRuntime;
    render(<ConnectionDialog profile={{...emptyProfile(),id:'saved',driverId:'removed-driver',host:'db.internal',user:'ops',port:3306}} runtime={runtime} onSaved={vi.fn()} onClose={vi.fn()}/>);
    await screen.findByRole('combobox',{name:'连接驱动'});
    fireEvent.click(screen.getByRole('button',{name:'测试连接'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('请先导入并选择数据库驱动');
    fireEvent.click(screen.getByRole('button',{name:'仅保存'}));
    expect(runtime.test).not.toHaveBeenCalled();expect(runtime.save).not.toHaveBeenCalled();
  });
  it('tests an unsaved draft without saving and clears stale success on edits', async () => {
    const { runtime } = await setup();
    expect(
      screen.getByRole('tab', { name: '数据库', selected: true }),
    ).toBeVisible();
    expect(screen.getByLabelText('连接名称（可选）')).not.toBeVisible();
    fireEvent.change(screen.getByLabelText('密码'), {
      target: { value: 'draft-secret' },
    });
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    await screen.findByText('连接成功 · MySQL 8.4.8');
    expect(runtime.test).toHaveBeenCalledWith(
      expect.objectContaining({
        name: '',
        environment: '',
        readOnly: false,
        password: 'draft-secret',
      }),
    );
    expect(runtime.save).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('数据库地址'), {
      target: { value: 'another-db' },
    });
    expect(screen.queryByRole('status')).toBeNull();
  });
  it('preserves an existing password unless explicitly cleared', async () => {
    const { runtime } = await setup(true);
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    await screen.findByRole('status');
    expect(runtime.test).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ password: expect.anything() }),
    );
    fireEvent.click(screen.getByRole('button', { name: '使用空密码' }));
    expect(screen.getByLabelText('密码')).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    await waitFor(() =>
      expect(runtime.test).toHaveBeenLastCalledWith(
        expect.objectContaining({ password: '' }),
      ),
    );
    fireEvent.click(screen.getByRole('button', { name: '撤销，保留原密码' }));
    expect(screen.getByLabelText('密码')).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    await waitFor(() =>
      expect(runtime.test).toHaveBeenLastCalledWith(
        expect.not.objectContaining({ password: expect.anything() }),
      ),
    );
    expect(runtime.save).not.toHaveBeenCalled();
  });
  it('shows failures and keeps the draft editable', async () => {
    const { runtime } = await setup();
    vi.mocked(runtime.test).mockRejectedValueOnce(
      new Error('connection refused'),
    );
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    expect((await screen.findByRole('alert')).textContent).toContain(
      'connection refused',
    );
    expect((screen.getByLabelText('用户名') as HTMLInputElement).disabled).toBe(
      false,
    );
    expect(runtime.save).not.toHaveBeenCalled();
  });
  it('copies only the diagnostic message, never the connection form password', async () => {
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const { runtime } = await setup();
    const detail =
      'JDBC 操作失败 · 建立连接／握手认证（SQLState 08001，代码 0）\nConnectException: Connection refused';
    vi.mocked(runtime.test).mockRejectedValueOnce(new Error(detail));
    fireEvent.change(screen.getByLabelText('密码'), {
      target: { value: 'private-form-password' },
    });
    fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
    await screen.findByRole('alert');
    fireEvent.click(screen.getByRole('button', { name: '复制错误详情' }));
    await screen.findByRole('button', { name: '错误详情已复制' });
    expect(writeText).toHaveBeenCalledWith('Error: ' + detail);
    expect(writeText.mock.calls.flat().join('')).not.toContain(
      'private-form-password',
    );
  });
  it('distinguishes save-only from save-and-connect without stale click state', async () => {
    const { runtime, onSaved } = await setup();
    fireEvent.click(screen.getByRole('button', { name: '仅保存' }));
    await waitFor(() =>
      expect(onSaved).toHaveBeenCalledWith(expect.anything(), false),
    );
    expect(runtime.save).toHaveBeenCalledWith(
      expect.objectContaining({ name: '' }),
    );
  });
  it('saves and connects by default', async () => {
    const { onSaved } = await setup();
    fireEvent.click(screen.getByRole('button', { name: '保存并连接' }));
    await waitFor(() =>
      expect(onSaved).toHaveBeenCalledWith(expect.anything(), true),
    );
  });
});

it('returns to the invalid database field from another section without testing', async () => {
  const { runtime } = await setup();
  fireEvent.change(screen.getByLabelText('用户名'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('tab', { name: '更多设置' }));
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  expect(screen.getByRole('tab', { name: '数据库' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  expect(runtime.test).not.toHaveBeenCalled();
});
it('requires an SSH node and returns to its section when testing', async () => {
  const { runtime } = await setup();
  fireEvent.click(await screen.findByRole('tab', { name: 'SSH 隧道' }));
  fireEvent.click(screen.getByLabelText('使用 SSH 隧道'));
  fireEvent.click(screen.getByRole('tab', { name: '数据库' }));
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  expect(screen.getByRole('tab', { name: 'SSH 隧道' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  expect(runtime.test).not.toHaveBeenCalled();
});
it('keeps transport choices internal and submits automatic SSH handling', async () => {
  const runtime = {
    jdbcStatus,
    gateways: vi.fn(async () => [{ id: 'final55', name: '55 经 50', host: '10.0.0.55' }]),
    test: vi.fn(async () => 'Gauss test'),
  } as unknown as DatabaseRuntime;
  render(<ConnectionDialog profile={{ ...emptyProfile(), driverId: driver.id, port: 32063, user: 'ops', gatewayId: 'final55' }} runtime={runtime} onSaved={vi.fn()} onClose={vi.fn()} />);
  fireEvent.click(await screen.findByRole('tab', { name: 'SSH 隧道' }));
  await screen.findByRole('option', { name: '55 经 50 · 10.0.0.55' });
  expect(screen.queryByLabelText('通道模式')).toBeNull();
  expect(screen.queryByLabelText('连接路径')).toBeNull();
  expect(screen.queryByText(/临时运行 Python 3/)).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await waitFor(() => expect(runtime.test).toHaveBeenCalledWith(expect.objectContaining({ gatewayId: 'final55', gatewayMode: 'auto', host: '127.0.0.1' })));
  await waitFor(() => expect(screen.getByRole('button', { name: '测试连接' })).toBeEnabled());
  fireEvent.click(screen.getByLabelText('直接连接'));
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await waitFor(() => expect(runtime.test).toHaveBeenLastCalledWith(expect.objectContaining({ gatewayId: '', gatewayMode: 'auto' })));
});
it('refreshes failed gateway loading and distinguishes gateway from target', async () => {
  const runtime = {
    jdbcStatus,
    gateways: vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue([{ id: 'jump', name: 'Jump 51', host: '10.0.0.51' }]),
    test: vi.fn(async () => 'MySQL 8.4.8'),
  } as unknown as DatabaseRuntime;
  render(
    <ConnectionDialog
      profile={{
        ...emptyProfile(), driverId: driver.id, port: 3306,
        host: '10.0.0.55',
        user: 'ops',
        gatewayId: 'jump',
      }}
      runtime={runtime}
      onSaved={vi.fn()}
      onClose={vi.fn()}
    />,
  );
  fireEvent.click(await screen.findByRole('tab', { name: 'SSH 隧道' }));
  await screen.findByText('SSH 连接列表加载失败，请重试。');
  fireEvent.click(screen.getByRole('button', { name: '刷新 SSH 连接列表' }));
  await screen.findByRole('option', { name: 'Jump 51 · 10.0.0.51' });
  expect(screen.queryByLabelText('连接路径')).toBeNull();
  expect(screen.queryByRole('alert')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('连接成功 · MySQL 8.4.8');
  expect(runtime.test).toHaveBeenCalledWith(
    expect.objectContaining({ host: '10.0.0.55', gatewayId: 'jump' }),
  );
});
it('supports keyboard section navigation and blocks closing while a test is pending', async () => {
  const runtime = {
    jdbcStatus,
    gateways: vi.fn(async () => []),
    test: vi.fn(() => new Promise<string>(() => {})),
  } as unknown as DatabaseRuntime;
  const onClose = vi.fn();
  render(
    <ConnectionDialog
      profile={{ ...emptyProfile(), driverId: driver.id, port: 3306, user: 'ops' }}
      runtime={runtime}
      onSaved={vi.fn()}
      onClose={onClose}
    />,
  );
  fireEvent.keyDown(await screen.findByRole('tab', { name: '数据库' }), {
    key: 'ArrowRight',
  });
  expect(screen.getByRole('tab', { name: 'SSH 隧道' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('正在验证连接，请稍候…');
  expect(screen.getByRole('button', { name: '取消' })).toBeDisabled();
  expect(
    screen.getByRole('button', { name: '关闭添加数据库连接' }),
  ).toBeDisabled();
  expect(screen.getByRole('tab', { name: '数据库' })).toBeDisabled();
  fireEvent(
    screen.getByRole('dialog'),
    new Event('cancel', { bubbles: true, cancelable: true }),
  );
  expect(onClose).not.toHaveBeenCalled();
});

it('makes readonly an explicit opt-in for supported drivers', async () => {
  const { runtime } = await setup();
  fireEvent.click(screen.getByRole('tab', { name: '更多设置' }));
  await screen.findByText('MySQL Connector/J');
  const readonly = screen.getByLabelText('限制为只读（可选）');
  expect(readonly).not.toBeChecked();
  fireEvent.click(readonly);
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('连接成功 · MySQL 8.4.8');
  expect(runtime.test).toHaveBeenCalledWith(
    expect.objectContaining({ readOnly: true }),
  );
  expect(screen.getByLabelText('连接标记（可选）')).toHaveAccessibleDescription(
    '显示在连接名称旁，仅作识别，不影响连接或权限。',
  );
  expect(screen.queryByLabelText('清空已保存的密码')).toBeNull();
});
it('replaces a saved password only when a new value is supplied', async () => {
  const { runtime } = await setup(true);
  fireEvent.change(screen.getByLabelText('密码'), {
    target: { value: 'replacement-secret' },
  });
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('连接成功 · MySQL 8.4.8');
  expect(runtime.test).toHaveBeenLastCalledWith(
    expect.objectContaining({ password: 'replacement-secret' }),
  );
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await waitFor(() =>
    expect(runtime.test).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ password: expect.anything() }),
    ),
  );
});
