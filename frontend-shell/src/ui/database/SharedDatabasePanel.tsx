import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  TbChevronDown,
  TbChevronUp,
  TbDatabase,
  TbLock,
  TbRefresh,
  TbSearch,
  TbUsers,
} from 'react-icons/tb';
import { confirmDialog } from '../feedback/ConfirmDialog';
import { DatabaseMenu, type DatabaseMenuItem } from './DatabaseMenu';
import type {
  DatabaseProfile,
  SharedDatabaseEntry,
  SharedDatabaseImportResult,
  SharedDatabaseRuntime,
} from './types';

interface SharedDatabasePanelProps {
  runtime?: SharedDatabaseRuntime;
  searchTerm?: string;
  onImported(profile: DatabaseProfile, connect: boolean): Promise<void> | void;
  onMissingDriver(result: SharedDatabaseImportResult): void;
  onError?(message: string): void;
}

const PANEL_HEIGHT_KEY = 'opscopilot:sharedDbPanelHeight';
const PANEL_COLLAPSED_KEY = 'opscopilot:sharedDbPanelCollapsed';
const DEFAULT_PANEL_HEIGHT = 200;
const MIN_PANEL_HEIGHT = 104;
const MIN_TREE_HEIGHT = 140;

function loadPanelHeight(): number {
  try {
    const raw = Number(localStorage.getItem(PANEL_HEIGHT_KEY));
    return raw >= MIN_PANEL_HEIGHT ? raw : DEFAULT_PANEL_HEIGHT;
  } catch {
    return DEFAULT_PANEL_HEIGHT;
  }
}

function loadPanelCollapsed(): boolean {
  try {
    return localStorage.getItem(PANEL_COLLAPSED_KEY) === '1';
  } catch {
    return false;
  }
}

