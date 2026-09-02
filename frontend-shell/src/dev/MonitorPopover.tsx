/**
 * MonitorPopover：资源监控弹层（S8 监控 v1）。
 * 采样 = 远端只读命令（loadavg/meminfo/df），打开时每 5 秒轮询，关闭即停。
 */
import React, { useEffect, useState } from 'react';
import { MonitorSample, SidecarClient } from '../core';

interface MonitorPopoverProps {
  client: SidecarClient;
  connectionId: string | null;
  hostLabel: string;
  onClose: () => void;
}

export const MonitorPopover: React.FC<MonitorPopoverProps> = ({ client, connectionId, hostLabel, onClose }) => {
  const [sample, setSample] = useState<MonitorSample | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!connectionId) return;
    let cancelled = false;
    const poll = async () => {
      try {
        const s = await client.monitorSample(connectionId);
        if (!cancelled) { setSample(s); setError(''); }
      } catch (err) {
        if (!cancelled) setError((err as Error).message);
      }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 5000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [client, connectionId]);

  const memPct = Math.round(sample?.memUsedPct ?? 0);

  return (
    <div className="monitor-pop" data-testid="monitor-pop">
      <div className="drawer-head">
        <span className="drawer-label">资源 · {hostLabel}</span>
        <button className="modal-close" aria-label="关闭监控" onClick={onClose}>×</button>
      </div>
      {!connectionId && <div className="drawer-empty">先打开一个终端</div>}
      {error && <div className="drawer-empty">{error}</div>}
      {sample && (
        <div className="monitor-grid">
          <div className="monitor-item">
            <span className="monitor-key">负载 (1m)</span>
            <span className="monitor-val mono">{sample.load1}</span>
          </div>
          <div className="monitor-item">
            <span className="monitor-key">内存</span>
            <span className="monitor-val mono">{memPct}% <small>{sample.memUsedMB.toFixed(0)}/{sample.memTotalMB.toFixed(0)} MB</small></span>
            <div className="progress"><div className="progress-fill" style={{ width: `${memPct}%` }} /></div>
          </div>
          <div className="monitor-item">
            <span className="monitor-key">磁盘 {sample.diskPath}</span>
            <span className="monitor-val mono">{sample.diskUsedPct}%</span>
            <div className="progress"><div className="progress-fill" style={{ width: `${sample.diskUsedPct}%` }} /></div>
          </div>
          <div className="monitor-note">每 5 秒采样 · 只读命令</div>
        </div>
      )}
    </div>
  );
};
