import { useEffect, useMemo, useRef, useState } from 'react';
import {
  TbDatabase,
  TbPlus,
  TbRefresh,
  TbPlayerPlay,
  TbPlayerStop,
  TbPlugConnected,
  TbPlugConnectedX,
  TbTable,
  TbSettings,
  TbTrash,
  TbDownload,
  TbCode,
  TbX,
  TbAlertCircle,
  TbFileCode,
  TbChevronLeft,
  TbChevronRight,
} from 'react-icons/tb';
import { IconButton } from '../controls/IconButton';
import { SQLEditor, type SQLEditorRef } from './SQLEditor';
import { DatabaseError } from './DatabaseError';
import { DatabaseExecutionHistory } from './DatabaseExecutionHistory';
import { DatabaseMenu } from './DatabaseMenu';
import { RenameQueryDialog } from './RenameQueryDialog';
import { ConnectionFailurePanel } from './ConnectionFailurePanel';
import { ResultGrid, resultCSV } from './ResultGrid';
import { emptyProfile } from './ConnectionDialog';
import { DatabaseExplorer } from './DatabaseExplorer';
import { ExplorerSplitter } from './ExplorerSplitter';
import { useDatabaseWorkspace } from './useDatabaseWorkspace';
import { objectKey, type QueryTab } from './workspaceModel';
import type { DatabaseProfile, DatabaseRuntime } from './types';
import './database.css';
import './explorer.css';
import type { ManagedDatabase } from '../../workspace/database';