function relativeTime(lastLoginAt: string): string {
  if (!lastLoginAt) return '';
  const t = new Date(lastLoginAt.replace(' ', 'T'));
  if (isNaN(t.getTime())) return lastLoginAt;
  const diffMs = Date.now() - t.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return '刚刚';
  if (diffMin < 60) return `${diffMin} 分钟前`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour} 小时前`;
  const diffDay = Math.floor(diffHour / 24);
  if (diffDay < 30) return `${diffDay} 天前`;
  return t.toLocaleDateString();
}

function engineBadge(entry: SharedDatabaseEntry): string {
  const dialect = (entry.driverDialect || '').toLowerCase();
  if (entry.engine === 'mysql' || dialect === 'mysql') return 'MySQL';
  if (dialect === 'postgresql' || dialect === 'postgres') return 'PG';
  if (dialect === 'oracle') return 'Oracle';
  if (dialect === 'sqlserver' || dialect === 'mssql') return 'SQLServer';
  if (dialect === 'dm' || dialect === 'dameng') return '达梦';
  if (entry.driverName) {
    return entry.driverName.slice(0, 10);
  }
  return entry.engine ? entry.engine.toUpperCase() : 'DB';
}

export function SharedDatabasePanel({
  runtime,
  searchTerm,
  onImported,
  onMissingDriver,
  onError,
}: SharedDatabasePanelProps) {
  const [enabled, setEnabled] = useState(false);
  const [entries, setEntries] = useState<SharedDatabaseEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [height, setHeight] = useState<number>(loadPanelHeight);
  const [collapsed, setCollapsed] = useState<boolean>(loadPanelCollapsed);
  const [menu, setMenu] = useState<{
    x: number;
    y: number;
    entry: SharedDatabaseEntry;
  } | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const heightRef = useRef(height);
  heightRef.current = height;

  const load = useCallback(async () => {
    if (!runtime) {
      setEnabled(false);
      setEntries([]);
      return;
    }
    try {
      const res = await runtime.list();
      setEnabled(!!res.enabled);
      setEntries(Array.isArray(res.databases) ? res.databases : []);
    } catch {
      setEnabled(false);
      setEntries([]);
    }
  }, [runtime]);

  useEffect(() => {
    if (!runtime) return;
    void load();
    const wailsRuntime = (window as unknown as {
      runtime?: { EventsOn?: (event: string, cb: () => void) => () => void };
    }).runtime;
    const off = wailsRuntime?.EventsOn?.('session-share:synced', () => {
      void load();
    });
    const onSettingsChanged = () => {
      void load();
    };
    window.addEventListener('settings-changed', onSettingsChanged);
    return () => {
      off?.();
      window.removeEventListener('settings-changed', onSettingsChanged);
    };
  }, [runtime, load]);

  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(PANEL_COLLAPSED_KEY, next ? '1' : '0');
      } catch {
        // ignore storage errors
      }
      return next;
    });
  };

  const startResize = (mouseDownEvent: React.MouseEvent) => {
    mouseDownEvent.preventDefault();
    const container = panelRef.current?.parentElement;
    if (!container) return;
    const containerRect = container.getBoundingClientRect();
    const maxHeight = Math.max(
      MIN_PANEL_HEIGHT,
      containerRect.height - MIN_TREE_HEIGHT,
    );

    const onMove = (e: MouseEvent) => {
      const next = containerRect.bottom - e.clientY;
      setHeight(Math.min(Math.max(next, MIN_PANEL_HEIGHT), maxHeight));
    };
    const onUp = () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      try {
        localStorage.setItem(
          PANEL_HEIGHT_KEY,
          String(Math.round(heightRef.current)),
        );
      } catch {
        // ignore
      }
    };
    document.body.style.cursor = 'ns-resize';
    document.body.style.userSelect = 'none';
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  };

  const handleImport = async (entry: SharedDatabaseEntry, connect: boolean) => {
    if (!runtime || busyKey) return;
    setBusyKey(entry.entryKey);
    try {
      const result = await runtime.importToLocal(entry.entryKey);
      if (result.missingDriver) {
        onMissingDriver(result);
        return;
      }
      if (!result.success || !result.profile) {
        onError?.(result.message || '导入共享数据库连接失败');
        return;
      }
      await onImported(result.profile, connect);
    } catch (e) {
      onError?.(e instanceof Error ? e.message : String(e));
    } finally {
      setBusyKey(null);
    }
  };

  const handleRemove = async (entry: SharedDatabaseEntry) => {
    if (!runtime) return;
    const ok = await confirmDialog.show({
      title: '删除共享数据库连接',
      message: `确定从团队共享中移除「${entry.name || entry.host}」吗？\n\n这不会删除你本地已保存的数据库连接。`,
      confirmText: '删除共享',
      danger: true,
    });
    if (!ok) return;
    try {
      await runtime.remove(entry.entryKey);
      await load();
    } catch (e) {
      onError?.(e instanceof Error ? e.message : String(e));
    }
  };

  const term = (searchTerm || '').trim().toLowerCase();
  const displayed = useMemo(() => {
    if (!term) return entries;
    return entries.filter((e) =>
      [
        e.name,
        e.host,
        e.user,
        e.database,
        e.owner,
        e.driverName,
        e.driverDialect,
        e.environment,
      ]
        .filter(Boolean)
        .some((v) => v!.toLowerCase().includes(term)),
    );
  }, [entries, term]);

  if (!enabled) {
    return null;
  }

  const menuItems = (entry: SharedDatabaseEntry): DatabaseMenuItem[] => [
    {
      label: '打开连接',
      action: () => void handleImport(entry, true),
    },
    {
      label: '仅保存到本地',
      action: () => void handleImport(entry, false),
    },
    ...(entry.own
      ? [
          {
            label: '删除共享',
            separator: true,
            danger: true,
            action: () => void handleRemove(entry),
          },
        ]
      : []),
  ];

  return (
    <section
      ref={panelRef}
      className="oc-db-shared"
      style={
        collapsed || displayed.length === 0
          ? { flexShrink: 0 }
          : { height: `${height}px`, flexShrink: 0 }
      }
      aria-label="团队共享数据库"
    >
      {!collapsed && displayed.length > 0 && (
        <div
          className="oc-db-shared-resize"
          onMouseDown={startResize}
          title="拖拽调整团队共享区域高度"
        />
      )}
      <div className="oc-db-shared-header">
        <span className="oc-db-shared-title-wrap">
          <TbUsers size={13} />
          <span className="oc-db-shared-title">团队共享</span>
          <span className="oc-db-shared-count">{displayed.length}</span>
        </span>
        <span className="oc-db-shared-actions">
          {!collapsed && (
            <button
              type="button"
              className="oc-db-shared-icon-btn"
              title="刷新共享数据库"
              disabled={loading}
              onClick={() => {
                setLoading(true);
                void load().finally(() => setLoading(false));
              }}
            >
              <TbRefresh size={13} />
            </button>
          )}
          <button
            type="button"
            className="oc-db-shared-icon-btn"
            title={collapsed ? '展开团队共享' : '折叠团队共享'}
            onClick={toggleCollapsed}
          >
            {collapsed ? <TbChevronUp size={13} /> : <TbChevronDown size={13} />}
          </button>
        </span>
      </div>

      {!collapsed && (
        <div className="oc-db-shared-list">
          {displayed.length === 0 ? (
            <div className="oc-db-shared-empty">
              {term ? (
                <span className="oc-db-shared-empty-search">
                  <TbSearch size={12} /> 无匹配的共享数据库
                </span>
              ) : (
                '暂无共享数据库'
              )}
            </div>
          ) : (
            displayed.map((entry) => {
              const locked = entry.hasSecrets && !entry.decryptable;
              const isBusy = busyKey === entry.entryKey;
              const tooltipParts = [
                `${entry.user}@${entry.host}:${entry.port}${entry.database ? '/' + entry.database : ''}`,
                entry.hasGateway
                  ? `SSH 隧道: ${entry.gatewayHost || '已启用'}`
                  : '',
                entry.environment ? `环境: ${entry.environment}` : '',
                `共享者: ${entry.owner}`,
                entry.lastLoginAt ? `最近连接: ${entry.lastLoginAt}` : '',
                locked ? '密钥不匹配，无法解密密码' : '双击自动导入并连接',
              ].filter(Boolean);

              return (
                <div
                  key={entry.entryKey}
                  className={`oc-db-shared-row${isBusy ? ' is-busy' : ''}`}
                  title={tooltipParts.join(' · ')}
                  onDoubleClick={() => void handleImport(entry, true)}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setMenu({ x: e.clientX, y: e.clientY, entry });
                  }}
                >
                  <span
                    className={`oc-db-shared-row-icon${locked ? ' is-locked' : ''}`}
                  >
                    {locked ? <TbLock size={14} /> : <TbDatabase size={14} />}
                  </span>
                  <div className="oc-db-shared-row-info">
                    <div className="oc-db-shared-row-name-line">
                      <span className="oc-db-shared-row-name">
                        {entry.name || entry.host}
                      </span>
                      <span className="oc-db-shared-chip">
                        {engineBadge(entry)}
                      </span>
                      {entry.readOnly && (
                        <span className="oc-db-shared-chip is-readonly">
                          只读
                        </span>
                      )}
                    </div>
                    <div className="oc-db-shared-row-meta">
                      <span>
                        {entry.user}@{entry.host}:{entry.port}
                      </span>
                      <span className="oc-db-shared-owner">· {entry.owner}</span>
                      {entry.lastLoginAt && (
                        <span className="oc-db-shared-time">
                          · {relativeTime(entry.lastLoginAt)}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      )}

      {menu && (
        <DatabaseMenu
          x={menu.x}
          y={menu.y}
          items={menuItems(menu.entry)}
          onClose={() => setMenu(null)}
        />
      )}
    </section>
  );
}
