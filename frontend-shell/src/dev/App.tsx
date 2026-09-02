/**
 * Shell 插件 dev harness：无平台宿主时的独立验证页，布局对齐现有 Shell 入口。
 * 覆盖 S2~S8：连接管理（含堡垒机）、多标签 keep-alive、分屏、出窗、
 * 快捷命令、文件传输、脚本、资源监控。
 * 平台到手后布局与交互由插件 UI 继承，core/* 原样复用。
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { TbScreenShare, TbCode, TbBolt, TbStethoscope } from 'react-icons/tb';
import { SidecarClient, QuickCommand } from '../core';
import { SidecarTerminal, makeSidecarConfigRuntime, makeSidecarFileTransferHost, makeSidecarScriptRuntime, makeSidecarShellSettingsRuntime, makeSidecarAIConfigRuntime, makeSidecarDiagnoseRuntime } from '../adapters/sidecar';
import {
  FlexLayoutAdapter, SessionStatus, TerminalRef, TerminalRuntime,
  SessionManager, SmartConnectModal, QuickCommandPanel, FilesPanel,
  ScriptRecordingPanel, ScriptListPanel, ScriptEditorModal, ShellSettingsModal, DiagnosePanel,
  CommandQueryOverlay, isEditableTarget, matchesShortcut,
  normalizeTerminalConfig, TerminalConfig, HighlightRule,
  logoUniversal as logo, Theme, readPersistedTheme, persistTheme, HostCapabilities,
} from '../ui';
import type { CompletionData } from '../ui/Terminal/CompletionOverlay';
import { sendToTerminal } from '../core/terminalRegistry';
import { MonitorPopover } from './MonitorPopover';

interface Tab {
  terminalId: string;
  connectionId: string;
  title: string;
  host: string;
  user: string;
}

const DEFAULT_RPC_URL = 'ws://127.0.0.1:9777/rpc?token=devtoken';
const DEFAULT_WS_BASE = 'ws://127.0.0.1:9777';
const DEFAULT_TOKEN = 'devtoken';

interface DetachParams {
  terminalId: string;
  wsBase: string;
  token: string;
  rpcUrl: string;
  title: string;
}

export interface SidecarEndpoint {
  rpcUrl: string;
  wsBase: string;
  token: string;
}

export interface SidecarShellAppProps {
  endpoint?: SidecarEndpoint;
  autoConnect?: boolean;
  capabilities?: Partial<HostCapabilities>;
}

const DEFAULT_CAPABILITIES: HostCapabilities = {
  standaloneChrome: true,
  fileTransfer: true,
  // 能力位与实现同源（方案 D5）：sidecar 无条件接线补全服务，故默认可用；
  // 经 ?completion=0 可关闭以验证门控。
  terminalCompletion: true,
};

/** 出窗模式：独立终端窗口，带完整产品外壳（标题栏/状态栏/断开动作）。
 *  数据面在 sidecar——本窗口刷新或关闭都不影响终端会话本身。 */
const DetachView: React.FC<DetachParams> = ({ terminalId, wsBase, token, rpcUrl, title }) => {
  const [client, setClient] = useState<SidecarClient | null>(null);
  const [error, setError] = useState('');
  const [lost, setLost] = useState(false);
  useEffect(() => {
    const c = new SidecarClient({ url: rpcUrl });
    c.open().then(() => setClient(c)).catch((err) => setError((err as Error).message));
    return () => c.close();
  }, [rpcUrl]);

  const disconnectAndClose = useCallback(async () => {
    if (client) {
      try { await client.closeTerminal(terminalId); } catch { /* 已退出视为成功 */ }
    }
    window.close();
  }, [client, terminalId]);

  return (
    <div className="detach-root">
      <header className="titlebar">
        <span className="brand"><span className="brand-mark">◆</span>OpsCopilot Shell · {title || '独立终端'}</span>
        <span className="titlebar-status">
          <span className={`dot${client && !lost ? ' ok' : ''}`} />
          {client && !lost ? '已连接' : lost ? '终端已断开' : '连接中…'}
        </span>
        <div className="titlebar-actions">
          <button className="ghost-btn" onClick={() => void disconnectAndClose()}>断开并关闭</button>
        </div>
      </header>
      <div className="term-stack" style={{ flex: 1 }}>
        {client
          ? <SidecarTerminal client={client} terminalId={terminalId} wsBase={wsBase} token={token} onLost={() => setLost(true)} />
          : <div className="term-empty">{error || '正在重连 sidecar…'}</div>}
      </div>
      <footer className="statusbar">
        <span className="seg">独立窗口</span>
        <span className="seg">终端 <b>{terminalId.slice(0, 14)}…</b></span>
        <span className="spacer" />
        <span className="seg" title={rpcUrl}>sidecar · {wsBase.replace('ws://', '')}</span>
      </footer>
    </div>
  );
};