export function DatabaseWorkspace({ runtime,managed }: { runtime: DatabaseRuntime;managed?:ManagedDatabase }) {
  const workspace = useRef<HTMLElement>(null);
  const model = useDatabaseWorkspace(runtime,managed);
  const {
    profiles,
    connections,
    tabs,
    activeTab,
    setActiveTab,
    update,
    current,
    error,
    setError,
    closeTab,
    removeProfile,
  } = model;
  const tab = tabs.find((t) => t.id === activeTab);
  const profileId = tab?.profileId ?? model.selectedProfile;
  const profile = profiles.find((p) => p.id === profileId);
  const connection = connections[profileId];
  const failure = model.connectionFailures[profileId];
  const busyProfile = model.connecting[profileId];
  const metadataBusy = model.metadataBusy[profileId];
  const profileRunning = model.running(profileId);
  const [editing, setEditing] = useState<DatabaseProfile | null>(null);
  const [limit, setLimit] = useState(1000);
  const [timeout, setTimeoutValue] = useState(30);
  const [editorPercent, setEditorPercent] = useState(42);
  const [tabMenu, setTabMenu] = useState<{
    id: string;
    x: number;
    y: number;
    trigger: HTMLElement;
  } | null>(null);
  const [renaming, setRenaming] = useState<QueryTab | null>(null);
  const menuTab = tabs.find((t) => t.id === tabMenu?.id);
  const dismissTabMenu = () => {
    tabMenu?.trigger.focus();
    setTabMenu(null);
  };
  const openTabMenu = (
    event: React.MouseEvent<HTMLElement> | React.KeyboardEvent<HTMLElement>,
    target: QueryTab,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    const rect = event.currentTarget.getBoundingClientRect();
    setTabMenu({
      id: target.id,
      x: ('clientX' in event && event.clientX) || rect.left + 12,
      y: ('clientY' in event && event.clientY) || rect.bottom,
      trigger:
        event.currentTarget.querySelector<HTMLButtonElement>('button') ??
        event.currentTarget,
    });
  };
  const editor = useRef<SQLEditorRef>(null);
  useEffect(()=>{
    if(managed?.workspace.pending&&tab?.id===`ai-query-${managed.workspace.id}`&&editor.current?.value()===managed.workspace.sql){managed.onPresented(managed.workspace.sql);}
  },[managed?.workspace.pending,tab?.sql,tab?.id]);
  const content = useRef<HTMLDivElement>(null);
  const tabStrip = useRef<HTMLElement>(null);
  useEffect(() => {
    tabStrip.current
      ?.querySelector<HTMLElement>('.is-selected')
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  }, [tab?.id]);
  model.options.current = { limit, timeout };
  const connect = model.openConnection;
  const disconnect = model.disconnect;
  const newQuery = () => {
    if (profileId)
      model.newQuery(
        profileId,
        tab?.schema ?? connection?.session.database ?? '',
      );
  };
  const tableView = (view: QueryTab['view']) =>
    tab && model.tableView(tab, view);
  const execute = (
    target: QueryTab,
    mode?: 'script',
    offset = target.offset,
  ) => {
    const selected =
      target.id === tab?.id ? (editor.current?.selection() ?? '') : '';
    return model.execute(
      target,
      mode === 'script'
        ? { mode: 'script' }
        : selected.length > 0
          ? { mode: 'selection', sql: selected }
          : {
              mode: 'statement',
              cursorOffset: editor.current?.cursorOffset() ?? 0,
            },
      offset,
    );
  };
  function resizeEditor(e: React.PointerEvent<HTMLDivElement>) {
    e.currentTarget.setPointerCapture(e.pointerId);
    const move = (ev: PointerEvent) => {
      const rect = content.current?.getBoundingClientRect();
      if (rect)
        setEditorPercent(
          Math.max(
            20,
            Math.min(75, ((ev.clientY - rect.top) / rect.height) * 100),
          ),
        );
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up, { once: true });
  }
  let result = tab?.result?.sets[tab.resultIndex];
  if (tab?.view === 'columns') result = tab.structure?.columns;
  if (tab?.view === 'indexes') result = tab.structure?.indexes;
  function exportResult() {
    if (!result) return;
    const blob = new Blob([resultCSV(result)], {
      type: 'text/csv;charset=utf-8',
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${tab?.table ?? 'query-result'}.csv`;
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  const completionSchema = useMemo(
    () =>
      Object.fromEntries(
        (connection?.tables[tab?.schema ?? ''] ?? []).map((t) => [
          t.name,
          connection?.structures[
            objectKey(tab?.schema ?? '', t.name)
          ]?.columns.rows.map((row) => row[0] ?? '') ?? [],
        ]),
      ),
    [connection, tab?.schema],
  );

  return (
    <section
      ref={workspace}
      className="oc-db-workspace"
      aria-label="数据库工作区"
      onKeyDown={(e) => {
        if (
          (e.ctrlKey || e.metaKey) &&
          ['Enter', 's', 'w', 'k'].includes(e.key)
        )
          e.stopPropagation();
      }}
    >
      <DatabaseExplorer
        model={model}
        runtime={runtime}
        editing={editing}
        setEditing={setEditing}
        onInsert={(text, pid, database) => {
          if (
            tab?.view === 'query' &&
            tab.profileId === pid &&
            (tab.schema === database || !tab.schema)
          ) {
            if (!tab.schema) update(tab.id, { schema: database });
            editor.current?.insert(text);
          } else model.newQuery(pid, database, text);
        }}
      />
      <ExplorerSplitter container={workspace} />
      <main className="oc-db-main">
        <header className="oc-db-context">
          <div>
            <TbDatabase size={17} />
            <strong>{profile?.name ?? '数据库'}</strong>
            {profile?.environment && (
              <span
                className={`oc-db-env ${profile.environment.includes('生产') ? 'is-production' : ''}`}
              >
                {profile.environment}
              </span>
            )}
            {connection && (
              <small title={connection.session.version}>
                {profile?.engine === 'jdbc' ? 'JDBC · ' : 'MySQL '}
                {connection.session.version.split('-')[0]} ·{' '}
                {connection.session.readOnly ? '只读' : '可写'}
              </small>
            )}
          </div>
          <div>
            {profile && (
              <>
                <button
                  aria-label={connection ? '断开数据库连接' : '连接数据库'}
                  title={connection ? '断开连接' : '连接'}
                  disabled={!!busyProfile || profileRunning || !!metadataBusy}
                  onClick={() =>
                    void (connection
                      ? disconnect(profile.id)
                      : connect(profile))
                  }
                >
                  {connection ? (
                    <TbPlugConnectedX size={16} />
                  ) : (
                    <TbPlugConnected size={16} />
                  )}
                </button>
                <button
                  aria-label="编辑数据库连接"
                  title="编辑连接"
                  disabled={profileRunning || !!metadataBusy || !!busyProfile}
                  onClick={() => setEditing(profile)}
                >
                  <TbSettings size={16} />
                </button>
                <button
                  aria-label="删除数据库连接"
                  title="删除连接"
                  disabled={profileRunning || !!metadataBusy || !!busyProfile}
                  onClick={() => void removeProfile(profile)}
                >
                  <TbTrash size={15} />
                </button>
              </>
            )}
          </div>
        </header>
        {error && (
          <div className="oc-db-banner">
            <DatabaseError message={error} />
            <IconButton label="关闭错误提示" onClick={() => setError('')}><TbX size={18} /></IconButton>
          </div>
        )}
        <nav ref={tabStrip} className="oc-db-tabs" aria-label="数据库查询标签">
          {profiles
            .filter((p) => model.connectionFailures[p.id])
            .map((p) => (
              <div
                key={`failure-${p.id}`}
                className={!tab && profileId === p.id ? 'is-selected' : ''}
              >
                <button
                  aria-label={`连接失败 · ${p.name}`}
                  onClick={() => {
                    model.setSelectedProfile(p.id);
                    setActiveTab('');
                  }}
                >
                  <TbAlertCircle size={14} />
                  连接失败<small>{p.name}</small>
                </button>
              </div>
            ))}
          {tabs.map((t) => (
            <div
              key={t.id}
              className={tab?.id === t.id ? 'is-selected' : ''}
              onContextMenu={(e) => openTabMenu(e, t)}
              onKeyDown={(e) => {
                if ((e.shiftKey && e.key === 'F10') || e.key === 'ContextMenu')
                  openTabMenu(e, t);
              }}
            >
              <button
                aria-label={`${t.title} · ${profiles.find((p) => p.id === t.profileId)?.name ?? '连接已删除'}`}
                title={`${profiles.find((p) => p.id === t.profileId)?.name ?? '连接已删除'} / ${t.schema || '未选择数据库'}`}
                onClick={() => setActiveTab(t.id)}
              >
                {t.table ? <TbTable size={13} /> : <TbCode size={14} />}
                {t.title}
                <small>
                  {profiles.find((p) => p.id === t.profileId)?.name}
                </small>
                {t.pending && <span className="oc-db-running-dot" />}
              </button>
              <button
                aria-label={`关闭${t.title} · ${profiles.find((p) => p.id === t.profileId)?.name ?? '连接已删除'}`}
                disabled={!!t.pending}
                onClick={() => closeTab(t)}
              >
                <TbX size={12} />
              </button>
            </div>
          ))}
          <button
            aria-label="新建 SQL 查询"
            title="新建查询"
            disabled={!profile}
            onClick={() => newQuery()}
          >
            <TbPlus size={16} />
          </button>
        </nav>
        {!connection && profile && failure ? (
          <ConnectionFailurePanel
            profile={profile}
            failure={failure}
            busy={!!busyProfile}
            onRetry={() => void connect(profile)}
            onEdit={() => setEditing(profile)}
          />
        ) : !connection && !(managed && tab) ? (
          <div className="oc-db-empty">
            <TbDatabase size={40} />
            <h3>
              {busyProfile
                ? '正在连接…'
                : profile
                  ? profile.name
                  : '打开数据库连接'}
            </h3>
            <p>
              {profile
                ? `${profile.host}:${profile.port} · ${profile.user}${profile.gatewayId ? ' · 通过 SSH' : ''}`
                : '从左侧选择连接，或添加数据库连接。'}
            </p>
            {profile ? (
              <button
                className="oc-db-primary"
                disabled={!!busyProfile || profileRunning || !!metadataBusy}
                onClick={() => void connect(profile)}
              >
                {busyProfile ? '正在连接…' : '连接数据库'}
              </button>
            ) : (
              <button
                className="oc-db-primary"
                onClick={() => setEditing(emptyProfile())}
              >
                添加连接
              </button>
            )}
          </div>
        ) : (
          <>
            {!tab ? (
              <div className="oc-db-empty">
                <button onClick={() => newQuery()}>新建 SQL 查询</button>
              </div>
            ) : (
              <>
                <div className="oc-db-query-toolbar">
                  <select
                    aria-label="当前数据库"
                    value={tab.schema}
                    disabled={profileRunning || !!metadataBusy || !!tab.table}
                    onChange={(e) => update(tab.id, { schema: e.target.value })}
                  >
                    <option value="">选择数据库</option>
                    {(connection?.schemas??managed?.workspace.schemas??[]).map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                  {tab.table ? (
                    <>
                      <div className="oc-db-view-tabs">
                        {(['data', 'columns', 'indexes', 'ddl'] as const).map(
                          (v) => (
                            <button
                              disabled={profileRunning || !!metadataBusy}
                              className={tab.view === v ? 'is-selected' : ''}
                              key={v}
                              onClick={() => void tableView(v)}
                            >
                              {
                                {
                                  data: '数据',
                                  columns: '字段',
                                  indexes: '索引',
                                  ddl: 'DDL',
                                }[v]
                              }
                            </button>
                          ),
                        )}
                      </div>
                      <button
                        title={
                          tab.view === 'data' ? '刷新表数据' : '刷新表结构'
                        }
                        aria-label={
                          tab.view === 'data' ? '刷新表数据' : '刷新表结构'
                        }
                        disabled={profileRunning || !!metadataBusy}
                        onClick={() =>
                          void model.tableView(tab, tab.view, true)
                        }
                      >
                        <TbRefresh size={15} />
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        className="oc-db-run"
                        disabled={!connection || profileRunning || !!metadataBusy}
                        onClick={() => void execute(tab)}
                        title="执行选中内容；未选中时仅执行光标所在语句（Ctrl+Enter）"
                      >
                        <TbPlayerPlay size={14} />
                        执行 SQL
                      </button>
                      <button
                        className="oc-db-run-script"
                        disabled={!connection || profileRunning || !!metadataBusy}
                        title="执行整页脚本，忽略选区；遇错停止（Ctrl+Shift+Enter）"
                        onClick={() => void execute(tab, 'script')}
                      >
                        <TbFileCode size={14} />
                        执行脚本
                      </button>
                      <button
                        disabled={!tab.pending}
                        onClick={() =>
                          tab.pending && void runtime.cancel(tab.pending)
                        }
                      >
                        <TbPlayerStop size={14} />
                        停止
                      </button>
                      <span className="oc-db-toolbar-spacer" />
                      <label>
                        上限
                        <select
                          aria-label="查询行数上限"
                          value={limit}
                          onChange={(e) => setLimit(Number(e.target.value))}
                        >
                          {[100, 1000, 5000].map((n) => (
                            <option key={n} value={n}>
                              {n} 行
                            </option>
                          ))}
                        </select>
                      </label>
                      <label>
                        超时
                        <select
                          aria-label="查询超时"
                          value={timeout}
                          onChange={(e) =>
                            setTimeoutValue(Number(e.target.value))
                          }
                        >
                          {[10, 30, 60, 120].map((n) => (
                            <option key={n} value={n}>
                              {n} 秒
                            </option>
                          ))}
                        </select>
                      </label>
                    </>
                  )}
                </div>
                <div className="oc-db-query-content" ref={content}>
                  {tab.view === 'query' && (
                    <>
                      <div
                        style={{ height: `${editorPercent}%`, minHeight: 90 }}
                      >
                        <SQLEditor
                          key={tab.id}
                          ref={editor}
                          value={tab.sql}
                          onChange={(sql) => {update(tab.id, { sql });if(managed&&tab.id===`ai-query-${managed.workspace.id}`&&!managed.workspace.ownerTurn)managed.onEdit(sql);}}
                          onRun={() => {
                            void execute(
                              current.current.tabs.find(
                                (t) => t.id === tab.id,
                              ) ?? tab,
                            );
                          }}
                          schema={completionSchema}
                          onRunScript={() => {
                            void execute(
                              current.current.tabs.find(
                                (t) => t.id === tab.id,
                              ) ?? tab,
                              'script',
                            );
                          }}
                        />
                      </div>
                      <div
                        className="oc-db-splitter"
                        role="separator"
                        aria-label="调整 SQL 编辑器高度"
                        aria-orientation="horizontal"
                        tabIndex={0}
                        onPointerDown={resizeEditor}
                        onKeyDown={(e) => {
                          if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
                            e.preventDefault();
                            setEditorPercent((v) =>
                              Math.max(
                                20,
                                Math.min(
                                  75,
                                  v + (e.key === 'ArrowDown' ? 5 : -5),
                                ),
                              ),
                            );
                          }
                        }}
                      />
                    </>
                  )}
                  {managed&&tab.id===`ai-query-${managed.workspace.id}`&&managed.workspace.blocks.length>0&&<DatabaseExecutionHistory blocks={managed.workspace.blocks} onSelect={block=>update(tab.id,{result:block.result,error:block.error,resultIndex:0})}/>}
                  {tab.error && (
                    <div className="oc-db-banner">
                      <DatabaseError message={tab.error} />
                    </div>
                  )}
                  {tab.view === 'ddl' ? (
                    <pre className="oc-db-ddl">
                      {tab.structure?.ddl ?? (metadataBusy ? '正在读取…' : '')}
                    </pre>
                  ) : (
                    <div className="oc-db-results">
                      <div className="oc-db-result-toolbar">
                        <div>
                          {tab.view === 'query' &&
                          (tab.result?.sets.length ?? 0) > 1 ? (
                            tab.result!.sets.map((_, i) => (
                              <button
                                className={
                                  tab.resultIndex === i ? 'is-selected' : ''
                                }
                                onClick={() =>
                                  update(tab.id, { resultIndex: i })
                                }
                                key={i}
                              >
                                结果 {i + 1}
                              </button>
                            ))
                          ) : (
                            <span>
                              {tab.view === 'columns'
                                ? '字段结构'
                                : tab.view === 'indexes'
                                  ? '索引'
                                  : '查询结果'}
                            </span>
                          )}
                        </div>
                        <span className="oc-db-toolbar-spacer" />
                        {result && (
                          <>
                            <small>
                              {result.columns.length
                                ? `${result.rows.length} 行`
                                : `影响 ${result.affectedRows} 行`}
                              {tab.result
                                ? ` · ${tab.result.durationMs} ms`
                                : ''}
                              {result.truncated ? ' · 已达返回上限' : ''}
                            </small>
                            <button
                              title="导出当前结果为 CSV"
                              aria-label="导出 CSV"
                              disabled={!result.columns.length}
                              onClick={exportResult}
                            >
                              <TbDownload size={15} />
                            </button>
                          </>
                        )}
                      </div>
                      {result?.columns.length ? (
                        <ResultGrid
                          key={`${tab.id}:${tab.view}:${tab.resultIndex}`}
                          result={result}
                        />
                      ) : (
                        <div className="oc-db-result-empty">
                          {metadataBusy && tab.table
                            ? '正在读取对象…'
                            : tab.pending
                              ? '正在执行…'
                              : result
                                ? `执行完成，影响 ${result.affectedRows} 行`
                                : '执行 SQL 后在这里查看结果'}
                        </div>
                      )}
                      {tab.table && tab.view === 'data' && (
                        <div className="oc-db-pagination">
                          <IconButton label="上一页"
                            disabled={profileRunning || tab.offset === 0}
                            onClick={() =>
                              void execute(
                                tab,
                                undefined,
                                Math.max(0, tab.offset - 100),
                              )
                            }
                          >
                            <TbChevronLeft size={18} />
                          </IconButton>
                          <span>
                            {tab.offset + 1}–
                            {tab.offset + (result?.rows.length ?? 0)} · 每页 100
                            行
                          </span>
                          <IconButton label="下一页"
                            disabled={profileRunning || !result?.truncated}
                            onClick={() =>
                              void execute(tab, undefined, tab.offset + 100)
                            }
                          >
                            <TbChevronRight size={18} />
                          </IconButton>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </>
            )}
          </>
        )}
        <footer className="oc-db-status">
          <span className={connection ? 'is-online' : ''}>●</span>
          {connection
            ? `${profile?.host}:${profile?.port} · ${profile?.user}`
            : '未连接'}
          <span className="oc-db-toolbar-spacer" />
          <span title="同一连接下的查询标签共用数据库会话；事务和会话变量也会共享。">
            {profileRunning
              ? '正在执行'
              : metadataBusy
                ? '正在读取对象'
                : connection
                  ? '就绪 · 共享会话'
                  : '数据库'}
          </span>
          {connection && tab?.schema ? ` · ${tab.schema}` : ''}
        </footer>
      </main>
      {tabMenu && menuTab && (
        <DatabaseMenu
          x={tabMenu.x}
          y={tabMenu.y}
          label="查询页签操作"
          onClose={dismissTabMenu}
          items={[
            {
              label: '重命名查询…',
              disabled: !!menuTab.table,
              action: () => setRenaming(menuTab),
            },
            {
              label: '复制查询',
              disabled: !!menuTab.table,
              action: () => model.duplicateQuery(menuTab),
            },
            {
              label: '关闭当前',
              separator: true,
              disabled: !!menuTab.pending,
              action: () => closeTab(menuTab),
            },
            ...(
              [
                ['关闭其他', tabs.filter((t) => t.id !== menuTab.id)],
                [
                  '关闭右侧',
                  tabs.slice(tabs.findIndex((t) => t.id === menuTab.id) + 1),
                ],
                ['关闭全部', tabs],
              ] as const
            ).map(([label, targets]) => ({
              label,
              disabled: !targets.some((t) => !t.pending),
              action: () => model.closeTabs(targets.map((t) => t.id)),
            })),
          ]}
        />
      )}
      {renaming && (
        <RenameQueryDialog
          title={renaming.title}
          onSave={(title) => update(renaming.id, { title })}
          onClose={() => setRenaming(null)}
        />
      )}
    </section>
  );
}
