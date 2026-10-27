import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
  type KeyboardEvent,
} from 'react';
import {
  TbChevronDown,
  TbChevronRight,
  TbCode,
  TbDatabase,
  TbDots,
  TbFolder,
  TbPlus,
  TbPackage,
  TbRefresh,
  TbSearch,
  TbTable,
} from 'react-icons/tb';
import {
  DatabaseConnectionTree,
  connectionVisible,
  type DatabaseTreeController,
} from './DatabaseConnectionTree';
import { ConnectionDialog, emptyProfile } from './ConnectionDialog';
import { JDBCDriverDialog } from './JDBCDriverDialog';
import { DatabaseGroupDialog } from './DatabaseGroupDialog';
import {
  DatabaseFolderDialog,
  type FolderAction,
} from './DatabaseFolderDialog';
import { DatabaseMenu, type DatabaseMenuItem } from './DatabaseMenu';
import type {
  DatabaseProfile,
  DatabaseRuntime,
  DatabaseSQLTemplate,
} from './types';
import {
  errorText,
  nodeKey,
  objectKey,
  systemDatabase,
  type DatabaseNode,
  type QueryTab,
} from './workspaceModel';
import type { DatabaseWorkspaceModel } from './useDatabaseWorkspace';

export function DatabaseExplorer({
  model: m,
  runtime,
  onInsert,
  editing,
  setEditing,
}: {
  model: DatabaseWorkspaceModel;
  runtime: DatabaseRuntime;
  onInsert(text: string, profileId: string, database: string): void;
  editing: DatabaseProfile | null;
  setEditing(p: DatabaseProfile | null): void;
}) {
  const [managingDrivers, setManagingDrivers] = useState(false);
  const [selected, setSelected] = useState('');
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [collapsedGroups, setCollapsedGroups] = useState<
    Record<string, boolean>
  >({});
  const [filter, setFilter] = useState('');
  const [scope, setScope] = useState<{
    profileId: string;
    database: string;
  } | null>(null);
  const tree = useRef<HTMLDivElement>(null);
  const [locate, setLocate] = useState(0);
  const locateTarget = useRef('');
  useEffect(() => {
    if (!locate) return;
    const element = Array.from(
      tree.current?.querySelectorAll<HTMLElement>('[data-db-node]') ?? [],
    ).find((el) => el.dataset.dbNode === locateTarget.current);
    element?.scrollIntoView({ block: 'nearest' });
    element?.focus();
  }, [locate]);
  const [showSystem, setShowSystem] = useState(false);
  const [connectedOnly, setConnectedOnly] = useState(false);
  const [moving, setMoving] = useState<DatabaseProfile | null>(null);
  const [folderAction, setFolderAction] = useState<FolderAction | null>(null);
  const [menu, setMenu] = useState<{
    x: number;
    y: number;
    node?: DatabaseNode;
    group?: string;
    trigger: HTMLElement;
  } | null>(null);
  const groups = [
    ...new Set([...m.groups, ...m.profiles.map((p) => p.group ?? '')]),
  ].sort((a, b) => a.localeCompare(b));
  const term = filter.trim().toLowerCase();
  const matches = (...values: (string | undefined)[]) =>
    values.some((value) => value?.toLowerCase().includes(term));
  const busy = (pid: string) =>
    m.running(pid) || !!m.metadataBusy[pid] || !!m.connecting[pid];
  const safely = (action: () => Promise<unknown>) => {
    void action().catch((e) => m.setError(errorText(e)));
  };
  const select = (node: DatabaseNode) => {
    setSelected(nodeKey(node));
    m.setSelectedProfile(node.profileId);
  };
  const openMenu = (e: MouseEvent<HTMLElement>, node?: DatabaseNode) => {
    e.preventDefault();
    e.stopPropagation();
    if (node) select(node);
    const rect = e.currentTarget.getBoundingClientRect();
    setMenu({
      x: e.clientX || rect.left + 12,
      y: e.clientY || rect.bottom,
      node,
      trigger: e.currentTarget,
    });
  };
  const groupMenu = (
    trigger: HTMLElement,
    group: string,
    x?: number,
    y?: number,
  ) => {
    const rect = trigger.getBoundingClientRect();
    setMenu({ x: x || rect.left + 12, y: y || rect.bottom, group, trigger });
  };
  const keyboard = (
    e: KeyboardEvent<HTMLElement>,
    node: DatabaseNode,
    open?: () => void,
    collapse?: () => void,
  ) => {
    if ((e.shiftKey && e.key === 'F10') || e.key === 'ContextMenu') {
      e.preventDefault();
      select(node);
      const rect = e.currentTarget.getBoundingClientRect();
      setMenu({
        x: rect.left + 12,
        y: rect.bottom,
        node,
        trigger: e.currentTarget,
      });
    } else if ((e.key === 'Enter' || e.key === 'ArrowRight') && open) {
      e.preventDefault();
      open();
    } else if (e.key === 'ArrowLeft' && collapse) {
      e.preventDefault();
      collapse();
    }
  };
  const setOpen = (node: DatabaseNode, open: boolean) =>
    setExpanded((all) => ({ ...all, [nodeKey(node)]: open }));
  async function expandConnection(p: DatabaseProfile, activate: boolean) {
    const node: DatabaseNode = { kind: 'connection', profileId: p.id };
    setOpen(node, true);
    select(node);
    if (activate) await m.openConnection(p);
    else await m.connect(p);
  }
  async function expandDatabase(pid: string, database: string) {
    setOpen({ kind: 'database', profileId: pid, database }, true);
    await m.loadTables(pid, database);
  }
  async function expandTable(node: Extract<DatabaseNode, { kind: 'table' }>) {
    setOpen(node, true);
    await m.loadStructure(node.profileId, node.database, node.table);
  }
  function addQuery(pid: string, database: string) {
    m.newQuery(pid, database);
  }
  const active = m.tabs.find((t) => t.id === m.activeTab);
  async function textAction(
    node: Extract<DatabaseNode, { kind: 'table' | 'column' }>,
    kind: DatabaseSQLTemplate['kind'],
    destination: 'copy' | 'query' | 'insert',
  ) {
    const text = await m.generateSQL(
      node.profileId,
      node.database,
      node.table,
      kind,
      node.kind === 'column' ? node.column : '',
    );
    if (destination === 'copy') await navigator.clipboard.writeText(text);
    else if (destination === 'insert')
      onInsert(text, node.profileId, node.database);
    else addQueryWithSQL(node.profileId, node.database, text);
  }
  function addQueryWithSQL(pid: string, database: string, sql: string) {
    m.newQuery(pid, database, sql);
  }
  function menuItems(): DatabaseMenuItem[] {
    if (menu?.group !== undefined) {
      const group = menu.group;
      return [
        {
          label: '在此分组新建连接',
          action: () => setEditing({ ...emptyProfile(), group }),
        },
        {
          label: '新建分组…',
          action: () => setFolderAction({ kind: 'create' }),
        },
        {
          label: '重命名分组…',
          separator: true,
          action: () => setFolderAction({ kind: 'rename', name: group }),
        },
        {
          label: '删除分组…',
          action: () => setFolderAction({ kind: 'delete', name: group }),
        },
      ];
    }
    const n = menu?.node;
    if (!n)
      return [
        { label: '添加数据库连接', action: () => setEditing(emptyProfile()) },
        {
          label: '新建分组…',
          action: () => setFolderAction({ kind: 'create' }),
        },
        {
          label: showSystem ? '隐藏系统数据库' : '显示系统数据库',
          action: () => setShowSystem((v) => !v),
        },
        {
          label: connectedOnly ? '显示所有连接' : '仅显示已连接',
          action: () => setConnectedOnly((v) => !v),
        },
        {
          label: '折叠全部',
          action: () =>
            setExpanded(
              Object.fromEntries(
                m.profiles.map((p) => [
                  nodeKey({ kind: 'connection', profileId: p.id }),
                  false,
                ]),
              ),
            ),
        },
      ];
    const p = m.profiles.find((p) => p.id === n.profileId);
    if (!p) return [];
    const c = m.connections[p.id];
    const blocked = busy(p.id);
    if (n.kind === 'connection')
      return [
        {
          label: c ? '断开连接' : '连接',
          disabled: blocked,
          action: () =>
            safely(() => (c ? m.disconnect(p.id) : expandConnection(p, true))),
        },
        {
          label: '重新连接',
          disabled: !c || blocked,
          action: () => safely(() => m.reconnect(p)),
        },
        {
          label: '新建查询',
          disabled: !c,
          separator: true,
          action: () =>
            addQuery(
              p.id,
              c?.session.database ||
                c?.schemas.find((s) => !systemDatabase(s)) ||
                '',
            ),
        },
        {
          label: '刷新数据库对象',
          disabled: !c || blocked,
          action: () => safely(() => m.refreshConnection(p.id)),
        },
        {
          label: '编辑连接',
          disabled: blocked,
          separator: true,
          action: () => setEditing(p),
        },
        {
          label: '复制连接配置',
          action: () =>
            setEditing({
              ...p,
              id: '',
              revision: 0,
              name: `${p.name} · 副本`,
              passwordSet: false,
            }),
        },
        { label: '移动到分组…', disabled: blocked, action: () => setMoving(p) },
        {
          label: '删除连接配置…',
          danger: true,
          disabled: blocked,
          separator: true,
          action: () => safely(() => m.removeProfile(p)),
        },
      ];
    if (n.kind === 'database')
      return [
        { label: '新建查询', action: () => addQuery(p.id, n.database) },
        {
          label: '刷新数据库对象',
          disabled: blocked,
          action: () => safely(() => m.loadTables(p.id, n.database, true)),
        },
        {
          label: '复制数据库名称',
          action: () => safely(() => navigator.clipboard.writeText(n.database)),
        },
        {
          label: '筛选此数据库的表',
          action: () => {
            setFilter('');
            setScope({ profileId: p.id, database: n.database });
            setOpen(n, true);
            void m.loadTables(p.id, n.database);
            document
              .querySelector<HTMLInputElement>('.oc-db-search input')
              ?.focus();
          },
        },
      ];
    if (n.kind === 'column')
      return [
        {
          label: '复制字段名',
          disabled: blocked,
          action: () => safely(() => textAction(n, 'name', 'copy')),
        },
        {
          label: '复制完整字段名',
          disabled: blocked,
          action: () => safely(() => textAction(n, 'qualified-name', 'copy')),
        },
        {
          label: '插入当前查询',
          disabled: blocked,
          action: () => safely(() => textAction(n, 'name', 'insert')),
        },
      ];
    const open = (view: QueryTab['view']) =>
      safely(() => m.openTable(p.id, n.database, n.table, view));
    return [
      ...(['data', 'columns', 'indexes', 'ddl'] as const).map((view, i) => ({
        label: ['打开数据', '查看字段', '查看索引', '查看 DDL'][i],
        disabled: blocked,
        action: () => open(view),
      })),
      {
        label: '复制名称',
        disabled: blocked,
        separator: true,
        action: () => safely(() => textAction(n, 'name', 'copy')),
      },
      {
        label: '复制完整名称',
        disabled: blocked,
        action: () => safely(() => textAction(n, 'qualified-name', 'copy')),
      },
      {
        label: '插入当前查询',
        disabled: blocked,
        action: () => safely(() => textAction(n, 'qualified-name', 'insert')),
      },
      ...(['select', 'insert', 'update', 'delete'] as const).map((kind) => ({
        label: `生成 ${kind.toUpperCase()}`,
        disabled: blocked || (n.tableType === 'VIEW' && kind !== 'select'),
        separator: kind === 'select',
        action: () => safely(() => textAction(n, kind, 'query')),
      })),
      {
        label: '刷新表结构',
        disabled: blocked,
        separator: true,
        action: () =>
          safely(async () => {
            const structure = await m.loadStructure(
              p.id,
              n.database,
              n.table,
              true,
            );
            for (const tab of m.current.current.tabs)
              if (
                tab.profileId === p.id &&
                tab.schema === n.database &&
                tab.table === n.table
              )
                m.update(tab.id, { structure });
          }),
      },
    ];
  }
  const controller: DatabaseTreeController = {
    scope,
    term,
    matches,
    showSystem,
    connectedOnly,
    expanded,
    selected,
    select,
    setOpen,
    openMenu,
    keyboard,
    expandConnection,
    expandDatabase,
    expandTable,
    setExpanded,
    onInsertColumn: (node) => textAction(node, 'name', 'insert'),
  };
  const visibleGroups = groups
    .map((group) => ({
      group,
      rendered: m.profiles
        .filter((p) => (p.group ?? '') === group)
        .filter((p) => connectionVisible(p, m, controller))
        .map((p) => (
          <DatabaseConnectionTree
            key={p.id}
            profile={p}
            model={m}
            controller={controller}
          />
        )),
    }))
    .filter(
      (g) =>
        g.rendered.length > 0 ||
        (!!g.group && !scope && !connectedOnly && (!term || matches(g.group))),
    );
  return (
    <aside className="oc-db-explorer">
      <header>
        <strong>
          <TbDatabase size={16} />
          数据库
        </strong>
        <div className="oc-db-explorer-actions">
          <button
            title="添加数据库连接"
            aria-label="添加数据库连接"
            onClick={() => setEditing(emptyProfile())}
          >
            <TbPlus size={17} />
          </button>
          <button
            type="button"
            title="驱动管理 · 查看、导入和移除 JDBC 驱动"
            aria-label="驱动管理"
            onClick={() => setManagingDrivers(true)}
          >
            <TbPackage size={17} />
          </button>
          <button
            aria-label="刷新连接列表"
            title="刷新连接列表"
            onClick={() => void m.loadProfiles()}
          >
            <TbRefresh size={15} />
          </button>
          <button
            title="对象树选项"
            aria-label="对象树选项"
            onClick={(e) => openMenu(e)}
          >
            <TbDots size={16} />
          </button>
        </div>
      </header>
      <div className="oc-db-search">
        <TbSearch size={14} />
        <input
          aria-label="搜索数据库或表"
          title="筛选连接及已加载的数据库对象"
          placeholder="筛选连接、库、表…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>
      {scope && (
        <small className="oc-db-filter-note">
          当前范围：{scope.database}
          <button onClick={() => setScope(null)}>清除范围</button>
        </small>
      )}
      {term && (
        <small className="oc-db-filter-note">
          匹配连接与已加载对象{' '}
          <button onClick={() => setFilter('')}>清除</button>
        </small>
      )}
      <div
        ref={tree}
        className="oc-db-tree"
        aria-label="数据库连接与对象"
        onContextMenu={(e) => {
          if (e.target === e.currentTarget) openMenu(e);
        }}
      >
        {visibleGroups.map(({ group, rendered }) => {
          return group ? (
            <section key={group} className="oc-db-group">
              <div
                className="oc-db-group-row"
                onContextMenu={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  groupMenu(e.currentTarget, group, e.clientX, e.clientY);
                }}
              >
                <button
                  className="oc-db-group-title"
                  aria-label={`${group}，${rendered.length} 个连接`}
                  onKeyDown={(e) => {
                    if (
                      (e.shiftKey && e.key === 'F10') ||
                      e.key === 'ContextMenu'
                    ) {
                      e.preventDefault();
                      e.stopPropagation();
                      groupMenu(e.currentTarget, group);
                    }
                  }}
                  aria-expanded={!collapsedGroups[group] || !!term}
                  onClick={() =>
                    setCollapsedGroups((all) => ({
                      ...all,
                      [group]: !all[group],
                    }))
                  }
                >
                  {!collapsedGroups[group] || term ? (
                    <TbChevronDown size={12} />
                  ) : (
                    <TbChevronRight size={12} />
                  )}
                  <TbFolder size={14} />
                  <span>{group}</span>
                  <small>{rendered.length}</small>
                </button>
                <button
                  className="oc-db-group-more"
                  aria-label={`管理分组 ${group}`}
                  title="管理分组"
                  onClick={(e) => groupMenu(e.currentTarget, group)}
                >
                  <TbDots size={14} />
                </button>
              </div>
              {(!collapsedGroups[group] || term) && (
                <div className="oc-db-tree-branch oc-db-group-children">
                  {rendered}
                  {rendered.length === 0 && (
                    <small className="oc-db-muted">空分组</small>
                  )}
                </div>
              )}
            </section>
          ) : (
            <div key="ungrouped">{rendered}</div>
          );
        })}
        {m.profiles.length === 0 && m.groups.length === 0 && (
          <div className="oc-db-explorer-empty">
            还没有数据库连接
            <button onClick={() => setEditing(emptyProfile())}>
              添加 MySQL 连接
            </button>
          </div>
        )}
        {m.profiles.length > 0 && visibleGroups.length === 0 && (
          <p className="oc-db-filter-note">没有匹配的连接或已加载对象</p>
        )}
      </div>
      <footer>
        <span>
          {m.profiles.length} 个连接 · {Object.keys(m.connections).length}{' '}
          个已连接
        </span>
        <button
          title="定位当前编辑对象"
          disabled={!active}
          onClick={() => {
            if (!active) return;
            const p = m.profiles.find((p) => p.id === active.profileId);
            if (!p) return;
            setCollapsedGroups((all) => ({ ...all, [p.group ?? '']: false }));
            const node: DatabaseNode = active.table
              ? {
                  kind: 'table',
                  profileId: p.id,
                  database: active.schema,
                  table: active.table,
                  tableType:
                    m.connections[p.id]?.tables[active.schema]?.find(
                      (t) => t.name === active.table,
                    )?.type ?? 'BASE TABLE',
                }
              : { kind: 'database', profileId: p.id, database: active.schema };
            setExpanded((all) => ({
              ...all,
              [nodeKey({ kind: 'connection', profileId: p.id })]: true,
              [nodeKey({
                kind: 'database',
                profileId: p.id,
                database: active.schema,
              })]: true,
            }));
            setFilter('');
            setScope(null);
            select(node);
            locateTarget.current = nodeKey(node);
            setLocate((v) => v + 1);
          }}
        >
          定位
        </button>
      </footer>
      {menu && (
        <DatabaseMenu
          x={menu.x}
          y={menu.y}
          items={menuItems()}
          onClose={() => {
            const trigger = menu.trigger;
            setMenu(null);
            trigger.focus();
          }}
        />
      )}
      {moving && (
        <DatabaseGroupDialog
          profile={moving}
          groups={groups.filter(Boolean)}
          onSave={(group) => m.moveProfile(moving, group)}
          onClose={() => setMoving(null)}
        />
      )}
      {folderAction && (
        <DatabaseFolderDialog
          action={folderAction}
          count={
            folderAction.kind === 'create'
              ? 0
              : m.profiles.filter((p) => p.group === folderAction.name).length
          }
          onSave={async (name) => {
            if (folderAction.kind === 'create') await m.createGroup(name);
            else if (folderAction.kind === 'rename')
              await m.renameGroup(folderAction.name, name);
            else await m.deleteGroup(folderAction.name);
            setCollapsedGroups((all) => ({ ...all, [name]: false }));
          }}
          onClose={() => setFolderAction(null)}
        />
      )}
      {editing && (
        <ConnectionDialog
          key={editing.id || editing.name}
          profile={editing}
          runtime={runtime}
          onClose={() => setEditing(null)}
          onSaved={(p, connect) => {
            setEditing(null);
            void m.saved(p, connect);
          }}
        />
      )}
      {managingDrivers && (
        <JDBCDriverDialog
          runtime={runtime}
          onClose={() => setManagingDrivers(false)}
        />
      )}
    </aside>
  );
}