export const App: React.FC<SidecarShellAppProps> = ({ endpoint, autoConnect = false, capabilities }) => {
  // 出窗模式短路
  const detachParams = useMemo(() => new URLSearchParams(window.location.search), []);
  const isDetach = detachParams.get('detach') !== null;

  const [rpcUrl, setRpcUrl] = useState(detachParams.get('rpc') ?? endpoint?.rpcUrl ?? DEFAULT_RPC_URL);
  const [wsBase, setWsBase] = useState(detachParams.get('wsbase') ?? endpoint?.wsBase ?? DEFAULT_WS_BASE);
  const [token, setToken] = useState(detachParams.get('token') ?? endpoint?.token ?? DEFAULT_TOKEN);
  const [detachTitle] = useState(detachParams.get('title') ?? '');
  const [client, setClient] = useState<SidecarClient | null>(null);
  const [connectingSidecar, setConnectingSidecar] = useState(isDetach);
  const [error, setError] = useState('');
  const [toast, setToast] = useState('');

  const [tabs, setTabs] = useState<Tab[]>([]);
  const [activeTab, setActiveTab] = useState('');
  const terminalRefs = useRef(new Map<string, TerminalRef>());

  const [qcOpen, setQcOpen] = useState(true);
  const [smartModalOpen, setSmartModalOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [sidebarTab, setSidebarTab] = useState<'sessions' | 'script' | 'diagnose'>('sessions');
  const [theme, setThemeState] = useState<Theme>(readPersistedTheme);
  const [editingScriptId, setEditingScriptId] = useState<string | null>(null);
  const scriptListRef = useRef<{ loadScripts: () => void } | null>(null);
  const [monitorOpen, setMonitorOpen] = useState(false);
  // Shell 设置（阶段 5）：后端落盘，重启保持
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [terminalConfig, setTerminalConfig] = useState<TerminalConfig>(() => normalizeTerminalConfig(undefined));
  const [completionDelay, setCompletionDelay] = useState(150);
  const [highlightRules, setHighlightRules] = useState<HighlightRule[]>([]);
  const [commandQueryShortcut, setCommandQueryShortcut] = useState('Ctrl+K');
  // 迭代 B：AI 单发能力（命令生成 + 智能连接），按配置门控
  const [aiConfigured, setAiConfigured] = useState(false);
  const [cqOpen, setCqOpen] = useState(false);
  const [cqText, setCqText] = useState('');
  const [cqLoading, setCqLoading] = useState(false);
  const [cqError, setCqError] = useState('');
  const [cqResult, setCqResult] = useState<{ command: string; explanation?: string } | null>(null);
  const autoConnectRequested = autoConnect || detachParams.get('autoconnect') === '1';
  const hostCapabilities = useMemo(() => ({
    ...DEFAULT_CAPABILITIES,
    standaloneChrome: detachParams.get('standalone') === '0' ? false : DEFAULT_CAPABILITIES.standaloneChrome,
    fileTransfer: detachParams.get('filetransfer') === '0' ? false : DEFAULT_CAPABILITIES.fileTransfer,
    terminalCompletion: detachParams.get('completion') === '0' ? false : DEFAULT_CAPABILITIES.terminalCompletion,
    ...capabilities,
  }), [capabilities, detachParams]);

  const connectSidecar = useCallback(async () => {
    setConnectingSidecar(true);
    setError('');
    const c = new SidecarClient({ url: rpcUrl });
    try {
      await c.open();
      // 手动更换控制面后，数据面随握手结果更新；宿主显式指定的代理地址仍优先。
      const initialized = await c.initialize();
      setWsBase(detachParams.get('wsbase') ?? endpoint?.wsBase ?? initialized.wsBase);
      setToken(detachParams.get('token') ?? endpoint?.token ?? initialized.token);
      c.on('terminal/exited', (params: any) => {
        const id = params?.terminalId;
        if (id) setTabs((ts) => ts.filter((t) => t.terminalId !== id));
      });
      setClient(c);
      // 拉取 Shell 设置：主题/终端参数/补全延迟/高亮规则/命令查询快捷键
      try {
        const st = await c.settingsGet() as any;
        if (st?.theme === 'light' || st?.theme === 'dark') {
          setThemeState(st.theme);
          persistTheme(st.theme);
        }
        if (st?.terminal) setTerminalConfig(normalizeTerminalConfig(st.terminal));
        if (typeof st?.completionDelay === 'number') setCompletionDelay(st.completionDelay);
        if (Array.isArray(st?.highlightRules)) setHighlightRules(st.highlightRules);
        if (typeof st?.commandQueryShortcut === 'string' && st.commandQueryShortcut.trim()) {
          setCommandQueryShortcut(st.commandQueryShortcut.trim());
        }
      } catch { /* 设置缺失时用默认 */ }
      // 拉取 AI 配置状态：决定命令生成/智能连接入口可见性
      try {
        const ai = await c.aiGetConfig() as any;
        setAiConfigured(!!ai?.configured);
      } catch { setAiConfigured(false); }
    } catch (err) {
      c.close();
      setError((err as Error).message);
    } finally {
      setConnectingSidecar(false);
    }
  }, [rpcUrl, detachParams, endpoint]);

  const showToast = useCallback((text: string) => {
    setToast(text);
    window.setTimeout(() => setToast(''), 2400);
  }, []);

  const openSession = useCallback(async (config: { host: string; port?: number; user: string; password: string; name?: string; bastion?: any }) => {
    if (!client) return false;
    setError('');
    try {
      const { connectionId } = await client.connect(config);
      const { terminalId } = await client.openTerminal(connectionId, 80, 24);
      const title = config.name || `${config.user}@${config.host}`;
      setTabs((ts) => [...ts, { terminalId, connectionId, title, host: config.host, user: config.user }]);
      setActiveTab(terminalId);
      return true;
    } catch (err) {
      setError((err as Error).message);
      return false;
    }
  }, [client]);

  const closeTab = useCallback(async (tab: Tab) => {
    setTabs((ts) => {
      const next = ts.filter((t) => t.terminalId !== tab.terminalId);
      if (activeTab === tab.terminalId) setActiveTab(next.at(-1)?.terminalId ?? '');
      return next;
    });
    try {
      await client?.closeTerminal(tab.terminalId);
    } catch { /* 已退出视为成功 */ }
  }, [client, activeTab]);

  const disconnectAll = useCallback(async () => {
    if (!client) return;
    for (const tab of tabs) {
      try { await client.closeTerminal(tab.terminalId); } catch { /* ignore */ }
    }
    setTabs([]);
    setActiveTab('');
    showToast('已断开全部终端');
  }, [client, tabs, showToast]);

  const detachTab = useCallback((tab: Tab) => {
    const url = `${window.location.pathname}?detach=${encodeURIComponent(tab.terminalId)}&wsbase=${encodeURIComponent(wsBase)}&token=${encodeURIComponent(token)}&rpc=${encodeURIComponent(rpcUrl)}&title=${encodeURIComponent(tab.title)}`;
    window.open(url, '_blank', 'width=920,height=620');
  }, [wsBase, token, rpcUrl]);

  const sendToActive = useCallback((content: string) => {
    if (!activeTab) return;
    if (sendToTerminal(activeTab, content.endsWith('\n') ? content : content + '\r')) {
      showToast('已发送到当前终端');
    }
  }, [activeTab, showToast]);

  // 迭代 B：设置弹窗关闭后刷新 AI 配置状态（保存密钥后入口立即出现）
  const refreshAiConfigured = useCallback(async () => {
    try {
      const ai = await client?.aiGetConfig() as any;
      setAiConfigured(!!ai?.configured);
    } catch { setAiConfigured(false); }
  }, [client]);

  const generateCommand = useCallback(async (overrideText?: string) => {
    const text = (overrideText ?? cqText).trim();
    if (!text || !client) return;
    if (overrideText !== undefined) setCqText(overrideText);
    setCqLoading(true);
    setCqError('');
    try {
      const result = await client.aiGenerateCommand(text);
      setCqResult(result);
    } catch (e: any) {
      setCqError((e?.message || e?.toString?.() || '生成失败') as string);
      setCqResult(null);
    } finally {
      setCqLoading(false);
    }
  }, [client, cqText]);

  const copyGeneratedCommand = useCallback(() => {
    const cmd = cqResult?.command?.trim();
    if (!cmd) return;
    void navigator.clipboard?.writeText(cmd);
  }, [cqResult]);

  // 命令查询快捷键（迭代 B）：AI 已配置且有激活终端时呼出
  useEffect(() => {
    if (!aiConfigured) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (isEditableTarget(e.target)) return;
      if (matchesShortcut(e, commandQueryShortcut)) {
        e.preventDefault();
        e.stopPropagation();
        if (!activeTab) { showToast('请先选择一个激活的终端'); return; }
        setCqText('');
        setCqError('');
        setCqResult(null);
        setCqOpen(true);
      }
    };
    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [aiConfigured, commandQueryShortcut, activeTab, showToast]);

  useEffect(() => () => client?.close(), [client]);

  // 主题：与产品同机制（data-theme 写 <html>，localStorage 持久化）
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);
  const toggleTheme = useCallback(() => {
    setThemeState((prev) => {
      const next = prev === 'dark' ? 'light' : 'dark';
      persistTheme(next);
      return next;
    });
  }, []);

  // 右侧导航：与产品 Sidebar 相同的开关语义（同 tab 再点 = 收起）
  const toggleSidebarPanel = useCallback((tab: 'sessions' | 'script' | 'diagnose') => {
    setSidebarOpen((open) => {
      if (open && sidebarTab === tab) return false;
      setSidebarTab(tab);
      return true;
    });
  }, [sidebarTab]);

  // 出窗与正式宿主模式自动连接 sidecar；dev harness 仍保留手动连接门。
  const detachId = detachParams.get('detach') ?? '';
  useEffect(() => {
    if (isDetach || autoConnectRequested) void connectSidecar();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isDetach, autoConnectRequested]);

  const activeTabObj = tabs.find((t) => t.terminalId === activeTab);
  const layoutTerminals = useMemo(() => tabs.map((tab) => ({
    id: tab.terminalId,
    title: tab.title,
    status: SessionStatus.CONNECTED,
    config: { host: tab.host, user: tab.user, port: 22 },
  })), [tabs]);
  const terminalRuntime = useMemo<TerminalRuntime>(() => ({
    resize: (terminalId, cols, rows) => client?.resize(terminalId, cols, rows),
    // 补全经能力位门控（D5）：位关时不提供实现，终端静默关闭补全。
    ...(hostCapabilities.terminalCompletion && client ? {
      getCompletions: (input: string, cursor: number) =>
        client.getCompletions(input, cursor) as Promise<CompletionData | null>,
    } : {}),
  }), [client, hostCapabilities.terminalCompletion]);

  // 会话树 / 快捷命令宿主适配器：依赖 client，构建一次。
  const sidecarConfigRuntime = useMemo(
    () => (client ? makeSidecarConfigRuntime(client) : null),
    [client],
  );
  const quickCommandHost = useMemo(
    () => (sidecarConfigRuntime ? sidecarConfigRuntime.quickCommandHost(sendToActive) : null),
    [sidecarConfigRuntime, sendToActive],
  );
  // 文件传输宿主（阶段 4）：共享 FilesPanel 经 shell.ft.*/shell.fs.* RPC 驱动
  const ftHost = useMemo(
    () => (client ? makeSidecarFileTransferHost(client) : null),
    [client],
  );
  // 结构化脚本宿主（阶段 5）：录制/编辑/变量回放经 shell.script.* RPC 驱动
  const scriptRuntime = useMemo(
    () => (client ? makeSidecarScriptRuntime(client) : null),
    [client],
  );
  const handleReplayScript = useCallback(async (scriptId: string) => {
    if (!activeTab) return;
    const ok = await confirmDialogReplay();
    if (!ok) return;
    try {
      await scriptRuntime?.replay(scriptId, activeTab);
      showToast('脚本回放完成');
    } catch (err) {
      setError(`回放失败: ${(err as Error).message}`);
    }
  }, [activeTab, scriptRuntime, showToast]);
  const confirmDialogReplay = useCallback(async () => {
    // 共享确认框走 confirmDialog（feedback 层）；此处直接复用
    const { confirmDialog } = await import('../ui');
    return confirmDialog.show({
      title: '回放脚本',
      message: '确定要在当前会话中回放此脚本吗？',
      confirmText: '回放',
    });
  }, []);

  // ---------- 出窗模式 ----------
  if (isDetach) {
    return (
      <DetachView terminalId={detachId} wsBase={wsBase} token={token} rpcUrl={rpcUrl} title={detachTitle} />
    );
  }

  // ---------- sidecar 接入门 ----------
  if (!client) {
    return (
      <div className="shell-root">
        <div className="sidecar-gate">
          <div className="gate-card">
            <div className="brand-mark">◆</div>
            <strong>OpsCopilot Shell</strong>
            <span className="muted">连接本地 shell-sidecar 后开始工作</span>
            <label className="gate-field">
              <span>控制面</span>
              <input value={rpcUrl} onChange={(e) => setRpcUrl(e.target.value)} data-testid="rpc-url" />
            </label>
            <button className="btn-primary" disabled={connectingSidecar} data-testid="connect-sidecar" onClick={() => void connectSidecar()}>
              {connectingSidecar ? '连接中…' : '连接 sidecar'}
            </button>
            {error && <div className="error-banner">{error}</div>}
          </div>
        </div>
      </div>
    );
  }

  // ---------- 主工作台 ----------
  return (
    <div className="shell-root">
      <header className="titlebar">
        <span className="brand"><img src={logo} alt="OpsCopilot" className="brand-logo shell-brand-logo" />OpsCopilot Shell</span>
        <span className="titlebar-status"><span className="dot ok" />sidecar 已连接</span>
        <div className="titlebar-actions">
          <button className="btn-primary" data-testid="open-connect-modal" onClick={() => setSmartModalOpen(true)}>＋ 新建连接</button>
          {tabs.length > 0 && <button className="ghost-btn" onClick={() => void disconnectAll()}>断开全部</button>}
          <button className="ghost-btn" data-testid="open-settings" title="Shell 设置" onClick={() => setSettingsOpen(true)}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3"></circle>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
            </svg>
          </button>
          <button className="ghost-btn" onClick={toggleTheme} title={theme === 'dark' ? '切换到亮色' : '切换到暗色'}>
            {theme === 'dark' ? (
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="4" />
                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
              </svg>
            ) : (
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
              </svg>
            )}
          </button>
        </div>
      </header>

      <div className="shell-body">
        <main className="shell-main">
          <div className="shared-layout-host">
            <FlexLayoutAdapter
              terminals={layoutTerminals}
              onTerminalData={(terminalId, data) => { sendToTerminal(terminalId, data); }}
              terminalRefs={terminalRefs}
              onCloseTerminal={(terminalId) => {
                const tab = tabs.find((item) => item.terminalId === terminalId);
                if (tab) void closeTab(tab);
              }}
              onRenameTerminal={(terminalId, title) => {
                setTabs((items) => items.map((item) => item.terminalId === terminalId ? { ...item, title } : item));
              }}
              activeTerminalId={activeTab || null}
              onActiveTerminalChange={(terminalId) => setActiveTab(terminalId ?? '')}
              terminalRuntime={terminalRuntime}
              theme={theme}
              completionDelay={completionDelay}
              terminalConfig={terminalConfig}
              highlightRules={highlightRules}
              renderTerminal={(terminalId, attachRef) => {
                const tab = tabs.find((item) => item.terminalId === terminalId);
                return (
                  <SidecarTerminal
                    client={client}
                    terminalId={terminalId}
                    wsBase={wsBase}
                    token={token}
                    theme={theme}
                    terminalConfig={terminalConfig}
                    completionDelay={completionDelay}
                    highlightRules={highlightRules}
                    getCompletions={terminalRuntime.getCompletions}
                    terminalRef={attachRef}
                    onLost={(_id, reason) => setError(`终端 ${tab?.title ?? terminalId} 断开：${reason}`)}
                  />
                );
              }}
              onDetachTerminal={hostCapabilities.standaloneChrome ? (terminalId) => {
                const tab = tabs.find((item) => item.terminalId === terminalId);
                if (tab) detachTab(tab);
              } : undefined}
              renderFileTransfer={(terminalId, terminalList) => (
                hostCapabilities.fileTransfer && ftHost
                  ? <FilesPanel activeTerminalId={terminalId} terminals={terminalList} host={ftHost} />
                  : null
              )}
            />
            {toast && <div className="toast" role="status">{toast}</div>}
            {monitorOpen && (
              <MonitorPopover client={client} connectionId={activeTabObj?.connectionId ?? null} hostLabel={activeTabObj ? `${activeTabObj.user}@${activeTabObj.host}` : ''} onClose={() => setMonitorOpen(false)} />
            )}
          </div>

          {quickCommandHost && (
            <QuickCommandPanel
              isOpen={qcOpen}
              onExecute={(content) => sendToActive(content)}
              host={quickCommandHost}
            />
          )}

          {error && <div className="error-banner" role="alert" data-testid="error-banner">{error}</div>}
        </main>

        {/* 右侧侧栏：产品基准结构（位于终端区与图标导航之间），含会话树/脚本两个页签 */}
        {sidebarOpen && (
          <aside className="h-sidebar">
            <div className="h-sidebar-head">
              <h3>{sidebarTab === 'sessions' ? '会话管理' : sidebarTab === 'script' ? '脚本' : 'AI 诊断'}</h3>
              <button className="h-sidebar-close" aria-label="收起侧栏" onClick={() => setSidebarOpen(false)}>×</button>
            </div>
            <div style={{ display: sidebarTab === 'sessions' ? 'flex' : 'none', flex: 1, minHeight: 0, flexDirection: 'column', overflow: 'hidden' }}>
              {sidecarConfigRuntime && (
                <SessionManager
                  onConnect={(config) => void openSession(config as any)}
                  runtime={sidecarConfigRuntime.sessionRuntime}
                />
              )}
            </div>
            <div style={{ display: sidebarTab === 'script' ? 'flex' : 'none', flex: 1, minHeight: 0, flexDirection: 'column', overflow: 'hidden' }}>
              {scriptRuntime && (
                <>
                  <ScriptRecordingPanel
                    activeSessionId={activeTab || null}
                    onRecordingComplete={() => scriptListRef.current?.loadScripts()}
                    runtime={scriptRuntime}
                  />
                  <ScriptListPanel
                    ref={scriptListRef}
                    activeSessionId={activeTab || null}
                    onEditScript={(id) => setEditingScriptId(id)}
                    onReplayScript={(id) => void handleReplayScript(id)}
                    runtime={scriptRuntime}
                  />
                </>
              )}
            </div>
            {client && aiConfigured && (
              <div style={{ display: sidebarTab === 'diagnose' ? 'flex' : 'none', flex: 1, minHeight: 0, flexDirection: 'column', overflow: 'hidden' }}>
                <DiagnosePanel
                  runtime={makeSidecarDiagnoseRuntime(() => client)}
                  bindTo={activeTabObj ? { terminalId: activeTabObj.terminalId, host: activeTabObj.host, user: activeTabObj.user } : undefined}
                />
              </div>
            )}
          </aside>
        )}

        {/* 右侧图标导航（40px，产品基准）：只展示 harness 具备的能力入口 */}
        <nav className="h-rightnav">
          <div
            className={`h-nav-icon${sidebarOpen && sidebarTab === 'sessions' ? ' on' : ''}`}
            title="会话管理"
            onClick={() => toggleSidebarPanel('sessions')}
          >
            {TbScreenShare({ size: 20 })}
          </div>
          <div
            className={`h-nav-icon${sidebarOpen && sidebarTab === 'script' ? ' on' : ''}`}
            title="脚本"
            onClick={() => toggleSidebarPanel('script')}
          >
            {TbCode({ size: 20 })}
          </div>
          {client && aiConfigured && (
            <div
              className={`h-nav-icon${sidebarOpen && sidebarTab === 'diagnose' ? ' on' : ''}`}
              title="AI 诊断"
              onClick={() => toggleSidebarPanel('diagnose')}
            >
              {TbStethoscope({ size: 20 })}
            </div>
          )}
          <div className="h-nav-spacer" />
          <div
            className={`h-nav-icon${qcOpen ? ' on' : ''}`}
            title="快捷命令"
            data-testid="toggle-qc"
            onClick={() => setQcOpen((o) => !o)}
          >
            {TbBolt({ size: 20 })}
          </div>
        </nav>
      </div>

      <footer className="statusbar">
        <span className="seg">连接 <b>{new Set(tabs.map((t) => t.connectionId)).size}</b></span>
        <span className="seg">终端 <b>{tabs.length}</b></span>
        {activeTabObj && <span className="seg"><b>{activeTabObj.user}@{activeTabObj.host}</b></span>}
        <span className="spacer" />
        <span className="seg click" data-testid="toggle-monitor" onClick={() => setMonitorOpen((o) => !o)}>资源</span>
        <span className="seg" title={rpcUrl}>sidecar · {wsBase.replace('ws://', '')}</span>
      </footer>

      <SmartConnectModal
        isOpen={smartModalOpen}
        onClose={() => setSmartModalOpen(false)}
        onConnect={(configs) => {
          for (const config of configs) {
            // Sidecar 新建连接：先落盘保存（带 name/host/user 的可存），再连接打开终端。
            if (config.name || config.host) {
              void client?.saveConfig({
                name: config.name || `${config.user}@${config.host}`,
                host: config.host,
                port: config.port,
                user: config.user,
                password: config.password ?? '',
                group: config.group,
              } as any);
            }
            void openSession(config as any);
          }
        }}
        onParse={async (input) => {
          if (!client) throw new Error('sidecar 未连接');
          const { configs } = await client.aiParseIntent(input);
          return configs as any;
        }}
        showAi={aiConfigured}
      />

      <ScriptEditorModal
        isOpen={editingScriptId !== null}
        scriptId={editingScriptId}
        onClose={() => setEditingScriptId(null)}
        onSave={() => scriptListRef.current?.loadScripts()}
        runtime={scriptRuntime!}
      />

      {client && (
        <ShellSettingsModal
          isOpen={settingsOpen}
          onClose={() => { setSettingsOpen(false); void refreshAiConfigured(); }}
          runtime={makeSidecarShellSettingsRuntime(() => client)}
          aiRuntime={makeSidecarAIConfigRuntime(() => client)}
          initial={{ theme, terminal: terminalConfig, completionDelay, highlightRules, commandQueryShortcut }}
          onApply={(next) => {
            setThemeState(next.theme);
            persistTheme(next.theme);
            setTerminalConfig(normalizeTerminalConfig(next.terminal));
            setCompletionDelay(next.completionDelay);
            setHighlightRules(next.highlightRules);
            if (next.commandQueryShortcut?.trim()) setCommandQueryShortcut(next.commandQueryShortcut.trim());
          }}
        />
      )}

      <CommandQueryOverlay
        visible={cqOpen}
        query={cqText}
        loading={cqLoading}
        result={cqResult}
        error={cqError}
        onQueryChange={setCqText}
        onGenerate={() => void generateCommand()}
        onRegenerate={() => void generateCommand()}
        onCopy={copyGeneratedCommand}
        onType={() => {
          const cmd = cqResult?.command?.trim();
          if (!cmd) return;
          setCqOpen(false);
          sendToActive(cmd);
        }}
        onClose={() => setCqOpen(false)}
      />
    </div>
  );
};
