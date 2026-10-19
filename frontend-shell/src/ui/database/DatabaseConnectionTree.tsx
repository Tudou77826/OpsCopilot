import type {
  Dispatch,
  SetStateAction,
  MouseEvent,
  KeyboardEvent,
} from 'react';
import {
  TbChevronDown,
  TbChevronRight,
  TbCode,
  TbDatabase,
  TbServer,
  TbTable,
} from 'react-icons/tb';
import type { DatabaseProfile } from './types';
import type { DatabaseWorkspaceModel } from './useDatabaseWorkspace';
import {
  errorText,
  nodeKey,
  objectKey,
  systemDatabase,
  type DatabaseNode,
} from './workspaceModel';

export interface DatabaseTreeController {
  scope: { profileId: string; database: string } | null;
  term: string;
  showSystem: boolean;
  connectedOnly: boolean;
  expanded: Record<string, boolean>;
  selected: string;
  setExpanded: Dispatch<SetStateAction<Record<string, boolean>>>;
  matches(...values: (string | undefined)[]): boolean;
  select(node: DatabaseNode): void;
  setOpen(node: DatabaseNode, open: boolean): void;
  openMenu(e: MouseEvent<HTMLElement>, node?: DatabaseNode): void;
  keyboard(
    e: KeyboardEvent<HTMLElement>,
    node: DatabaseNode,
    open?: () => void,
    collapse?: () => void,
  ): void;
  expandConnection(p: DatabaseProfile, activate: boolean): Promise<void>;
  expandDatabase(pid: string, database: string): Promise<void>;
  expandTable(node: Extract<DatabaseNode, { kind: 'table' }>): Promise<void>;
  onInsertColumn(
    node: Extract<DatabaseNode, { kind: 'column' }>,
  ): Promise<void>;
}
export function connectionVisible(
  p: DatabaseProfile,
  m: DatabaseWorkspaceModel,
  ui: DatabaseTreeController,
) {
  const c = m.connections[p.id];
  if ((ui.scope && ui.scope.profileId !== p.id) || (ui.connectedOnly && !c))
    return false;
  if (!ui.term || ui.matches(p.name, p.host, p.environment, p.group))
    return true;
  return (
    c?.schemas.some(
      (database) =>
        (!ui.scope || ui.scope.database === database) &&
        (ui.matches(database) ||
          c.tables[database]?.some((t) => ui.matches(t.name, t.comment))),
    ) ?? false
  );
}
export function DatabaseConnectionTree({
  profile: p,
  model: m,
  controller: ui,
}: {
  profile: DatabaseProfile;
  model: DatabaseWorkspaceModel;
  controller: DatabaseTreeController;
}) {
  const {
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
    onInsertColumn,
  } = ui;
  const safely = (action: () => Promise<unknown>) => {
    void action().catch((e) => m.setError(errorText(e)));
  };

  if (scope && scope.profileId !== p.id) return null;
  const c = m.connections[p.id];
  const connectionNode: DatabaseNode = {
    kind: 'connection',
    profileId: p.id,
  };
  const match = matches(p.name, p.host, p.environment, p.group);
  const schemas = (c?.schemas ?? []).filter(
    (database) =>
      (!scope || scope.database === database) &&
      (showSystem || systemDatabase(database) === false || !!term || !!scope) &&
      (!term ||
        match ||
        matches(database) ||
        c?.tables[database]?.some((table) =>
          matches(table.name, table.comment),
        )),
  );
  if (connectedOnly && !c) return null;
  if (term && !match && !schemas.length) return null;
  const open = !!term || (expanded[nodeKey(connectionNode)] ?? !!c);
  return (
    <div key={p.id} className="oc-db-tree-connection" data-profile-id={p.id}>
      <div
        className="oc-db-connection-row"
        onContextMenu={(e) => openMenu(e, connectionNode)}
      >
        <button
          className="oc-db-node-toggle"
          aria-label={`${open ? '折叠' : '展开'}连接 ${p.name}`}
          aria-expanded={open}
          disabled={!!m.connecting[p.id]}
          onClick={() =>
            open
              ? setOpen(connectionNode, false)
              : safely(() => expandConnection(p, false))
          }
        >
          {open ? <TbChevronDown size={13} /> : <TbChevronRight size={13} />}
        </button>
        <button
          data-db-node={nodeKey(connectionNode)}
          className={`oc-db-connection ${selected === nodeKey(connectionNode) ? 'is-selected' : ''}`}
          title={`${p.host}:${p.port} · ${p.user}${p.environment ? ' · ' + p.environment : ''}`}
          onClick={() => select(connectionNode)}
          onDoubleClick={() => safely(() => expandConnection(p, true))}
          onKeyDown={(e) =>
            keyboard(
              e,
              connectionNode,
              () => safely(() => expandConnection(p, true)),
              () => setOpen(connectionNode, false),
            )
          }
        >
          <TbServer size={15} />
          <span>
            <strong>{p.name}</strong>
            <small>
              {p.environment ? `${p.environment} · ` : ''}
              {p.host}:{p.port}
            </small>
          </span>
          <i
            className={
              c ? 'is-online' : m.connecting[p.id] ? 'is-connecting' : ''
            }
            aria-label={
              c ? '已连接' : m.connecting[p.id] ? '正在连接' : '未连接'
            }
          />
        </button>
      </div>
      {open && c && (
        <div className="oc-db-tree-branch oc-db-connection-children">
          {schemas.map((database) => {
            const n: DatabaseNode = {
              kind: 'database',
              profileId: p.id,
              database,
            };
            const databaseMatch = match || matches(database);
            const tableList = c.tables[database];
            const databaseOpen =
              !!term || (expanded[nodeKey(n)] ?? !!tableList);
            const tables = tableList?.filter(
              (t) => !term || databaseMatch || matches(t.name, t.comment),
            );
            return (
              <div key={database} className="oc-db-tree-database">
                <button
                  data-db-node={nodeKey(n)}
                  className={`oc-db-schema ${selected === nodeKey(n) ? 'is-selected' : ''}`}
                  aria-expanded={databaseOpen}
                  disabled={m.running(p.id)}
                  onClick={() => {
                    select(n);
                    if (databaseOpen) setOpen(n, false);
                    else safely(() => expandDatabase(p.id, database));
                  }}
                  onContextMenu={(e) => openMenu(e, n)}
                  onKeyDown={(e) =>
                    keyboard(
                      e,
                      n,
                      () => safely(() => expandDatabase(p.id, database)),
                      () => setOpen(n, false),
                    )
                  }
                >
                  {databaseOpen ? (
                    <TbChevronDown size={13} />
                  ) : (
                    <TbChevronRight size={13} />
                  )}
                  <TbDatabase size={13} />
                  <span title={database}>{database}</span>
                </button>
                {databaseOpen && (
                  <div className="oc-db-tables oc-db-tree-branch">
                    {!tables ? (
                      <small className="oc-db-muted">
                        {m.metadataBusy[p.id]
                          ? '正在读取…'
                          : '展开或刷新以读取对象'}
                      </small>
                    ) : tables.length === 0 ? (
                      <small className="oc-db-muted">
                        {term ? '没有匹配的表或视图' : '没有表或视图'}
                      </small>
                    ) : (
                      (['BASE TABLE', 'VIEW'] as const).map((type) => {
                        const items = tables.filter((t) =>
                          type === 'VIEW'
                            ? t.type === 'VIEW'
                            : t.type !== 'VIEW',
                        );
                        if (!items.length) return null;
                        const categoryKey = JSON.stringify([
                          p.id,
                          database,
                          type,
                        ]);
                        const categoryOpen =
                          !!term || (expanded[categoryKey] ?? true);
                        return (
                          <div key={type} className="oc-db-object-category">
                            <button
                              className="oc-db-category"
                              aria-expanded={categoryOpen}
                              onClick={() =>
                                setExpanded((all) => ({
                                  ...all,
                                  [categoryKey]: !categoryOpen,
                                }))
                              }
                            >
                              {categoryOpen ? (
                                <TbChevronDown size={11} />
                              ) : (
                                <TbChevronRight size={11} />
                              )}
                              {type === 'VIEW' ? '视图' : '表'}
                              <small>{items.length}</small>
                            </button>
                            {categoryOpen && (
                              <div className="oc-db-tree-branch oc-db-category-items">
                                {items.map((t) => {
                                  const tableNode: Extract<
                                    DatabaseNode,
                                    { kind: 'table' }
                                  > = {
                                    kind: 'table',
                                    profileId: p.id,
                                    database,
                                    table: t.name,
                                    tableType: t.type,
                                  };
                                  const tableOpen =
                                    expanded[nodeKey(tableNode)] ?? false;
                                  const structure =
                                    c.structures[objectKey(database, t.name)];
                                  return (
                                    <div key={t.name}>
                                      <div
                                        className="oc-db-table-row"
                                        onContextMenu={(e) =>
                                          openMenu(e, tableNode)
                                        }
                                      >
                                        <button
                                          className="oc-db-node-toggle"
                                          aria-label={`${tableOpen ? '折叠' : '展开'}字段 ${t.name}`}
                                          aria-expanded={tableOpen}
                                          disabled={m.running(p.id)}
                                          onClick={() =>
                                            tableOpen
                                              ? setOpen(tableNode, false)
                                              : safely(() =>
                                                  expandTable(tableNode),
                                                )
                                          }
                                        >
                                          {tableOpen ? (
                                            <TbChevronDown size={11} />
                                          ) : (
                                            <TbChevronRight size={11} />
                                          )}
                                        </button>
                                        <button
                                          data-db-node={nodeKey(tableNode)}
                                          className={`oc-db-table ${selected === nodeKey(tableNode) ? 'is-selected' : ''}`}
                                          title={t.comment || t.name}
                                          disabled={m.running(p.id)}
                                          onClick={() => select(tableNode)}
                                          onDoubleClick={() =>
                                            safely(() =>
                                              m.openTable(
                                                p.id,
                                                database,
                                                t.name,
                                              ),
                                            )
                                          }
                                          onContextMenu={(e) =>
                                            openMenu(e, tableNode)
                                          }
                                          onKeyDown={(e) =>
                                            keyboard(
                                              e,
                                              tableNode,
                                              () =>
                                                safely(() =>
                                                  m.openTable(
                                                    p.id,
                                                    database,
                                                    t.name,
                                                  ),
                                                ),
                                              () => setOpen(tableNode, false),
                                            )
                                          }
                                        >
                                          <TbTable size={13} />
                                          <span>{t.name}</span>
                                          {t.type === 'VIEW' && (
                                            <small>视图</small>
                                          )}
                                        </button>
                                      </div>
                                      {tableOpen && (
                                        <div className="oc-db-columns oc-db-tree-branch">
                                          {!structure ? (
                                            <small className="oc-db-muted">
                                              {m.metadataBusy[p.id]
                                                ? '正在读取字段…'
                                                : '读取失败，请重新展开'}
                                            </small>
                                          ) : (
                                            structure.columns.rows.map(
                                              (row) => {
                                                const column = row[0] ?? '';
                                                const columnNode: DatabaseNode =
                                                  {
                                                    kind: 'column',
                                                    profileId: p.id,
                                                    database,
                                                    table: t.name,
                                                    column,
                                                  };
                                                return (
                                                  <button
                                                    key={column}
                                                    title={`${row[1] ?? ''}${row[6] ? ' · ' + row[6] : ''}`}
                                                    className={
                                                      selected ===
                                                      nodeKey(columnNode)
                                                        ? 'is-selected'
                                                        : ''
                                                    }
                                                    onClick={() =>
                                                      select(columnNode)
                                                    }
                                                    onContextMenu={(e) =>
                                                      openMenu(e, columnNode)
                                                    }
                                                    onDoubleClick={() =>
                                                      safely(() =>
                                                        onInsertColumn(
                                                          columnNode,
                                                        ),
                                                      )
                                                    }
                                                    onKeyDown={(e) =>
                                                      keyboard(e, columnNode)
                                                    }
                                                  >
                                                    <TbCode size={11} />
                                                    <span>{column}</span>
                                                    <small>{row[1]}</small>
                                                  </button>
                                                );
                                              },
                                            )
                                          )}
                                        </div>
                                      )}
                                    </div>
                                  );
                                })}
                              </div>
                            )}
                          </div>
                        );
                      })
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
