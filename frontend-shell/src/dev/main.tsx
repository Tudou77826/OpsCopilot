import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { App, SidecarEndpoint } from './App';
import type { HostCapabilities } from '../ui';
import { ToastProvider, ConfirmDialogInternal, readPersistedTheme } from '../ui';
import '../styles.css';

// 共享产品组件（SessionManager/连接表单/快捷命令等）依赖 Toast 与确认框，
// 装配方式与 Wails 入口（frontend/src/main.tsx）保持一致。
// 主题在渲染前写入 <html data-theme>，与产品防 FOUC 机制一致。
document.documentElement.setAttribute('data-theme', readPersistedTheme());

interface BootstrapResponse extends SidecarEndpoint {
  rpcPath?: string;
  capabilities?: Partial<HostCapabilities>;
}

const BootstrapApp: React.FC = () => {
  const bootstrapPath = new URLSearchParams(window.location.search).get('bootstrap');
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!bootstrapPath) return;
    let disposed = false;
    void fetch(bootstrapPath, { cache: 'no-store' })
      .then(async (response) => {
        const body = await response.json() as BootstrapResponse & { error?: string };
        if (!response.ok) throw new Error(body.error ?? `HTTP ${response.status}`);
        const rpcUrl = body.rpcUrl ?? (body.rpcPath
          ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}${body.rpcPath}`
          : '');
        if (!rpcUrl || !body.wsBase || !body.token) throw new Error('宿主返回的 sidecar 端点不完整');
        if (!disposed) setBootstrap({ ...body, rpcUrl });
      })
      .catch((cause) => {
        if (!disposed) setError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => { disposed = true; };
  }, [bootstrapPath]);

  if (!bootstrapPath) return <App />;
  if (error) return <div className="shell-root"><div className="sidecar-gate"><div className="error-banner">OpsCopilot Shell 启动失败：{error}</div></div></div>;
  if (!bootstrap) return <div className="shell-root"><div className="sidecar-gate"><div className="muted">正在启动 OpsCopilot Shell…</div></div></div>;
  return <App endpoint={bootstrap} capabilities={bootstrap.capabilities} autoConnect />;
};

createRoot(document.getElementById('root')!).render(
  <ToastProvider>
    <BootstrapApp />
    <ConfirmDialogInternal />
  </ToastProvider>
);
