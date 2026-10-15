import React, { useEffect, useRef, useState } from 'react';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';
import type { CompanionAction } from '../../companion/content';
import AchievementsPanel from './AchievementsPanel';
import { PixelSprite } from './PixelSprite';
import { CompanionEgg, HATCH_OPEN_DURATION_MS } from './CompanionEgg';
import { CompanionHatchDialog, CompanionHatchInvitation } from './CompanionHatchDialog';
import { useCompanion } from './useCompanion';
import { companionSpecies, speciesInfo } from '../../companion/species';
import './companion.css';

export function CompanionEnable({ enabled, onChange }: { enabled: boolean; onChange(enabled: boolean): void }) {
  return <section className="oc-companion-enable"><label><input type="checkbox" checked={enabled} onChange={e => onChange(e.target.checked)} />启用伙伴养成与功能奖章</label><p>通过日常实践陪伴伙伴成长，记录自己的功能积累。关闭后暂停采集，保留存档。加入组织排名需要另行确认。</p></section>;
}

export default function CompanionPage({ host, socialHost, preferences }: { host: CompanionHost; socialHost: SocialHost; preferences?: React.ReactNode }) {
  const { snapshot, error: readError, refresh, accept } = useCompanion(host);
  const [name, setName] = useState('');
  const [editRevision, setEditRevision] = useState<number>();
  const [offer, setOffer] = useState<CompanionSnapshot>();
  const [hatchResult, setHatchResult] = useState<CompanionSnapshot>();
  const [busy, setBusy] = useState(false), [opening, setOpening] = useState(false);
  const [hatchReady, setHatchReady] = useState(false);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [action, setAction] = useState<CompanionAction>('idle');
  const previous = useRef(snapshot), pending = useRef(false), mounted = useRef(true);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const revealTimer = useRef<ReturnType<typeof setTimeout>>();
  const resultHeading = useRef<HTMLHeadingElement>(null), focusResult = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clearTimeout(timer.current);
      clearTimeout(revealTimer.current);
    };
  }, []);
  useEffect(() => {
    if (!opening && !offer && focusResult.current) { resultHeading.current?.focus(); focusResult.current = false; }
  }, [opening, offer]);
  useEffect(() => {
    if (snapshot?.pet && previous.current?.pet && snapshot.pet.xp > previous.current.pet.xp) {
      setAction('happy');
      setNotice(snapshot.level > previous.current.level ? `${snapshot.pet.name}升到了 Lv.${snapshot.level}！` : `伙伴获得了 ${snapshot.pet.xp - previous.current.pet.xp} 点成长经验`);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setAction('idle'), 5000);
    }
    previous.current = snapshot;
  }, [snapshot]);
  const pet = snapshot?.pet, level = snapshot?.level ?? 1;
  const levelXP = snapshot?.levelXP ?? 0, required = snapshot?.levelXPRequired ?? 10;

  async function recover(cause: unknown) {
    if (!mounted.current) return;
    setError(String(cause instanceof Error ? cause.message : cause));
    setOffer(undefined);
    setEditRevision(undefined);
    await refresh().catch(() => undefined);
  }
  async function rename() {
    if (editRevision === undefined || pending.current || !name.trim()) return;
    pending.current = true; setBusy(true); setError('');
    try {
      const next = await host.rename(name.trim(), editRevision);
      if (mounted.current) { accept(next); setEditRevision(undefined); setNotice('伙伴名称已保存。'); }
    } catch (cause) { await recover(cause); }
    finally { pending.current = false; if (mounted.current) setBusy(false); }
  }
  async function hatch(quote: CompanionSnapshot) {
    if (pending.current || !quote.canHatch) return;
    setOffer(quote);
    pending.current = true; setBusy(true); setOpening(true); setHatchReady(false); setError(''); setNotice(''); setHatchResult(undefined);
    clearTimeout(timer.current); setAction('idle');
    try {
      const next = await host.hatch(quote.revision);
      if (!mounted.current) return;
      // The backend has already committed. Closing the page during this visual
      // reveal never loses the result or submits another draw on the next visit.
      accept(next);
      setHatchReady(true);
      const reveal = () => {
        if (!mounted.current) return;
        setHatchResult(next);
        setOpening(false); setBusy(false); pending.current = false;
        setNotice(`开出了${speciesInfo(next.pet!.speciesId).name}，${speciesInfo(next.pet!.speciesId).character}。${quote.pet ? `已消耗 ${quote.hatchCost} 点经验。` : ''}`);
      };
      if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) reveal();
      else revealTimer.current = setTimeout(reveal, HATCH_OPEN_DURATION_MS);
    } catch (cause) {
      pending.current = false;
      if (mounted.current) { setOpening(false); setBusy(false); }
      await recover(cause);
    }
  }
  function closeHatch() {
    if (pending.current) return;
    if (hatchResult) focusResult.current = true;
    setOffer(undefined); setHatchResult(undefined);
  }
  async function nameNewCompanion(value: string, revision: number) {
    if (pending.current) return;
    pending.current = true; setBusy(true);
    try {
      const next = await host.rename(value, revision);
      if (mounted.current) {
        accept(next); setNotice(`以后就叫你「${next.pet!.name}」了。`);
        focusResult.current = true; setOffer(undefined); setHatchResult(undefined);
      }
    } finally { pending.current = false; if (mounted.current) setBusy(false); }
  }
  function greet() {
    setAction('greet'); clearTimeout(timer.current);
    timer.current = setTimeout(() => setAction('idle'), 3000);
  }
  function finishInteraction() { clearTimeout(timer.current); setAction('idle'); }
  return <div className="oc-companion-page">
    <header className="oc-companion-page-heading"><h2>伙伴与奖章</h2>{preferences}</header>
    {(error || readError) && <div role="alert" className="oc-companion-error">{error || readError}<button type="button" onClick={() => void refresh().catch(() => {})}>重新读取存档</button></div>}
    {!snapshot ? <p role="status">正在读取伙伴存档…</p> : <section className="oc-companion-home" aria-label="我的伙伴" aria-busy={busy}>
      <div className="oc-companion-scene">
        {opening || !pet ? <CompanionEgg opening={opening} shaking={false}/> : <button type="button" className="oc-companion-touch" aria-label="与伙伴打招呼" onClick={greet}><PixelSprite species={pet.speciesId} palette={pet.paletteId} action={action} name={pet.name} scale={2} animate={!offer} onComplete={finishInteraction}/></button>}
      </div>
      <div className="oc-companion-details">
        {opening ? <div className="oc-companion-hatch-intro"><h3>正在开蛋…</h3><p>即将见到新的伙伴</p></div> : !pet ? <div className="oc-companion-hatch-intro">
          <h3>遇见你的伙伴</h3><p>首次开蛋免费，{companionSpecies.length} 种伙伴等概率出现。形象与默认性格随开蛋确定。</p>
          <div className="oc-companion-actions"><button type="button" disabled={busy || !!readError || !snapshot.canHatch} onClick={() => void hatch(snapshot)}>免费开蛋</button></div>
        </div> : <>
          <div className="oc-companion-overview"><h3 ref={resultHeading} tabIndex={-1}>{pet.name}</h3><span>Lv.{level} · {pet.name !== speciesInfo(pet.speciesId).name && `${speciesInfo(pet.speciesId).name} · `}{speciesInfo(pet.speciesId).character}</span><small title={`初次相遇于 ${new Date(pet.adoptedAt).toLocaleDateString('zh-CN')}`}>今日经验 {snapshot.todayXP ?? 0}/{snapshot.dailyXPMax ?? 12}</small></div>
          <div className="oc-companion-xp"><span>成长经验 <strong>{pet.xp}</strong></span><span>{level < 100 ? `${levelXP} / ${required} · 升至 Lv.${level + 1} 还需 ${Math.max(0, required - levelXP)} 点` : 'Lv.100 · 已满级'}</span></div>
          <progress aria-label="伙伴升级进度" max={level < 100 ? required : 1} value={level < 100 ? levelXP : 1}/>
          {editRevision !== undefined ? <form className="oc-companion-rename" onSubmit={e => { e.preventDefault(); void rename(); }}>
            <label>伙伴名称<input autoFocus value={name} disabled={busy} maxLength={24} onChange={e => setName(e.target.value)} autoComplete="off"/></label>
            <div className="oc-companion-actions"><button type="submit" disabled={busy || !name.trim()}>保存名称</button><button type="button" disabled={busy} onClick={() => setEditRevision(undefined)}>取消</button></div>
          </form> : <div className="oc-companion-actions">
            <button type="button" disabled={busy} onClick={greet}>打个招呼</button>
            <button type="button" disabled={busy} onClick={() => { clearTimeout(timer.current); setAction(action === 'sleep' ? 'idle' : 'sleep'); }}>{action === 'sleep' ? '继续陪伴' : '休息一会'}</button>
            <button type="button" disabled={busy || !!readError} onClick={() => { setName(pet.name); setEditRevision(snapshot.revision); setOffer(undefined); }}>修改名字</button>
            <CompanionHatchInvitation snapshot={snapshot} disabled={busy || !!readError} onOpen={() => { setOffer(snapshot); setHatchResult(undefined); setError(''); }}/>
          </div>}
        </>}
        <details className="oc-companion-rules"><summary>开蛋与成长规则</summary><p>首次免费开蛋，重新开蛋会消耗成长经验，费用在确认前展示，等概率获得当前之外的伙伴。等级由剩余成长经验计算：每 10 点一级，最高 Lv.100；形象和体型不随等级改变。成功建立连接 +2，完成文件传输 +3，脚本全部命令完成派发 +4。每类每天最多计两次，每日总上限 12，以 UTC 日期结算。派发完成不代表远端执行成功。未开蛋时不累积经验；满级后继续积累经验，奖章统计独立。</p></details>
        <p className="oc-companion-notice" role="status" aria-live="polite">{opening ? '正在开蛋…' : notice}</p>
      </div>
    </section>}
    {offer && <CompanionHatchDialog quote={offer} opening={opening} ready={hatchReady} result={hatchResult} onConfirm={() => void hatch(offer)} onClose={closeHatch} onName={nameNewCompanion}/>}
    <AchievementsPanel host={socialHost}/>
  </div>;
}
