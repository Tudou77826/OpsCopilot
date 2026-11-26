import { useEffect, useMemo, useRef, useState } from 'react';
import {
  TbDatabase,
  TbRefresh,
  TbPlayerPlay,
  TbPlayerStop,
  TbTable,
  TbCode,
  TbX,
  TbAlertCircle,
  TbFileCode,
  TbChevronLeft,
  TbChevronRight,
  TbChevronDown,
  TbSearch,
  TbUpload,
  TbCopy,
  TbCheck,
  TbArrowBackUp,
} from 'react-icons/tb';
import { IconButton } from '../controls/IconButton';
import { SQLEditor, type SQLEditorRef } from './SQLEditor';
import { DatabaseError } from './DatabaseError';
import { DatabaseExecutionHistory } from './DatabaseExecutionHistory';
import { DatabaseMenu } from './DatabaseMenu';
import { RenameQueryDialog } from './RenameQueryDialog';
import { ConnectionFailurePanel } from './ConnectionFailurePanel';
import { ResultGrid, resultCSV } from './ResultGrid';
import {
  CellUpdateConfirmDialog,
  type PendingBatchUpdate,
} from './CellUpdateConfirmDialog';
import {
  applyPendingEditsToResult,
  buildBatchUpdateSQL,
  extractPrimaryKeys,
  inferEditableTarget,
  type SQLDialect,
} from './resultGridUtils';
import { emptyProfile } from './ConnectionDialog';
import { DatabaseExplorer } from './DatabaseExplorer';
import { ExplorerSplitter } from './ExplorerSplitter';
import { useDatabaseWorkspace } from './useDatabaseWorkspace';
import { errorText, objectKey, type QueryTab } from './workspaceModel';
import type {
  DatabaseProfile,
  DatabaseProfileInput,
  DatabaseRuntime,
} from './types';
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
  } = model;
  const tab = tabs.find((t) => t.id === activeTab);
  const profileId = tab?.profileId ?? model.selectedProfile;
  const profile = profiles.find((p) => p.id === profileId);
  const connection = connections[profileId];
  const failure = model.connectionFailures[profileId];
  const busyProfile = model.connecting[profileId];
  const metadataBusy = model.metadataBusy[profileId];
  const profileRunning = model.running(profileId);
  const [editing, setEditing] = useState<DatabaseProfileInput | null>(null);
  const [limit, setLimit] = useState(1000);
  const [timeout, setTimeoutValue] = useState(30);
  const [editorPercent, setEditorPercent] = useState(42);
  const [tabMenu, setTabMenu] = useState<
    | {
        kind: 'query';
        id: string;
        x: number;
        y: number;
        trigger: HTMLElement;
      }
    | {
        kind: 'failure';
        profileId: string;
        x: number;
        y: number;
        trigger: HTMLElement;
      }
    | null
  >(null);
  const [renaming, setRenaming] = useState<QueryTab | null>(null);
  const [resultFilter, setResultFilter] = useState('');
  const [filteredCount, setFilteredCount] = useState<number | null>(null);
  const [pendingEdits, setPendingEdits] = useState<
    Record<number, Record<number, string | null>>
  >({});
  const [pendingBatch, setPendingBatch] = useState<PendingBatchUpdate | null>(
    null,
  );
  const [sqlCopied, setSqlCopied] = useState(false);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [updateError, setUpdateError] = useState('');
  const failureProfiles = profiles.filter((p) => model.connectionFailures[p.id]);
  const menuTab =
    tabMenu?.kind === 'query'
      ? tabs.find((t) => t.id === tabMenu.id)
      : undefined;
  const menuFailureProfile =
    tabMenu?.kind === 'failure'
      ? failureProfiles.find((p) => p.id === tabMenu.profileId)
      : undefined;
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
      kind: 'query',
      id: target.id,
      x: ('clientX' in event && event.clientX) || rect.left + 12,
      y: ('clientY' in event && event.clientY) || rect.bottom,
      trigger:
        event.currentTarget.querySelector<HTMLButtonElement>('button') ??
        event.currentTarget,
    });
  };
  const openFailureTabMenu = (
    event: React.MouseEvent<HTMLElement> | React.KeyboardEvent<HTMLElement>,
    target: DatabaseProfile,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    const rect = event.currentTarget.getBoundingClientRect();
    setTabMenu({
      kind: 'failure',
      profileId: target.id,
      x: ('clientX' in event && event.clientX) || rect.left + 12,
      y: ('clientY' in event && event.clientY) || rect.bottom,
      trigger:
        event.currentTarget.querySelector<HTMLButtonElement>('button') ??
        event.currentTarget,
    });
  };
  const closeFailureTab = (targetProfileId: string) => {
    const idx = failureProfiles.findIndex((p) => p.id === targetProfileId);
    const remainingFailures = failureProfiles.filter(
      (p) => p.id !== targetProfileId,
    );
    model.clearConnectionFailure(targetProfileId);
    if (!tab && profileId === targetProfileId) {
      const nextFailure =
        remainingFailures[Math.min(idx, remainingFailures.length - 1)];
      if (nextFailure) {
        model.setSelectedProfile(nextFailure.id);
      } else if (tabs.length > 0) {
        setActiveTab(tabs[tabs.length - 1].id);
      } else {
        model.setSelectedProfile('');
      }
    }
  };
  const attemptedCompletionSchemas = useRef(new Set<string>());
  useEffect(() => {
    if (
      !tab ||
      tab.table ||
      !tab.schema ||
      !connection ||
      connection.tables[tab.schema] ||
      profileRunning ||
      metadataBusy
    ) {
      return;
    }
    const key = `${connection.session.id}:${tab.schema}`;
    if (attemptedCompletionSchemas.current.has(key)) return;
    attemptedCompletionSchemas.current.add(key);
    void model.loadTables(tab.profileId, tab.schema, false, true);
  }, [
    tab?.id,
    tab?.table,
    tab?.profileId,
    tab?.schema,
    connection,
    profileRunning,
    metadataBusy,
  ]);
  const editor = useRef<SQLEditorRef>(null);
  const revealed = useRef('');
  useEffect(() => {
    const w = managed?.workspace;
    if (!w?.revealNonce || !w.revealBlock || revealed.current === w.revealNonce) return;
    const queryId = `ai-query-${w.id}`;
    if (tab?.id !== queryId) { setActiveTab(queryId); return; }
    const block = w.blocks.find(item => item.id === w.revealBlock);
    if (!block) return;
    revealed.current = w.revealNonce;
    update(queryId, {result:block.result, error:block.error, resultIndex:0});
  }, [managed?.workspace.revealNonce, tab?.id]);
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
  const resultViewKey = `${tab?.id ?? ''}:${tab?.view ?? ''}:${tab?.resultIndex ?? 0}`;
  useEffect(() => {
    setResultFilter('');
    setFilteredCount(null);
    setPendingEdits({});
    setPendingBatch(null);
  }, [resultViewKey, result]);

  const dirtyCellCount = useMemo(
    () =>
      Object.values(pendingEdits).reduce(
        (sum, rowMap) => sum + Object.keys(rowMap).length,
        0,
      ),
    [pendingEdits],
  );

  const sqlDialect: SQLDialect =
    profile?.engine === 'jdbc' &&
    !(profile.driverId ?? '').toLowerCase().includes('mysql')
      ? 'standard'
      : 'mysql';
  const editableTarget = useMemo(() => {
    if (!tab) return null;
    if (tab.table && tab.view === 'data') {
      return { schema: tab.schema, table: tab.table };
    }
    if (tab.view === 'query' && result?.statement) {
      return inferEditableTarget(result.statement, tab.schema);
    }
    return null;
  }, [tab?.table, tab?.view, tab?.schema, result?.statement]);

  const canEditTableData = Boolean(
    editableTarget &&
      connection &&
      !connection.session.readOnly &&
      !profileRunning &&
      !metadataBusy,
  );

  function stageCellEdit(
    origRowIndex: number,
    colIndex: number,
    newValue: string | null,
  ) {
    if (!result) return;
    const origValue = result.rows[origRowIndex]?.[colIndex] ?? null;
    setPendingEdits((prev) => {
      const next = { ...prev };
      const rowEdits = { ...(next[origRowIndex] ?? {}) };
      if (newValue === origValue) {
        delete rowEdits[colIndex];
      } else {
        rowEdits[colIndex] = newValue;
      }
      if (Object.keys(rowEdits).length === 0) {
        delete next[origRowIndex];
      } else {
        next[origRowIndex] = rowEdits;
      }
      return next;
    });
  }

  function revertCellEdit(origRowIndex: number, colIndex: number) {
    setPendingEdits((prev) => {
      if (!prev[origRowIndex]) return prev;
      const next = { ...prev };
      const rowEdits = { ...next[origRowIndex] };
      delete rowEdits[colIndex];
      if (Object.keys(rowEdits).length === 0) {
        delete next[origRowIndex];
      } else {
        next[origRowIndex] = rowEdits;
      }
      return next;
    });
  }

  async function buildCurrentBatchUpdate(): Promise<PendingBatchUpdate | null> {
    if (!tab || !editableTarget || !connection || !result || dirtyCellCount === 0) {
      return null;
    }
    const targetSchema = editableTarget.schema || tab.schema;
    const targetTable = editableTarget.table;
    let structure: typeof tab.structure =
      (tab.table === targetTable ? tab.structure : undefined) ??
      connection.structures[objectKey(targetSchema, targetTable)];
    if (!structure && targetSchema) {
      try {
        structure = await model.loadStructure(
          tab.profileId,
          targetSchema,
          targetTable,
        );
      } catch {
        // Structure fetch failed; fall back to full-row WHERE with explicit warning.
      }
    }
    const primaryKeys = extractPrimaryKeys(structure);
    const built = buildBatchUpdateSQL({
      schema: targetSchema,
      table: targetTable,
      dialect: sqlDialect,
      pendingEdits,
      columns: result.columns,
      rows: result.rows,
      primaryKeys,
    });
    if (built.rowCount === 0) return null;
    return {
      rowCount: built.rowCount,
      cellCount: built.cellCount,
      sql: built.sql,
      usedPrimaryKey: built.usedPrimaryKey,
      isProduction: Boolean(profile?.environment?.includes('生产')),
    };
  }

  async function openSubmitDialog() {
    const batch = await buildCurrentBatchUpdate();
    if (!batch) return;
    setUpdateError('');
    setPendingBatch(batch);
  }

  async function copyPendingUpdateSQL() {
    const batch = await buildCurrentBatchUpdate();
    if (!batch) return;
    try {
      await navigator.clipboard.writeText(batch.sql);
      setSqlCopied(true);
      window.setTimeout(() => setSqlCopied(false), 1800);
    } catch {
      setUpdateError('');
      setPendingBatch(batch);
    }
  }

  async function confirmBatchUpdate(sql: string) {
    if (!tab || !connection || !pendingBatch || !tab.result) return;
    setUpdateBusy(true);
    setUpdateError('');
    try {
      const res = await runtime.query({
        sessionId: connection.session.id,
        requestId: crypto.randomUUID(),
        database: tab.schema,
        sql,
        mode: pendingBatch.rowCount > 1 ? 'script' : 'statement',
        limit: 1,
        timeoutSeconds: timeout,
      });
      if (res.error) {
        setUpdateError(res.error);
        return;
      }
      const nextSets = tab.result.sets.map((set, idx) =>
        idx === tab.resultIndex
          ? applyPendingEditsToResult(set, pendingEdits)
          : set,
      );
      update(tab.id, {
        result: { ...tab.result, sets: nextSets },
      });
      setPendingEdits({});
      setPendingBatch(null);
    } catch (e) {
      setUpdateError(errorText(e));
    } finally {
      setUpdateBusy(false);
    }
  }

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
        {error && (
          <div className="oc-db-banner">
            <DatabaseError message={error} />
            <IconButton label="关闭错误提示" onClick={() => setError('')}><TbX size={18} /></IconButton>
          </div>
        )}
        <nav ref={tabStrip} className="oc-db-tabs" aria-label="数据库查询标签">
          {failureProfiles.map((p) => (
            <div
              key={`failure-${p.id}`}
              className={!tab && profileId === p.id ? 'is-selected' : ''}
              onContextMenu={(e) => openFailureTabMenu(e, p)}
              onKeyDown={(e) => {
                if ((e.shiftKey && e.key === 'F10') || e.key === 'ContextMenu')
                  openFailureTabMenu(e, p);
              }}
            >
              <button
                aria-label={`连接失败 · ${p.name}`}
                title={`连接失败 · ${p.name}`}
                onClick={() => {
                  model.setSelectedProfile(p.id);
                  setActiveTab('');
                }}
              >
                <TbAlertCircle size={14} />
                连接失败<small>{p.name}</small>
              </button>
              <button
                aria-label={`关闭连接失败 · ${p.name}`}
                onClick={() => closeFailureTab(p.id)}
              >
                <TbX size={12} />
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
          <div className={`oc-db-empty${busyProfile ? ' is-connecting' : ''}`}>
            <div className="oc-db-empty-icon">
              <TbDatabase size={36} />
              {busyProfile && (
                <span className="oc-db-empty-ring" aria-hidden />
              )}
            </div>
            <h3>
              {busyProfile
                ? `正在连接 ${profile?.name ?? ''}…`
                : profile
                  ? profile.name
                  : '打开数据库连接'}
            </h3>
            <p>
              {profile
                ? `${profile.host}:${profile.port} · ${profile.user}${profile.gatewayId ? ' · 通过 SSH' : ''}`
                : '从左侧选择连接，或添加数据库连接。'}
            </p>
            {!busyProfile &&
              (profile ? (
                <button
                  className="oc-db-primary"
                  disabled={profileRunning || !!metadataBusy}
                  onClick={() => void connect(profile)}
                >
                  连接数据库
                </button>
              ) : (
                <button
                  className="oc-db-primary"
                  onClick={() => setEditing(emptyProfile())}
                >
                  添加连接
                </button>
              ))}
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
                  <div className="oc-db-select-wrap oc-db-schema-select">
                    <TbDatabase size={13} className="oc-db-select-lead" aria-hidden />
                    <select
                      aria-label="当前数据库"
                      value={tab.schema}
                      disabled={profileRunning || !!metadataBusy || !!tab.table}
                      onChange={(e) => {
                        const nextSchema = e.target.value;
                        if (nextSchema && connection) {
                          attemptedCompletionSchemas.current.delete(
                            `${connection.session.id}:${nextSchema}`,
                          );
                        }
                        update(tab.id, { schema: nextSchema });
                      }}
                    >
                      <option value="">选择数据库</option>
                      {(connection?.schemas??managed?.workspace.schemas??[]).map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                    <TbChevronDown size={13} className="oc-db-select-caret" aria-hidden />
                  </div>
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
                        <div className="oc-db-select-wrap">
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
                          <TbChevronDown size={12} className="oc-db-select-caret" aria-hidden />
                        </div>
                      </label>
                      <label>
                        超时
                        <div className="oc-db-select-wrap">
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
                          <TbChevronDown size={12} className="oc-db-select-caret" aria-hidden />
                        </div>
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
                  {managed&&tab.id===`ai-query-${managed.workspace.id}`&&managed.workspace.blocks.length>0&&<DatabaseExecutionHistory blocks={managed.workspace.blocks} revealId={managed.workspace.revealBlock} revealNonce={managed.workspace.revealNonce} onSelect={block=>update(tab.id,{result:block.result,error:block.error,resultIndex:0})}/>}
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
                        <div className="oc-db-result-tabs">
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
                            {result.columns.length > 0 && (
                              <div className="oc-db-result-filter">
                                <TbSearch size={13} aria-hidden />
                                <input
                                  type="search"
                                  aria-label="筛选结果行"
                                  placeholder="筛选结果…"
                                  value={resultFilter}
                                  onChange={(e) =>
                                    setResultFilter(e.target.value)
                                  }
                                />
                                {resultFilter && (
                                  <button
                                    type="button"
                                    aria-label="清除结果筛选"
                                    title="清除筛选"
                                    onClick={() => setResultFilter('')}
                                  >
                                    <TbX size={12} />
                                  </button>
                                )}
                              </div>
                            )}
                            <small>
                              {result.columns.length
                                ? resultFilter.trim() &&
                                  filteredCount !== null
                                  ? `匹配 ${filteredCount} / ${result.rows.length} 行`
                                  : `${result.rows.length} 行`
                                : `影响 ${result.affectedRows} 行`}
                              {tab.result
                                ? ` · ${tab.result.durationMs} ms`
                                : ''}
                              {result.truncated ? ' · 已达返回上限' : ''}
                            </small>
                            {canEditTableData && result.columns.length > 0 && (
                              <div className="oc-db-dirty-actions">
                                {dirtyCellCount > 0 && (
                                  <span
                                    className="oc-db-dirty-badge"
                                    title={`有 ${dirtyCellCount} 个未提交的单元格修改`}
                                  >
                                    {dirtyCellCount}
                                  </span>
                                )}
                                <button
                                  type="button"
                                  className={
                                    dirtyCellCount > 0 ? 'is-dirty-submit' : ''
                                  }
                                  title={
                                    dirtyCellCount > 0
                                      ? `提交 ${dirtyCellCount} 处修改到数据库`
                                      : '暂无未提交修改'
                                  }
                                  aria-label="提交修改"
                                  disabled={dirtyCellCount === 0}
                                  onClick={() => void openSubmitDialog()}
                                >
                                  <TbCheck size={15} />
                                </button>
                                <button
                                  type="button"
                                  className={sqlCopied ? 'is-copied' : ''}
                                  title={
                                    sqlCopied
                                      ? '已复制 UPDATE SQL'
                                      : '复制待执行的 UPDATE SQL'
                                  }
                                  aria-label="复制更新 SQL"
                                  disabled={dirtyCellCount === 0}
                                  onClick={() => void copyPendingUpdateSQL()}
                                >
                                  {sqlCopied ? (
                                    <TbCheck size={15} />
                                  ) : (
                                    <TbCopy size={15} />
                                  )}
                                </button>
                                <button
                                  type="button"
                                  title="撤销全部未提交修改"
                                  aria-label="撤销全部修改"
                                  disabled={dirtyCellCount === 0}
                                  onClick={() => setPendingEdits({})}
                                >
                                  <TbArrowBackUp size={15} />
                                </button>
                              </div>
                            )}
                            <button
                              title="导出当前结果为 CSV"
                              aria-label="导出 CSV"
                              disabled={!result.columns.length}
                              onClick={exportResult}
                            >
                              <TbUpload size={15} />
                            </button>
                          </>
                        )}
                      </div>
                      {result?.columns.length ? (
                        <ResultGrid
                          key={resultViewKey}
                          result={result}
                          filterText={resultFilter}
                          onFilteredCountChange={(filtered) =>
                            setFilteredCount(filtered)
                          }
                          target={{
                            schema: editableTarget?.schema ?? tab.schema,
                            table: editableTarget?.table ?? tab.table,
                            dialect: sqlDialect,
                          }}
                          canEdit={canEditTableData}
                          pendingEdits={pendingEdits}
                          onStageCellEdit={stageCellEdit}
                          onRevertCellEdit={revertCellEdit}
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
            {
              label: '关闭其他',
              disabled:
                !tabs.some((t) => t.id !== menuTab.id && !t.pending) &&
                failureProfiles.length === 0,
              action: () => {
                model.closeTabs(
                  tabs.filter((t) => t.id !== menuTab.id).map((t) => t.id),
                );
                model.clearAllConnectionFailures();
              },
            },
            {
              label: '关闭右侧',
              disabled: !tabs
                .slice(tabs.findIndex((t) => t.id === menuTab.id) + 1)
                .some((t) => !t.pending),
              action: () =>
                model.closeTabs(
                  tabs
                    .slice(tabs.findIndex((t) => t.id === menuTab.id) + 1)
                    .map((t) => t.id),
                ),
            },
            {
              label: '关闭全部',
              disabled:
                !tabs.some((t) => !t.pending) && failureProfiles.length === 0,
              action: () => {
                model.closeTabs(tabs.map((t) => t.id));
                model.clearAllConnectionFailures();
              },
            },
          ]}
        />
      )}
      {tabMenu && menuFailureProfile && (
        <DatabaseMenu
          x={tabMenu.x}
          y={tabMenu.y}
          label="连接失败页签操作"
          onClose={dismissTabMenu}
          items={[
            {
              label: '重新连接',
              disabled: !!model.connecting[menuFailureProfile.id],
              action: () => void connect(menuFailureProfile),
            },
            {
              label: '编辑连接…',
              action: () => setEditing(menuFailureProfile),
            },
            {
              label: '关闭当前',
              separator: true,
              action: () => closeFailureTab(menuFailureProfile.id),
            },
            {
              label: '关闭其他',
              disabled:
                !tabs.some((t) => !t.pending) && failureProfiles.length <= 1,
              action: () => {
                model.closeTabs(tabs.map((t) => t.id));
                model.clearAllConnectionFailures(menuFailureProfile.id);
              },
            },
            {
              label: '关闭全部',
              action: () => {
                model.closeTabs(tabs.map((t) => t.id));
                model.clearAllConnectionFailures();
                if (!tab && profileId === menuFailureProfile.id) {
                  model.setSelectedProfile('');
                }
              },
            },
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
      {pendingBatch && (
        <CellUpdateConfirmDialog
          update={pendingBatch}
          busy={updateBusy}
          error={updateError}
          onConfirm={(sql) => void confirmBatchUpdate(sql)}
          onClose={() => {
            if (!updateBusy) setPendingBatch(null);
          }}
        />
      )}
    </section>
  );
}
