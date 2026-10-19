import { afterEach, beforeAll, expect, it, vi } from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react';
import { ConnectionDialog, emptyProfile } from './ConnectionDialog';
import type { DatabaseJDBCDriver, DatabaseRuntime } from './types';

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = vi.fn(function (
    this: HTMLDialogElement,
  ) {
    this.setAttribute('open', '');
  });
  HTMLDialogElement.prototype.close = vi.fn();
});
afterEach(cleanup);
it('does not silently disable readonly when selecting a generic driver', async () => {
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({
      available: true,
      message: '',
      drivers: [{ id: 'generic', name: 'Vendor', dialect: 'generic' }],
    })),
    test: vi.fn(async () => 'vendor'),
  } as unknown as DatabaseRuntime;
  render(
    <ConnectionDialog
      profile={{ ...emptyProfile(), port: 32063, user: 'reader', readOnly: true }}
      runtime={runtime}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  await screen.findByText('Vendor');
  expect(screen.queryByLabelText('连接驱动')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByRole('alert');
  expect(runtime.test).not.toHaveBeenCalled();
  const readonly = screen.getByLabelText('限制为只读（可选）');
  expect(readonly).toBeChecked();
  fireEvent.click(readonly);
  expect(readonly).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('连接成功 · vendor');
  expect(runtime.test).toHaveBeenCalledWith(
    expect.objectContaining({ readOnly: false, engine: 'jdbc' }),
  );
});
it('imports a driver and selects it without losing connection fields or password', async () => {
  const driver: DatabaseJDBCDriver = {
    id: 'driver-1',
    name: 'MySQL Connector/J',
    className: 'com.mysql.cj.jdbc.Driver',
    urlTemplate: 'jdbc:mysql://{host}:{port}/{database}',
    dialect: 'mysql',
    version: '9.7',
    jars: [{ name: 'mysql.jar', sha256: 'hash' }],
  };
  let drivers: DatabaseJDBCDriver[] = [];
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({
      available: true,
      message: '运行组件已就绪',
      drivers,
    })),
    chooseJDBCJars: vi.fn(async () => ['D:/drivers/mysql.jar']),
    importJDBC: vi.fn(async () => {
      drivers = [driver];
      return driver;
    }),
    test: vi.fn(async () => 'MySQL 8.4.8'),
    save: vi.fn(),
  } as unknown as DatabaseRuntime;
  render(
    <ConnectionDialog
      profile={{ ...emptyProfile(), host: 'db.internal', user: 'ops' }}
      runtime={runtime}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  await screen.findByText('先导入数据库驱动');
  expect(screen.queryByLabelText('数据库地址')).toBeNull();
  screen.getByRole('button', { name: '导入驱动…' }).focus();
  fireEvent.click(screen.getByRole('button', { name: '导入驱动…' }));
  const manager = screen.getByRole('dialog', { name: '驱动管理' });
  await within(manager).findByText('运行组件已就绪');
  fireEvent.change(within(manager).getByLabelText('驱动类型'), {target: {value: 'mysql'}});
  fireEvent.click(within(manager).getByRole('button', { name: '选择 JAR…' }));
  await within(manager).findByText('mysql.jar');
  fireEvent.click(within(manager).getByRole('button', { name: '导入并验证' }));
  await screen.findByText('MySQL Connector/J');
  expect(runtime.importJDBC).toHaveBeenCalledWith(
    expect.objectContaining({
      paths: ['D:/drivers/mysql.jar'],
      dialect: 'mysql',
    }),
  );
  expect(screen.getAllByRole('dialog')).toHaveLength(1);
  expect(screen.queryByLabelText('连接驱动')).toBeNull();
  expect(screen.getByLabelText('数据库地址')).toHaveValue('db.internal');
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'test-secret' } });
  fireEvent.click(screen.getByRole('button', {name: '更换／管理'}));
  fireEvent.click(screen.getByRole('button', {name: '管理驱动…'}));
  await screen.findByRole('button', {name: '使用驱动'});
  fireEvent.click(screen.getByRole('button', {name: '使用驱动'}));
  expect(screen.getByLabelText('密码')).toHaveValue('test-secret');
  fireEvent.click(screen.getByRole('button', { name: '测试连接' }));
  await screen.findByText('连接成功 · MySQL 8.4.8');
  expect(runtime.test).toHaveBeenCalledWith(
    expect.objectContaining({
      engine: 'jdbc',
      driverId: driver.id,
      host: 'db.internal',
      user: 'ops',
      password: 'test-secret',
      readOnly: false,
    }),
  );
});
it('keeps import disabled when the optional runtime is missing', async () => {
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({
      available: false,
      message: '未安装 JDBC 运行组件',
      drivers: [],
    })),
  } as unknown as DatabaseRuntime;
  render(
    <ConnectionDialog
      profile={emptyProfile()}
      runtime={runtime}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  await screen.findByText('先导入数据库驱动');
  screen.getByRole('button', { name: '导入驱动…' }).focus();
  fireEvent.click(screen.getByRole('button', { name: '导入驱动…' }));
  await screen.findByText('未安装 JDBC 运行组件');
  expect(screen.getByRole('button', { name: '导入并验证' })).toBeDisabled();
  expect(screen.queryByLabelText('连接驱动')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: '完成' }));
  expect(screen.getByRole('button', { name: '导入驱动…' })).toHaveFocus();
  expect(screen.getAllByRole('dialog')).toHaveLength(1);
});

