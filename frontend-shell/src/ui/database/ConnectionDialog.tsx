import { useEffect, useId, useRef, useState } from 'react';
import { TbRefresh, TbCheck, TbInfoCircle, TbDatabase, TbFileImport } from 'react-icons/tb';
import { DatabaseDialog } from './DatabaseDialog';
import { JDBCDriverDialog } from './JDBCDriverDialog';
import { DatabaseError } from './DatabaseError';
import type { DatabaseJDBCStatus } from './types';
import type {
  DatabaseGateway,
  DatabaseProfile,
  DatabaseProfileInput,
  DatabaseRuntime,
} from './types';

export const emptyProfile = (): DatabaseProfile => ({
  id: '',
  revision: 0,
  name: '',
  environment: '',
  group: '',
  engine: 'jdbc',
  host: '127.0.0.1',
  port: 0,
  user: '',
  database: '',
  tls: 'off',
  readOnly: false,
  gatewayId: '',
  gatewayMode: 'auto',
  passwordSet: false,
});

export function ConnectionDialog({
  profile,
  runtime,
  onClose,
  onSaved,
}: {
  profile: DatabaseProfile;
  runtime: DatabaseRuntime;
  onClose(): void;
  onSaved(profile: DatabaseProfile, connect: boolean): void;
}) {
  const form = useRef<HTMLFormElement>(null);
  const environmentHelpId = useId();
  const environmentInputId = useId();
  const readOnlyHelpId = useId();
  const passwordHelpId = useId();
  const [draft, setDraft] = useState({
    ...profile,
    engine: 'jdbc',
    driverId: profile.engine === 'jdbc' ? profile.driverId : '',
  });
  const [section, setSection] = useState('database');
  const [password, setPassword] = useState('');
  const [clearPassword, setClearPassword] = useState(false);
  const [usingSSH, setUsingSSH] = useState(!!profile.gatewayId);
  const [gateways, setGateways] = useState<DatabaseGateway[]>([]);
  const [gatewayError, setGatewayError] = useState('');
  const [loadingGateways, setLoadingGateways] = useState(false);
  const [busy, setBusy] = useState<'save' | 'test' | null>(null);
  const [testedVersion, setTestedVersion] = useState('');
  const [error, setError] = useState('');
  const [jdbc, setJDBC] = useState<DatabaseJDBCStatus>();
  const [managingDrivers, setManagingDrivers] = useState(false);
  const [choosingDriver, setChoosingDriver] = useState(false);
  const driverSelect = useRef<HTMLSelectElement>(null);
  async function refreshGateways() {
    setLoadingGateways(true);
    setGatewayError('');
    try {
      setGateways(await runtime.gateways());
    } catch {
      setGatewayError('SSH 连接列表加载失败，请重试。');
    } finally {
      setLoadingGateways(false);
    }
  }
  async function refreshDrivers() {
    setError('');
    try {
      const next = await runtime.jdbcStatus();
      setJDBC(next);
      if (!profile.id && next.drivers.length === 1) {
        const driver = next.drivers[0];
        setDraft((p) => p.driverId ? p : {
          ...p, engine: 'jdbc', driverId: driver.id,
          port: p.port || (driver.dialect === 'mysql' ? 3306 : 0),
        });
      }
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void refreshDrivers();
    void refreshGateways();
  }, [runtime]);
  const invalidate = () => {
    setTestedVersion('');
    setError('');
  };
  const change = <K extends keyof DatabaseProfile>(
    key: K,
    value: DatabaseProfile[K],
  ) => {
    setDraft((p) => ({ ...p, [key]: value }));
    invalidate();
  };
  function input(): DatabaseProfileInput {
    const value: DatabaseProfileInput = {
      ...draft,
      gatewayId: usingSSH ? draft.gatewayId : '',
      gatewayMode: 'auto',
    };
    if (password || clearPassword || !profile.id) value.password = password;
    return value;
  }
  function valid() {
    if (!draft.driverId || !selectedDriver) {
      setSection('database');
      setChoosingDriver(true);
      setError('请先导入并选择数据库驱动。');
      return false;
    }
    const invalid = form.current?.querySelector<
      HTMLInputElement | HTMLSelectElement
    >('input:invalid, select:invalid, textarea:invalid');
    if (invalid) {
      setSection(
        invalid.closest<HTMLElement>('[data-section]')?.dataset.section ??
          'database',
      );
      requestAnimationFrame(() => invalid.reportValidity());
      return false;
    }
    if (draft.readOnly && !supportsReadOnly) {
      setSection('options');
      setTestedVersion('');
      setError(
        '当前驱动不支持程序只读限制，请关闭“限制为只读”。如需只读访问，请使用数据库只读账号。',
      );
      return false;
    }
    return true;
  }
  async function save(connect: boolean) {
    if (busy || !valid()) return;
    setBusy('save');
    setError('');
    try {
      onSaved(await runtime.save(input()), connect);
    } catch (e) {
      setError(String(e));
      setBusy(null);
    }
  }
  async function test() {
    if (busy || !valid()) return;
    setBusy('test');
    invalidate();
    try {
      setTestedVersion(await runtime.test(input()));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }
  const gateway = gateways.find((g) => g.id === draft.gatewayId);
  const selectedDriver = jdbc?.drivers.find((d) => d.id === draft.driverId);
  const supportsReadOnly = selectedDriver?.dialect === 'mysql';
  const needsImport = jdbc?.drivers.length === 0;
  const preparing = !jdbc;
  const driverFirst = needsImport || preparing;
  useEffect(() => {
    if (needsImport && !managingDrivers) {
      form.current?.querySelector<HTMLButtonElement>('[data-db-initial-focus]')?.focus();
    }
  }, [needsImport, managingDrivers]);
  const tabs = [
    ['database', '数据库'],
    ['ssh', 'SSH 隧道'],
    ['options', '更多设置'],
  ] as const;
  const selectDriver = (id: string) => {
    setChoosingDriver(false);
    change('engine', 'jdbc');
    change('driverId', id);
    if (!draft.port && jdbc?.drivers.find((d) => d.id === id)?.dialect === 'mysql') {
      change('port', 3306);
    }
  };
  return (
    <>
      <DatabaseDialog
        title={profile.id ? '编辑连接' : '添加数据库连接'}
        subtitle={profile.name || '配置数据库目标与连接方式'}
        formRef={form}
        noValidate
        busy={!!busy}
        visible={!managingDrivers}
        onClose={onClose}
        onSubmit={(e) =>
          void save(
            (e.nativeEvent as SubmitEvent).submitter?.getAttribute(
              'data-save-only',
            ) !== 'true',
          )
        }
        navigation={driverFirst ? undefined : (
          <>
            <div
              className="oc-db-modal-tabs"
              role="tablist"
              aria-label="连接设置分类"
            >
              {tabs.map(([id, label]) => (
                <button
                  key={id}
                  id={`oc-db-section-${id}`}
                  role="tab"
                  type="button"
                  aria-selected={section === id}
                  tabIndex={section === id ? 0 : -1}
                  aria-controls={`oc-db-panel-${id}`}
                  disabled={!!busy}
                  onClick={() => setSection(id)}
                >
                  {label}
                  {id === 'ssh' && usingSSH && <i />}
                </button>
              ))}
            </div>
          </>
        )}
        feedback={
          error ? (
            <DatabaseError message={error} />
          ) : testedVersion ? (
            <div role="status" className="oc-db-test-success">
              <TbCheck size={16} />
              连接成功 · {testedVersion}
            </div>
          ) : busy === 'test' ? (
            <div role="status" className="oc-db-muted">
              正在验证连接，请稍候…
            </div>
          ) : undefined
        }
        actions={driverFirst ? (
          <>
            <button type="button" onClick={onClose}>取消</button>
            {needsImport ? (
              <button type="button" className="oc-db-primary" data-db-initial-focus onClick={() => setManagingDrivers(true)}>
                <TbFileImport size={16} />导入驱动…
              </button>
            ) : error ? (
              <button type="button" className="oc-db-primary" onClick={() => void refreshDrivers()}>重新加载</button>
            ) : null}
          </>
        ) : (
          <>
            <button
              className="oc-db-test-button"
              type="button"
              disabled={!!busy}
              onClick={() => void test()}
            >
              {busy === 'test' ? '正在测试…' : '测试连接'}
            </button>
            <button type="button" disabled={!!busy} onClick={onClose}>
              取消
            </button>
            <button type="submit" data-save-only="true" disabled={!!busy}>
              仅保存
            </button>
            <button className="oc-db-primary" type="submit" disabled={!!busy}>
              {busy === 'save' ? '正在保存…' : '保存并连接'}
            </button>
          </>
        )}
      >
        {driverFirst ? (
          <div className="oc-db-driver-onboarding">
            {needsImport ? (
              <>
                <TbDatabase size={32} aria-hidden="true" />
                <h4>先导入数据库驱动</h4>
                <p>选择厂商提供的 JDBC 文件，导入一次，之后即可直接创建连接。</p>
              </>
            ) : (
              <p role="status">{error ? '暂时无法读取驱动列表，请重试。' : '正在读取驱动…'}</p>
            )}
          </div>
        ) : (
          <fieldset disabled={!!busy} className="oc-db-connection-fields">
            <section
              role="tabpanel"
              id="oc-db-panel-database"
              aria-labelledby="oc-db-section-database"
              hidden={section !== 'database'}
              data-section="database"
            >
              {(!selectedDriver || choosingDriver) && (
                <div className="oc-db-driver-selection">
                  <div className="oc-db-driver-choice">
                    <label>
                      连接驱动
                      <select
                        required
                        ref={driverSelect}
                        value={draft.driverId ?? ''}
                        onChange={(e) => selectDriver(e.target.value)}
                      >
                        <option value="">请选择驱动</option>
                        {jdbc?.drivers.map((d) => (
                          <option value={d.id} key={d.id}>{d.name} · JDBC</option>
                        ))}
                        {draft.driverId && !selectedDriver && (
                          <option value={draft.driverId}>原 JDBC 驱动不可用</option>
                        )}
                      </select>
                    </label>
                    <button className="oc-db-secondary" type="button" onClick={() => setManagingDrivers(true)}>
                      管理驱动…
                    </button>
                  </div>
                  {profile.id && profile.engine !== 'jdbc' && !draft.driverId && (
                    <p className="oc-db-help">请选择导入的驱动，原地址和已保存的密码会保留。</p>
                  )}
                  {selectedDriver && (
                    <button type="button" onClick={() => setChoosingDriver(false)}>完成更换</button>
                  )}
                </div>
              )}
              <div className="oc-db-target-fields">
                <label>
                  数据库地址
                  <input
                    autoFocus
                    data-db-initial-focus
                    required
                    value={draft.host}
                    onChange={(e) => change('host', e.target.value)}
                    autoComplete="off"
                  />
                </label>
                <label>
                  端口
                  <input
                    required
                    type="number"
                    min={1}
                    max={65535}
                    value={draft.port || ''}
                    onChange={(e) => change('port', Number(e.target.value))}
                  />
                </label>
              </div>
              <div className="oc-db-form-grid">
                <label>
                  用户名
                  <input
                    required
                    value={draft.user}
                    onChange={(e) => change('user', e.target.value)}
                    autoComplete="off"
                  />
                </label>
                <div className="oc-db-password-field">
                  <label>
                    密码
                    <input
                      type="password"
                      value={password}
                      disabled={clearPassword}
                      aria-describedby={
                        profile.passwordSet ? passwordHelpId : undefined
                      }
                      onChange={(e) => {
                        setPassword(e.target.value);
                        invalidate();
                      }}
                      autoComplete="new-password"
                      placeholder={
                        clearPassword
                          ? '将使用空密码'
                          : profile.passwordSet
                            ? '已保存，留空保持'
                            : '数据库密码'
                      }
                    />
                  </label>
                  {profile.passwordSet && (
                    <div className="oc-db-password-help">
                      <span id={passwordHelpId}>
                        {clearPassword
                          ? '保存后移除本机保存的密码。'
                          : '输入新密码可更换。'}
                      </span>
                      <button
                        type="button"
                        onClick={() => {
                          setClearPassword(!clearPassword);
                          setPassword('');
                          invalidate();
                        }}
                      >
                        {clearPassword ? '撤销，保留原密码' : '使用空密码'}
                      </button>
                    </div>
                  )}
                </div>
              </div>
              <label>
                数据库（可选）
                <input
                  value={draft.database}
                  onChange={(e) => change('database', e.target.value)}
                  placeholder="可留空，连接后选择"
                />
              </label>
              {selectedDriver && !choosingDriver && (
                <div className="oc-db-current-driver">
                  <span>驱动 <strong>{selectedDriver.name}</strong></span>
                  <button type="button" onClick={() => {
                    setChoosingDriver(true);
                    requestAnimationFrame(() => driverSelect.current?.focus());
                  }}>更换／管理</button>
                </div>
              )}
              <div className="oc-db-connection-note">
                <span>
                  {usingSSH
                    ? `通过 SSH：${gateway?.name ?? '待选择节点'}`
                    : '直接连接数据库'}{' '}
                  · {draft.readOnly ? '已开启只读限制' : '权限由数据库账号决定'}
                </span>
                <button type="button" onClick={() => setSection('ssh')}>
                  配置连接方式
                </button>
              </div>
            </section>
            <section
              role="tabpanel"
              id="oc-db-panel-ssh"
              aria-labelledby="oc-db-section-ssh"
              hidden={section !== 'ssh'}
              data-section="ssh"
            >
              <div className="oc-db-section-heading">
                <h4>连接方式</h4>
                <p>通过跳板机访问内网数据库时，选择 SSH 隧道。</p>
              </div>
              <div className="oc-db-route-switch">
                <label>
                  <input
                    type="radio"
                    name="database-route"
                    checked={!usingSSH}
                    onChange={() => {
                      setUsingSSH(false);
                      change('gatewayId', '');
                    }}
                  />
                  直接连接
                </label>
                <label>
                  <input
                    type="radio"
                    name="database-route"
                    checked={usingSSH}
                    onChange={() => {
                      setUsingSSH(true);
                      invalidate();
                    }}
                  />
                  使用 SSH 隧道
                </label>
              </div>
              {usingSSH && (
                <>
                  <div className="oc-db-driver-choice">
                    <label>
                      SSH 隧道
                      <select
                        required
                        value={draft.gatewayId}
                        onChange={(e) => change('gatewayId', e.target.value)}
                      >
                        <option value="">请选择 SSH 连接</option>
                        {gateways.map((g) => (
                          <option key={g.id} value={g.id}>
                            {g.name === g.host ? g.host : `${g.name} · ${g.host}`}
                          </option>
                        ))}
                        {draft.gatewayId && !gateway && (
                          <option value={draft.gatewayId}>
                            原 SSH 连接不可用，请重新选择
                          </option>
                        )}
                      </select>
                    </label>
                    <button
                      type="button"
                      aria-label="刷新 SSH 连接列表"
                      title="刷新 SSH 连接列表"
                      disabled={loadingGateways}
                      onClick={() => void refreshGateways()}
                    >
                      <TbRefresh size={16} />
                    </button>
                  </div>
                  {gatewayError && (
                    <p role="alert" className="oc-db-error">
                      {gatewayError}
                    </p>
                  )}
                  <p className="oc-db-help">
                    数据库地址从最后一台 SSH 节点访问。数据库就在该节点本机时，可填写 127.0.0.1。
                  </p>
                  <p className="oc-db-help">
                    列表来自已保存的 SSH 连接。新增节点后，可在这里刷新列表。
                  </p>
                </>
              )}
              {!usingSSH && (
                <p className="oc-db-help">
                  程序从本机访问数据库地址，不经过 SSH 节点。
                </p>
              )}
            </section>
            <section
              role="tabpanel"
              id="oc-db-panel-options"
              aria-labelledby="oc-db-section-options"
              hidden={section !== 'options'}
              data-section="options"
            >
              <div className="oc-db-form-grid">
                <label>
                  连接名称（可选）
                  <input
                    maxLength={100}
                    value={draft.name}
                    onChange={(e) => change('name', e.target.value)}
                    placeholder={draft.port ? `${draft.host}:${draft.port}` : draft.host}
                  />
                </label>
                <div className="oc-db-labeled-field">
                  <div className="oc-db-field-label">
                    <label htmlFor={environmentInputId}>连接标记（可选）</label>
                    <ConnectionHelp
                      id={environmentHelpId}
                      label="连接标记说明"
                      text="显示在连接名称旁，仅作识别，不影响连接或权限。"
                    />
                  </div>
                  <input
                    id={environmentInputId}
                    value={draft.environment}
                    aria-describedby={environmentHelpId}
                    onChange={(e) => change('environment', e.target.value)}
                    placeholder="例如：生产、测试"
                  />
                </div>
              </div>
              <label>
                连接分组（可选）
                <input
                  value={draft.group ?? ''}
                  maxLength={100}
                  onChange={(e) => change('group', e.target.value)}
                  placeholder="例如：订单系统"
                />
              </label>
              <div className="oc-db-option-section">
                <label>
                  TLS 加密
                  <select
                    value={draft.tls}
                    onChange={(e) => change('tls', e.target.value)}
                  >
                    <option value="off">关闭</option>
                    <option value="required">启用并验证服务器证书</option>
                  </select>
                </label>
                <div className="oc-db-readonly-option">
                  <div className="oc-db-field-label">
                    <label className="oc-db-checkbox">
                      <input
                        type="checkbox"
                        checked={draft.readOnly}
                        aria-describedby={readOnlyHelpId}
                        disabled={!supportsReadOnly && !draft.readOnly}
                        onChange={(e) => change('readOnly', e.target.checked)}
                      />
                      限制为只读（可选）
                    </label>
                    <ConnectionHelp
                      id={readOnlyHelpId}
                      label="只读限制说明"
                      above
                      text={
                        supportsReadOnly
                          ? '默认关闭，读写权限由数据库账号决定。开启后限制写入，并使用只读事务。'
                          : '此驱动暂不支持程序只读限制。如需只读访问，请使用数据库只读账号。'
                      }
                    />
                  </div>
                  {!supportsReadOnly && (
                    <small className="oc-db-muted">当前驱动不支持</small>
                  )}
                </div>
                {draft.engine === 'jdbc' &&
                  selectedDriver?.dialect === 'mysql' && (
                    <label className="oc-db-checkbox oc-db-option-wide">
                      <input
                        type="checkbox"
                        checked={draft.jdbcAllowPublicKeyRetrieval ?? false}
                        onChange={(e) =>
                          change('jdbcAllowPublicKeyRetrieval', e.target.checked)
                        }
                      />
                      允许获取服务器 RSA 公钥
                    </label>
                  )}
              </div>
            </section>
          </fieldset>
        )}
      </DatabaseDialog>
      {managingDrivers && (
        <JDBCDriverDialog
          runtime={runtime}
          onClose={() => setManagingDrivers(false)}
          onChanged={setJDBC}
          onSelect={(driver) => {
            change('engine', 'jdbc');
            change('driverId', driver.id);
            if (!draft.port && driver.dialect === 'mysql') change('port', 3306);
            setChoosingDriver(false);
            setManagingDrivers(false);
          }}
        />
      )}
    </>
  );
}

function ConnectionHelp({
  id,
  label,
  text,
  above = false,
}: {
  id: string;
  label: string;
  text: string;
  above?: boolean;
}) {
  return (
    <span
      className={`oc-db-field-help${above ? ' oc-db-field-help-above' : ''}`}
    >
      <button type="button" aria-label={label} aria-describedby={id}>
        <TbInfoCircle size={14} />
      </button>
      <span role="tooltip" id={id}>
        {text}
      </span>
    </span>
  );
}
