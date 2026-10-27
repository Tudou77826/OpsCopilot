import { useEffect, useState } from 'react';
import { TbFileCode, TbCheck } from 'react-icons/tb';
import { DatabaseDialog } from './DatabaseDialog';
import { DatabaseError } from './DatabaseError';
import type {
  DatabaseJDBCDriver,
  DatabaseJDBCImport,
  DatabaseJDBCStatus,
  DatabaseRuntime,
} from './types';

const mysqlPreset: DatabaseJDBCImport = {
  name: 'MySQL Connector/J',
  className: 'com.mysql.cj.jdbc.Driver',
  urlTemplate: 'jdbc:mysql://{host}:{port}/{database}',
  dialect: 'mysql',
  paths: [],
};

const gaussPreset: DatabaseJDBCImport = {
  name: 'Gauss 100',
  className: 'com.huawei.gauss.jdbc.ZenithDriver',
  urlTemplate: 'jdbc:zenith:@{host}:{port}',
  dialect: 'generic',
  paths: [],
};

export function JDBCDriverDialog({
  runtime,
  onClose,
  onChanged,
  onSelect,
}: {
  runtime: DatabaseRuntime;
  onClose(): void;
  onChanged?(status: DatabaseJDBCStatus): void;
  onSelect?(driver: DatabaseJDBCDriver): void;
}) {
  const [status, setStatus] = useState<DatabaseJDBCStatus>();
  const [section, setSection] = useState('installed');
  const [draft, setDraft] = useState(gaussPreset);
  const [preset, setPreset] = useState('gauss');
  const [paths, setPaths] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [imported, setImported] = useState('');
  async function refresh() {
    const next = await runtime.jdbcStatus();
    setStatus(next);
    onChanged?.(next);
    return next;
  }
  useEffect(() => {
    void refresh()
      .then((next) => {
        if (!next.drivers.length) setSection('import');
      })
      .catch((e) => setError(String(e)));
  }, [runtime]);
  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError('');
    setImported('');
    try {
      await action();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  const files = paths
    .split(/\r?\n/)
    .map((p) => p.trim())
    .filter(Boolean);
  return (
    <DatabaseDialog
      title="驱动管理"
      subtitle={onSelect ? '管理本机 JDBC 驱动，连接信息在返回后继续填写。' : '查看已安装的 JDBC 驱动，或导入厂商提供的 JAR 文件。'}
      onClose={onClose}
      busy={busy}
      onSubmit={() => {
        if (section !== 'import' || !status?.available || !files.length) return;
        void run(async () => {
          const driver = await runtime.importJDBC({ ...draft, paths: files });
          await refresh();
          if (onSelect) {
            onSelect(driver);
            return;
          }
          setImported(driver.name);
          setSection('installed');
        });
      }}
      navigation={
        <div
          className="oc-db-modal-tabs"
          role="tablist"
          aria-label="驱动管理分类"
        >
          <button
            type="button"
            role="tab"
            id="oc-db-installed-tab"
            aria-controls="oc-db-installed-panel"
            aria-selected={section === 'installed'}
            tabIndex={section === 'installed' ? 0 : -1}
            disabled={busy}
            onClick={() => setSection('installed')}
          >
            已安装{status ? ` · ${status.drivers.length}` : ''}
          </button>
          <button
            type="button"
            role="tab"
            id="oc-db-import-tab"
            aria-controls="oc-db-import-panel"
            aria-selected={section === 'import'}
            tabIndex={section === 'import' ? 0 : -1}
            disabled={busy}
            onClick={() => setSection('import')}
          >
            导入驱动
          </button>
        </div>
      }
      feedback={
        error ? (
          <DatabaseError message={error} />
        ) : imported ? (
          <div className="oc-db-test-success" role="status">
            <TbCheck />
            已导入 {imported}
          </div>
        ) : busy ? (
          <p role="status">正在处理，请稍候…</p>
        ) : undefined
      }
      actions={
        <>
          <button type="button" disabled={busy} onClick={onClose}>
            完成
          </button>
          {section === 'import' && (
            <button
              className="oc-db-primary"
              disabled={busy || !status?.available || !files.length}
              type="submit"
            >
              {busy ? '正在处理…' : '导入并验证'}
            </button>
          )}
        </>
      }
    >
      <p
        className={`oc-db-runtime-status ${status?.available ? 'oc-db-muted' : 'oc-db-write-note'}`}
        role="status"
      >
        {status?.message ?? '正在检查运行组件…'}
      </p>
      {section === 'installed' ? (
        <section
          role="tabpanel"
          id="oc-db-installed-panel"
          aria-labelledby="oc-db-installed-tab"
        >
          {status?.drivers.length ? (
            <ul className="oc-db-driver-list">
              {status.drivers.map((driver) => (
                <li key={driver.id}>
                  <span>
                    <strong>{driver.name}</strong>
                    <small>
                      {driver.className} · v{driver.version} ·{' '}
                      {driver.jars.length} 个文件
                    </small>
                  </span>
                  <div className="oc-db-driver-actions">
                    {onSelect && (
                      <button
                        type="button"
                        disabled={busy || !status.available}
                        onClick={() => onSelect(driver)}
                      >
                        使用驱动
                      </button>
                    )}
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        void run(async () => {
                          await runtime.deleteJDBC(driver.id);
                          await refresh();
                        })
                      }
                    >
                      移除
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <div className="oc-db-driver-empty">
              {status
                ? '尚未导入驱动。请选择厂商提供的 JDBC JAR 文件。'
                : '正在加载驱动…'}
            </div>
          )}
        </section>
      ) : (
        <section
          role="tabpanel"
          id="oc-db-import-panel"
          aria-labelledby="oc-db-import-tab"
        >
          <fieldset
            className="oc-db-import-fields"
            disabled={busy || !status?.available}
          >
            <div className="oc-db-form-grid">
              <label>
                驱动类型
                <select
                  value={preset}
                  onChange={(e) => {
                    setError('');
                    setPreset(e.target.value);
                    setDraft(
                      e.target.value === 'gauss' ? gaussPreset : e.target.value === 'mysql'
                        ? mysqlPreset
                        : {
                            name: '',
                            className: '',
                            urlTemplate: 'jdbc:zenith:@{host}:{port}',
                            dialect: 'generic',
                            paths: [],
                          },
                    );
                  }}
                >
                  <option value="gauss">Gauss 100</option>
                  <option value="mysql">MySQL（需导入 Connector/J）</option>
                  <option value="generic">其他 JDBC 驱动（通用能力）</option>
                </select>
              </label>
              <label>
                驱动名称
                <input
                  required
                  maxLength={200}
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                />
              </label>
            </div>
            <details className="oc-db-manual-paths" open={preset === 'generic' ? true : undefined}>
              <summary>高级驱动设置</summary>
              <label>
                驱动类
                <input
                  required
                  value={draft.className}
                  onChange={(e) => setDraft({ ...draft, className: e.target.value })}
                />
              </label>
              <label>
                URL 模板
                <input
                  required
                  value={draft.urlTemplate}
                  readOnly={draft.dialect === 'mysql'}
                  onChange={(e) => setDraft({ ...draft, urlTemplate: e.target.value })}
                />
              </label>
            </details>
            <div className="oc-db-file-picker">
              <div>
                <strong>驱动文件</strong>
                <span>
                  {files.length
                    ? `已选择 ${files.length} 个 JAR`
                    : '选择厂商提供的 JAR，可包含依赖文件'}
                </span>
              </div>
              <button
                type="button"
                onClick={() =>
                  void run(async () => {
                    const selected = await runtime.chooseJDBCJars();
                    if (selected?.length) setPaths(selected.join('\n'));
                  })
                }
              >
                选择 JAR…
              </button>
            </div>
            {files.length > 0 && (
              <ul className="oc-db-jar-list">
                {files.map((file, i) => (
                  <li key={`${file}-${i}`} title={file}>
                    <TbFileCode />
                    <span>{file.split(/[\\/]/).pop()}</span>
                  </li>
                ))}
              </ul>
            )}
            <details className="oc-db-manual-paths">
              <summary>手动填写文件路径</summary>
              <label>
                驱动文件路径
                <textarea
                  rows={2}
                  value={paths}
                  onChange={(e) => setPaths(e.target.value)}
                  placeholder="本机绝对路径，每行一个 JAR"
                />
              </label>
            </details>
          </fieldset>
          <p className="oc-db-help">
            仅导入可信厂商的驱动。账号密码在连接设置中填写。
          </p>
          {draft.dialect === 'generic' && (
            <p className="oc-db-help">
              通用驱动提供连接、查询和标准元数据；数据库特有的
              DDL、分页与只读能力取决于驱动支持。
            </p>
          )}
        </section>
      )}
    </DatabaseDialog>
  );
}