it('requires an imported driver and never offers built-in MySQL', async () => {
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({available: true, message: '', drivers: []})),
    save: vi.fn(), test: vi.fn(),
  } as unknown as DatabaseRuntime;
  render(<ConnectionDialog profile={{...emptyProfile(), port: 32063, user: 'ops'}} runtime={runtime} onClose={vi.fn()} onSaved={vi.fn()} />);
  await screen.findByText('先导入数据库驱动');
  expect(screen.queryByRole('combobox')).toBeNull();
  expect(screen.queryByRole('button', {name: '保存并连接'})).toBeNull();
  expect(screen.queryByRole('button', {name: '测试连接'})).toBeNull();
  expect(screen.queryByRole('tab')).toBeNull();
  expect(screen.getByRole('button', {name: '导入驱动…'})).toHaveClass('oc-db-primary');
  expect(runtime.save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', {name: '导入驱动…'}));
  await screen.findByRole('button', {name: '选择 JAR…'});
  expect(screen.getByLabelText('驱动类型')).toHaveValue('gauss');
  expect(screen.getByLabelText('驱动类')).toHaveValue('com.huawei.gauss.jdbc.ZenithDriver');
  expect(screen.getByLabelText('URL 模板')).toHaveValue('jdbc:zenith:@{host}:{port}');
  expect(screen.getByLabelText('驱动类')).not.toBeVisible();
});
it('does not silently bind a legacy MySQL connection to the only imported Gauss driver', async () => {
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({available:true, message:'', drivers:[{id:'gauss',name:'Gauss 100',dialect:'generic'}]})),
    test:vi.fn(),
  } as unknown as DatabaseRuntime;
  render(<ConnectionDialog profile={{...emptyProfile(),id:'legacy',engine:'mysql',port:3306,user:'ops',passwordSet:true}} runtime={runtime} onClose={vi.fn()} onSaved={vi.fn()} />);
  await screen.findByRole('option', {name:'Gauss 100 · JDBC'});
  expect(screen.getByLabelText('连接驱动')).toHaveValue('');
  expect(screen.getByLabelText('密码')).toHaveAttribute('placeholder','已保存，留空保持');
  fireEvent.click(screen.getByRole('button', {name:'测试连接'}));
  await screen.findByText('请先导入并选择数据库驱动。');
  expect(runtime.test).not.toHaveBeenCalled();
});

it('asks for an explicit choice with multiple drivers, then collapses the choice after selection', async () => {
  const mysql = {id:'mysql',name:'MySQL Connector/J',dialect:'mysql'};
  const gauss = {id:'gauss',name:'Gauss 100',dialect:'generic'};
  const runtime = {
    gateways: vi.fn(async () => []),
    jdbcStatus: vi.fn(async () => ({available:true,message:'',drivers:[mysql,gauss]})),
    test:vi.fn(async () => 'Gauss test'),
  } as unknown as DatabaseRuntime;
  render(<ConnectionDialog profile={{...emptyProfile(),host:'db.internal',port:32063,user:'ops'}} runtime={runtime} onClose={vi.fn()} onSaved={vi.fn()} />);
  await screen.findByLabelText('连接驱动');
  expect(screen.getByLabelText('连接驱动')).toHaveValue('');
  fireEvent.change(screen.getByLabelText('连接驱动'), {target:{value:'gauss'}});
  expect(screen.queryByLabelText('连接驱动')).toBeNull();
  expect(screen.getByText('Gauss 100')).toBeVisible();
  fireEvent.click(screen.getByRole('button',{name:'更换／管理'}));
  expect(screen.getByLabelText('连接驱动')).toHaveValue('gauss');
  expect(screen.getByLabelText('数据库地址')).toHaveValue('db.internal');
  fireEvent.click(screen.getByRole('button',{name:'完成更换'}));
  fireEvent.click(screen.getByRole('button',{name:'测试连接'}));
  await screen.findByText('连接成功 · Gauss test');
  expect(runtime.test).toHaveBeenCalledWith(expect.objectContaining({driverId:'gauss',host:'db.internal',port:32063}));
});
it('offers retry instead of a false empty-driver state when loading fails', async () => {
  const runtime = {
    gateways:vi.fn(async () => []),
    jdbcStatus:vi.fn().mockRejectedValueOnce(new Error('read failed')).mockResolvedValue({available:true,message:'',drivers:[]}),
  } as unknown as DatabaseRuntime;
  render(<ConnectionDialog profile={emptyProfile()} runtime={runtime} onClose={vi.fn()} onSaved={vi.fn()} />);
  await screen.findByRole('button',{name:'重新加载'});
  expect(screen.queryByRole('button',{name:'导入驱动…'})).toBeNull();
  fireEvent.click(screen.getByRole('button',{name:'重新加载'}));
  await screen.findByText('先导入数据库驱动');
  expect(screen.getByRole('button',{name:'导入驱动…'})).toHaveFocus();
});
