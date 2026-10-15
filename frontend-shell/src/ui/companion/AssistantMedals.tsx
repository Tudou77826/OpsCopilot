import React, { useEffect, useId, useRef, useState } from 'react';
import type { AchievementStatus, SocialHost } from '../../companion/socialHost';
import type { MedalDefinition } from '../../companion/types';
import { PixelMedal } from './PixelSprite';

const tiers = { bronze: '铜章', silver: '银章', gold: '金章' };

/** Local progress only. Opening the assistant never opts in or synchronizes. */
export function AssistantMedals({ host, onManage }: { host: SocialHost; onManage(): void }) {
  const [status, setStatus] = useState<AchievementStatus>();
  const [catalog, setCatalog] = useState<MedalDefinition[]>();
  const [error, setError] = useState(false);
  const request = useRef(0), live = useRef(false);
  const tooltipId = useId();
  useEffect(() => {
    live.current = true;
    let definitions: MedalDefinition[] | undefined;
    const refresh = async () => {
      const sequence = ++request.current;
      try {
        const [next, items] = await Promise.all([host.status(), definitions ? Promise.resolve(definitions) : host.catalog()]);
        if (live.current && request.current === sequence) {
          definitions = items; setStatus(next); setCatalog(items); setError(false);
        }
      } catch { if (live.current && request.current === sequence) setError(true); }
    };
    const visible = () => { if (!document.hidden) void refresh(); };
    visible();
    const timer = setInterval(visible, 5000);
    document.addEventListener('visibilitychange', visible);
    return () => { live.current = false; request.current++; clearInterval(timer); document.removeEventListener('visibilitychange', visible); };
  }, [host]);
  const earned = (catalog ?? []).flatMap(item => {
    const progress = status?.summary.progress.find(entry => entry.kind === item.kind);
    return progress?.medal ? [{ item, progress, medal: progress.medal }] : [];
  });
  // Respect an explicit showcase, including empty slots after a medal is revoked.
  // With no selection, preview up to three earned medals without saving a choice.
  const weight = { bronze: 0, silver: 1, gold: 2 };
  const displayed = status?.selected.length
    ? status.selected.flatMap(kind => earned.filter(entry => entry.item.kind === kind)).slice(0, 3)
    : [...earned].sort((a, b) => weight[b.medal.tier] - weight[a.medal.tier]).slice(0, 3);
  return <section className="oc-assistant-medals" aria-label="功能奖章">
    {catalog && status ? displayed.length ? <ul className="oc-assistant-medals-strip">{displayed.map(({item, progress, medal}, index) => {
      const description = `${item.name} · ${tiers[medal.tier]} · ${progress.count}${item.unit}`;
      const id = `${tooltipId}-${index}`;
      return <li key={item.kind} className="oc-assistant-medal" data-kind={item.kind} data-earned="true">
        <button type="button" aria-label={description} aria-describedby={id} onClick={onManage}>
          <PixelMedal kind={item.kind} tier={medal.tier} small/>
        </button>
        <span id={id} role="tooltip" className="oc-assistant-medal-caption">{description}</span>
      </li>;
    })}</ul> : <span className="oc-assistant-medals-status">{earned.length ? '暂未选出可展示的奖章' : '尚未获得奖章'}</span>
      : <span role="status" className="oc-assistant-medals-status">{error ? '奖章暂未读取' : '正在读取…'}</span>}
    <button type="button" className="oc-assistant-medals-more" aria-label="养成与奖章" title="查看全部奖章" onClick={onManage}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d="M7 17 17 7M7 7h10v10"/></svg></button>
    {error && status && <span role="status" className="oc-assistant-medals-stale">奖章暂未更新</span>}
  </section>;
}
