import { useCallback, useEffect, useRef, useState } from 'react';
import type { ManagedDatabase } from '../../workspace/database';
import type {
  DatabaseProfile,
  DatabaseRuntime,
  DatabaseSQLTemplate,
  DatabaseStructure,
  SQLExecution,
} from './types';
import {
  errorText,
  objectKey,
  systemDatabase,
  type ConnectionState,
  type QueryTab,
} from './workspaceModel';

export interface ConnectionFailure {
  message: string;
  stage: string;
  occurredAt: string;
}

export function useDatabaseWorkspace(runtime: DatabaseRuntime,managed?: ManagedDatabase) {
  const [profiles, setProfiles] = useState<DatabaseProfile[]>([]);
  const [groups, setGroups] = useState<string[]>([]);
  const [connections, setConnections] = useState<
    Record<string, ConnectionState>
  >({});
  const [tabs, setTabs] = useState<QueryTab[]>([]);
  const [activeTab, changeActiveTab] = useState('');
  const [selectedProfile, changeSelectedProfile] = useState('');
  const [connectionFailures, setConnectionFailures] = useState<
    Record<string, ConnectionFailure>
  >({});
  const navigation = useRef(0);
  function setActiveTab(id: string) {
    navigation.current++;
    changeActiveTab(id);
  }
  function setSelectedProfile(id: string) {
    navigation.current++;
    changeSelectedProfile(id);
  }
  function clearConnectionFailure(id: string) {
    setConnectionFailures((all) => {
      const next = { ...all };
      delete next[id];
      return next;
    });
  }
  const [connecting, setConnecting] = useState<Record<string, boolean>>({});
  const [metadataBusy, setMetadataBusy] = useState<Record<string, boolean>>({});
  const [error, setError] = useState('');
  const current = useRef({ connections, tabs });
  const generation = useRef(0);
  const openings = useRef(
    new Map<string, Promise<ConnectionState | undefined>>(),
  );
  const metadata = useRef(new Map<string, Promise<unknown>>());
  const requests = useRef(new Set<string>());
  const managedSeen = useRef('');
  const managedSQL = useRef('');
  const managedBlock = useRef('');
  const managedOwner = useRef('');
  const managedHumanQuery = useRef(false);
  const setConnection = (id: string, value?: ConnectionState) => {
    const next = { ...current.current.connections };
    if (value) next[id] = value;
    else delete next[id];
    current.current.connections = next;
    setConnections(next);
  };
  const changeTabs = (change: (all: QueryTab[]) => QueryTab[]) => {
    const next = change(current.current.tabs);
    current.current.tabs = next;
    setTabs(next);
  };
  const update = (id: string, patch: Partial<QueryTab>) =>
    changeTabs((all) => all.map((t) => (t.id === id ? { ...t, ...patch } : t)));
  useEffect(() => {
    if (!managed) return;
    const w = managed.workspace;
    if (w.ownerTurn && managedOwner.current !== w.ownerTurn) managedHumanQuery.current = false;
    managedOwner.current = w.ownerTurn;
    const primary = `ai-query-${w.id}`;
    const fresh = managedSeen.current !== w.id;
    const missing = !current.current.tabs.some(t => t.id === primary);
    if (fresh || (w.pending && missing)) {
      managedSeen.current = w.id;
      changeTabs(all => [...all, {
        id: primary, profileId: w.profile.id,
        schema: w.session.database || w.schemas?.find(s => !systemDatabase(s)) || '',
        title: 'AI 查询记录', sql: w.sql, offset: 0, view: 'query', resultIndex: 0,
      }]);
      setSelectedProfile(w.profile.id);
      setActiveTab(primary);
    }
    if (w.connected && !current.current.connections[w.profile.id]) {
      setConnection(w.profile.id, {
        session: w.session,
        schemas: w.schemas ?? (w.session.database ? [w.session.database] : []),
        tables: {}, structures: {},
      });
    }
    if (!w.connected && current.current.connections[w.profile.id]?.session.id === w.session.id) {
      setConnection(w.profile.id);
    }
    if ((w.ownerTurn || !managed.editing) && (fresh || managedSQL.current !== w.sql)) {
      update(primary, { sql: w.sql });
      managedSQL.current = w.sql;
    }
    const last = w.blocks.at(-1);
    const resultKey = last ? `${last.id}:${last.status}` : '';
    if (last && managedBlock.current !== resultKey) {
      managedBlock.current = resultKey;
      if (!managedHumanQuery.current) update(primary, {
        result: last.result, resultIndex: 0, error: last.error || last.result?.error,
        pending: last.status === 'running' ? last.id : undefined,
      });
    }
    if (w.pending) setActiveTab(primary);
  }, [managed?.workspace.revision, managed?.editing]);
  const running = (pid: string) =>
    current.current.tabs.some((t) => t.profileId === pid && !!t.pending);
  const loadProfiles = useCallback(async () => {
    try {
      await refreshCatalog();
    } catch (e) {
      setError(errorText(e));
    }
  }, [runtime]);
  async function refreshCatalog() {
    const [profiles, groups] = await Promise.all([
      runtime.profiles(),
      runtime.groups(),
    ]);
    setProfiles(profiles);
    setGroups(groups);
  }
  useEffect(() => {
    generation.current++;
    void loadProfiles();
    return () => {
      generation.current++;
      for (const id of requests.current)
        void runtime.cancel(id).catch(() => {});
      for (const c of Object.values(current.current.connections))
        if(c.session.id!==managed?.workspace.session.id)void runtime.disconnect(c.session.id).catch(() => {});
    };
  }, [runtime, loadProfiles]);

  // Connection establishment is deduplicated per profile, allowing other profiles
  // to connect independently. Late completions cannot create orphan sessions.
  async function connect(
    p: DatabaseProfile,
  ): Promise<ConnectionState | undefined> {
    const existing = current.current.connections[p.id];
    if (existing) return existing;
    const opening = openings.current.get(p.id);
    if (opening) return opening;
    setConnecting((all) => ({ ...all, [p.id]: true }));
    const gen = generation.current;
    const task = (async () => {
      let session;
      let stage = '建立连接';
      try {
        session = await runtime.connect(p.id);
        stage = '读取数据库列表';
        const schemas = await runtime.schemas(session.id);
        const database = schemas.includes(p.database)
          ? p.database
          : (schemas.find((s) => !systemDatabase(s)) ?? '');
        stage = '读取数据表列表';
        const tables = database
          ? await runtime.tables(session.id, database)
          : [];
        if (gen !== generation.current) {
          await runtime.disconnect(session.id);
          return;
        }
        const next: ConnectionState = {
          session,
          schemas,
          tables: database ? { [database]: tables } : {},
          structures: {},
        };
        setConnection(p.id, next);
        clearConnectionFailure(p.id);
        return next;
      } catch (e) {
        if (session) await runtime.disconnect(session.id).catch(() => {});
        if (gen === generation.current)
          setConnectionFailures((all) => ({
            ...all,
            [p.id]: {
              message: errorText(e),
              stage,
              occurredAt: new Date().toISOString(),
            },
          }));
      } finally {
        openings.current.delete(p.id);
        if (gen === generation.current)
          setConnecting((all) => ({ ...all, [p.id]: false }));
      }
    })();
    openings.current.set(p.id, task);
    return task;
  }
  async function disconnect(pid: string): Promise<boolean> {
    const c = current.current.connections[pid];
    if (!c) return true;
    try {
      await runtime.disconnect(c.session.id);
      if (current.current.connections[pid]?.session.id === c.session.id) {
        setConnection(pid);
        changeTabs(all => all.map(t => t.profileId === pid ? {...t,pending:undefined} : t));
      }
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    }
  }
  async function reconnect(p: DatabaseProfile) {
    if (
      running(p.id) ||
      openings.current.has(p.id) ||
      metadata.current.has(p.id)
    )
      return;
    if (!await disconnect(p.id)) return;
    await connect(p);
  }
  async function openConnection(p: DatabaseProfile) {
    setSelectedProfile(p.id);
    setActiveTab('');
    const intent = navigation.current;
    const connected = await connect(p);
    // A late connection must not replace a query the user has switched back to.
    if (!connected || intent !== navigation.current) return;
    const first = current.current.tabs.find((t) => t.profileId === p.id);
    if (first) setActiveTab(first.id);
    else
      newQuery(
        p.id,
        connected.session.database ||
          connected.schemas.find((s) => !systemDatabase(s)) ||
          '',
      );
  }
  function newQuery(pid: string, schema = '', sql = '') {
    const next: QueryTab = {
      id: crypto.randomUUID(),
      profileId: pid,
      schema,
      sql,
      title: `查询 ${Math.max(0, ...current.current.tabs.filter((t) => t.profileId === pid && !t.table).map((t) => Number(t.title.match(/\d+$/)?.[0]) || 0)) + 1}`,
      offset: 0,
      view: 'query',
      resultIndex: 0,
    };
    changeTabs((all) => [...all, next]);
    setActiveTab(next.id);
    return next;
  }
  // Metadata operations serialize per physical session and never overlap queries.
  async function withMetadata<T>(
    pid: string,
    operation: (c: ConnectionState) => Promise<T>,
  ): Promise<T> {
    const c = current.current.connections[pid];
    if (!c) throw new Error('请先连接数据库');
    if (running(pid)) throw new Error('该连接正在执行查询，请稍后重试');
    const previous = metadata.current.get(pid) ?? Promise.resolve();
    const gen = generation.current;
    const task = previous
      .catch(() => {})
      .then(async () => {
        if (
          gen !== generation.current ||
          current.current.connections[pid]?.session.id !== c.session.id
        )
          throw new Error('连接已变更，请重试');
        return operation(current.current.connections[pid]);
      });
    metadata.current.set(pid, task);
    setMetadataBusy((all) => ({ ...all, [pid]: true }));
    try {
      return await task;
    } finally {
      if (metadata.current.get(pid) === task) {
        metadata.current.delete(pid);
        if (gen === generation.current)
          setMetadataBusy((all) => ({ ...all, [pid]: false }));
      }
    }
  }
  async function loadTables(pid: string, database: string, force = false) {
    if (!force && current.current.connections[pid]?.tables[database]) return;
    try {
      await withMetadata(pid, async (c) => {
        const tables = await runtime.tables(c.session.id, database);
        if (current.current.connections[pid]?.session.id !== c.session.id)
          return;
        const structures = { ...c.structures };
        if (force)
          for (const key of Object.keys(structures))
            if (JSON.parse(key)[0] === database) delete structures[key];
        setConnection(pid, {
          ...c,
          tables: { ...c.tables, [database]: tables },
          structures,
        });
      });
    } catch (e) {
      setError(errorText(e));
    }
  }
  async function refreshConnection(pid: string) {
    try {
      await withMetadata(pid, async (c) => {
        const schemas = await runtime.schemas(c.session.id);
        const tables: ConnectionState['tables'] = {};
        for (const database of Object.keys(c.tables))
          if (schemas.includes(database))
            tables[database] = await runtime.tables(c.session.id, database);
        if (current.current.connections[pid]?.session.id === c.session.id)
          setConnection(pid, { ...c, schemas, tables, structures: {} });
      });
    } catch (e) {
      setError(errorText(e));
    }
  }
  async function loadStructure(
    pid: string,
    database: string,
    table: string,
    force = false,
  ): Promise<DatabaseStructure | undefined> {
    const key = objectKey(database, table);
    const cached = current.current.connections[pid]?.structures[key];
    if (!force && cached) return cached;
    return withMetadata(pid, async (c) => {
      const structure = await runtime.structure({
        sessionId: c.session.id,
        database,
        table,
        requestId: crypto.randomUUID(),
        offset: 0,
        limit: 100,
      });
      if (current.current.connections[pid]?.session.id !== c.session.id) return;
      setConnection(pid, {
        ...c,
        structures: { ...c.structures, [key]: structure },
      });
      return structure;
    });
  }
  async function execute(
    target: QueryTab,
    execution?: SQLExecution,
    offset = target.offset,
  ) {
    const c = current.current.connections[target.profileId];
    if (!c) {
      setError('请先连接该标签对应的数据库');
      return;
    }
    if (running(target.profileId) || metadata.current.has(target.profileId))
      return;
    const sql = execution?.sql ?? target.sql;
    if (!target.table && !sql.trim()) return;
    const requestId = crypto.randomUUID();
    if (managed && target.id === `ai-query-${managed.workspace.id}`) managedHumanQuery.current = true;
    const gen = generation.current;
    requests.current.add(requestId);
    update(target.id, { pending: requestId, error: '', offset });
    try {
      const result = target.table
        ? await runtime.browse({
            sessionId: c.session.id,
            requestId,
            database: target.schema,
            table: target.table,
            offset,
            limit: 100,
          })
        : await runtime.query({
            sessionId: c.session.id,
            requestId,
            database: target.schema,
            sql,
            mode: execution?.mode ?? 'statement',
            cursorOffset: execution?.cursorOffset ?? 0,
            limit: options.current.limit,
            timeoutSeconds: options.current.timeout,
          });
      if (
        gen !== generation.current ||
        current.current.connections[target.profileId]?.session.id !==
          c.session.id
      )
        return;
      const now = current.current.tabs.find((t) => t.id === target.id);
      if (now?.pending === requestId)
        update(target.id, {
          pending: undefined,
          result,
          resultIndex: 0,
          error: result.error,
        });
      if (result.disconnected) setConnection(target.profileId);
    } catch (e) {
      if (gen === generation.current &&
          current.current.connections[target.profileId]?.session.id === c.session.id &&
          current.current.tabs.find(t => t.id === target.id)?.pending === requestId)
        update(target.id, { pending: undefined, error: errorText(e) });
    } finally {
      requests.current.delete(requestId);
    }
  }
  const options = useRef({ limit: 1000, timeout: 30 });
  async function tableView(
    target: QueryTab,
    view: QueryTab['view'],
    force = false,
  ) {
    update(target.id, { view, error: '' });
    if (view === 'data') {
      if (force || !target.result)
        await execute(target, undefined, force ? 0 : target.offset);
      return;
    }
    try {
      const structure = await loadStructure(
        target.profileId,
        target.schema,
        target.table!,
        force,
      );
      if (structure) update(target.id, { structure });
    } catch (e) {
      update(target.id, { error: errorText(e) });
    }
  }
  async function openTable(
    pid: string,
    database: string,
    table: string,
    view: QueryTab['view'] = 'data',
  ) {
    let next = current.current.tabs.find(
      (t) => t.profileId === pid && t.schema === database && t.table === table,
    );
    if (!next) {
      next = {
        id: crypto.randomUUID(),
        profileId: pid,
        schema: database,
        table,
        title: table,
        sql: '',
        offset: 0,
        view,
        resultIndex: 0,
      };
      changeTabs((all) => [...all, next!]);
    }
    setActiveTab(next.id);
    await tableView(next, view);
  }
  async function generateSQL(
    pid: string,
    database: string,
    table: string,
    kind: DatabaseSQLTemplate['kind'],
    column = '',
  ) {
    return withMetadata(pid, (c) =>
      runtime.generateSQL({
        sessionId: c.session.id,
        database,
        table,
        kind,
        column,
      }),
    );
  }
  async function saved(p: DatabaseProfile, shouldConnect: boolean) {
    // An edited endpoint/credential must not be displayed as the old live session.
    // Moving a group uses moveProfile instead and preserves the connection.
    if (!await disconnect(p.id)) {
      setError('连接配置已保存，当前会话继续使用原配置；重新连接后新配置才会生效。');
      return;
    }
    setProfiles((all) => [...all.filter((v) => v.id !== p.id), p]);
    setGroups((all) =>
      p.group && !all.includes(p.group) ? [...all, p.group] : all,
    );
    setSelectedProfile(p.id);
    clearConnectionFailure(p.id);
    if (shouldConnect) await openConnection(p);
  }
  async function moveProfile(p: DatabaseProfile, group: string) {
    const next = await runtime.save({ ...p, group });
    setProfiles((all) => all.map((v) => (v.id === p.id ? next : v)));
    setGroups((all) => (group && !all.includes(group) ? [...all, group] : all));
  }
  async function createGroup(name: string) {
    await runtime.createGroup(name);
    await refreshCatalog();
  }
  async function renameGroup(oldName: string, newName: string) {
    await runtime.renameGroup(oldName, newName);
    await refreshCatalog();
  }
  async function deleteGroup(name: string) {
    await runtime.deleteGroup(name);
    await refreshCatalog();
  }
  async function removeProfile(p: DatabaseProfile) {
    if (
      !window.confirm(
        `删除连接「${p.name}」？保存的凭据也会移除，数据库内容不受影响。`,
      )
    )
      return;
    try {
      await runtime.remove(p.id, p.revision);
      await disconnect(p.id);
      clearConnectionFailure(p.id);
      changeTabs((all) => all.filter((t) => t.profileId !== p.id));
      if (current.current.tabs.every((t) => t.id !== activeTab))
        setActiveTab(current.current.tabs.at(-1)?.id ?? '');
      setProfiles((all) => all.filter((v) => v.id !== p.id));
      if (selectedProfile === p.id) setSelectedProfile('');
    } catch (e) {
      setError(errorText(e));
    }
  }
  function closeTabs(ids: string[]) {
    const selected = new Set(ids);
    const all = current.current.tabs;
    const remaining = all.filter((v) => !selected.has(v.id) || !!v.pending);
    changeTabs(() => remaining);
    if (activeTab && !remaining.some((t) => t.id === activeTab))
      setActiveTab(
        remaining[
          Math.min(
            all.findIndex((t) => t.id === activeTab),
            remaining.length - 1,
          )
        ]?.id ?? '',
      );
  }
  function closeTab(t: QueryTab) {
    closeTabs([t.id]);
  }
  function duplicateQuery(t: QueryTab) {
    const source = current.current.tabs.find((v) => v.id === t.id);
    if (!source || source.table) return;
    const copy = newQuery(source.profileId, source.schema, source.sql);
    update(copy.id, { title: `${source.title.slice(0, 50)} 副本` });
  }
  return {
    profiles,
    groups,
    createGroup,
    renameGroup,
    deleteGroup,
    connections,
    tabs,
    activeTab,
    setActiveTab,
    selectedProfile,
    setSelectedProfile,
    connecting,
    connectionFailures,
    metadataBusy,
    error,
    setError,
    current,
    options,
    running,
    update,
    loadProfiles,
    connect,
    openConnection,
    disconnect,
    reconnect,
    newQuery,
    loadTables,
    refreshConnection,
    loadStructure,
    execute,
    openTable,
    tableView,
    generateSQL,
    saved,
    moveProfile,
    removeProfile,
    closeTab,
    closeTabs,
    duplicateQuery,
  };
}

export type DatabaseWorkspaceModel = ReturnType<typeof useDatabaseWorkspace>;
