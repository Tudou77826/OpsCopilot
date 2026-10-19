import React, { useEffect, useRef, useState } from 'react';
import type { AchievementStatus, SocialHost } from '../../companion/socialHost';
import type { MedalDefinition, MedalKind, RankingPage, PublicCompanionProfile } from '../../companion/types';
import { btnSmall, inputStyle } from '../settings/settingsStyles';
import './achievements.css';
import { PixelMedal, PixelSprite } from './PixelSprite';

const labels = { bronze: '铜章', silver: '银章', gold: '金章' };
const facts: Record<MedalKind, string> = {
  'knowledge-archive': '含有效结论的独立归档记录', 'script-created': '本人保存的有效脚本',
  'quick-command-created': '本人保存的非空快捷指令', 'case-completed': '已保存根因与结论的排查记录',
  'knowledge-read': '成功查阅正文的不同知识文件', 'script-used': '完成全部派发的不同脚本',
  'quick-command-used': '成功派发的不同快捷指令', 'active-day': '发生有效业务操作的累计日期（UTC）',
};

export default function AchievementsPanel({ host, organizationEnabled = false }: { host: SocialHost; organizationEnabled?: boolean }) {
  const [status, setStatus] = useState<AchievementStatus>();
  const [catalog, setCatalog] = useState<MedalDefinition[]>([]);
  const [selected, setSelected] = useState<MedalKind[]>([]);
  const [baseUrl, setBaseUrl] = useState(''), [credential, setCredential] = useState(''), [claim, setClaim] = useState(false);
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  const [kind, setKind] = useState<MedalKind>('knowledge-archive');
  const [ranking, setRanking] = useState<RankingPage>(), [profile, setProfile] = useState<PublicCompanionProfile>();
  const rankRequest = useRef(0), live = useRef(true);
  const dirty = useRef(false), saving = useRef(false), pending = useRef<MedalKind[]>();
  const selectedRef = useRef<MedalKind[]>([]);
  const readSequence = useRef(0);
  const [saveNotice, setSaveNotice] = useState('');
  const [organizationError, setOrganizationError] = useState('');
  const accept = (next: AchievementStatus) => { if(!live.current)return; setStatus(next); if (!dirty.current) { selectedRef.current = next.selected ?? []; setSelected(selectedRef.current); } if(next.binding)setBaseUrl(next.binding.baseUrl); };
  const action = async (run: () => Promise<void>) => {
    setBusy(true); setError(''); try { await run(); } catch(e) { if(live.current)setError(String(e instanceof Error ? e.message : e)); }
    finally { if(live.current)setBusy(false); }
  };
  const load = async () => { const request = ++readSequence.current; const [next, definitions] = await Promise.all([host.status(), host.catalog()]); if(live.current && request === readSequence.current) {accept(next);setCatalog(definitions);} };
  useEffect(() => {
    live.current=true; void action(load);
    const refresh = () => { if (!document.hidden) void load().catch(() => {}); };
    const timer = setInterval(refresh, 5000);
    document.addEventListener('visibilitychange', refresh);
    return () => {live.current=false;rankRequest.current++;readSequence.current++;clearInterval(timer);document.removeEventListener('visibilitychange', refresh);};
  }, [host]);
  const ranks = async (offset = 0, snapshotId?: string) => {
    const request = ++rankRequest.current;
    try { const next = await host.rankings({kind,offset,limit:20,snapshotId}); if(live.current&&request===rankRequest.current)setRanking(next); }
    catch(e){ if(live.current&&request===rankRequest.current){setRanking(undefined);setOrganizationError(String(e instanceof Error?e.message:e));} }
  };
  useEffect(() => { setRanking(undefined);setProfile(undefined);rankRequest.current++;if(organizationEnabled && status?.binding)void ranks(); }, [organizationEnabled,kind,status?.binding?.member.userId]);
  async function saveSelection() {
    if (saving.current) return;
    saving.current = true;
    try {
      // Serialize user choices so an older disk write cannot replace the latest selection.
      while (pending.current) {
        const choice = pending.current; pending.current = undefined;
        try {
          const next = await host.showcase(choice);
          readSequence.current++;
          if (live.current) {
            setStatus(next); setError('');
            if (!pending.current) {
              dirty.current = false; selectedRef.current = next.selected ?? choice;
              setSelected(selectedRef.current); setSaveNotice('已保存');
            }
          }
        } catch (cause) {
          pending.current = pending.current ?? choice;
          if (live.current) {setError(`展示选择未保存：${cause instanceof Error ? cause.message : String(cause)}`);setSaveNotice('');}
          break;
        }
      }
    } finally { saving.current = false; }
  }
  const toggle = (value: MedalKind) => {
    const current = selectedRef.current;
    if (!current.includes(value) && current.length >= 3) return;
    const next = current.includes(value) ? current.filter(item => item !== value) : [...current, value];
    selectedRef.current = next; dirty.current = true; pending.current = next;
    readSequence.current++;
    setSelected(next); setSaveNotice('正在保存…'); void saveSelection();
  };
  const definition = catalog.find(v=>v.kind===kind);
  const progress = status?.summary.progress ?? [];
  const pendingVisibility = status?.participationPending !== undefined;

  return <div className="oc-achievements" aria-busy={busy}>
    {error && <div role="alert" className="oc-achievements-error">{error}<button type="button" style={btnSmall} onClick={()=>void (dirty.current ? saveSelection() : action(load))}>重试</button></div>}
    <section className="oc-achievements-card">
      <div className="oc-achievements-heading"><h3>功能奖章 <small>展示 {selected.length}/3</small></h3>{saveNotice && <small className="oc-achievements-save-state" role="status">{saveNotice}</small>}</div>
      <div className="oc-achievements-grid">{catalog.map(item=>{
        const entry=progress.find(v=>v.kind===item.kind), medal=entry?.medal;
        return <button type="button" key={item.kind} className="oc-achievements-medal" title={`${facts[item.kind]}；${entry?.nextThreshold ? `下一级 ${entry.nextThreshold} ${item.unit}` : '已达到最高等级'}`} aria-pressed={selected.includes(item.kind)} disabled={busy || !status || (!medal && !selected.includes(item.kind)) || (!selected.includes(item.kind)&&selected.length===3)} onClick={()=>toggle(item.kind)}>
          <PixelMedal kind={item.kind} tier={medal?.tier??'locked'} small/>
          <span><strong>{item.name}</strong><span className="oc-achievements-quantity">{entry?.count??0} <small>{item.unit}</small> · {medal?labels[medal.tier]:'尚未获章'}</span></span>
        </button>;
      })}</div>
      <details className="oc-achievements-rules"><summary>计数说明</summary><p>选择最多三枚奖章展示。派发成功不代表远端执行成功；导入与旧资料不会自动归为本人制作。</p><dl>{catalog.map(item=>{const next=progress.find(entry=>entry.kind===item.kind)?.nextThreshold;return <div key={item.kind}><dt>{item.name}</dt><dd><span>{facts[item.kind]}</span><span className="oc-achievements-caption">{next?`下一级 ${next} ${item.unit}`:'已达到最高等级'}</span></dd></div>;})}</dl></details>
    </section>
    {organizationEnabled && <section className="oc-achievements-card oc-achievements-social">
      {organizationError && <p role="alert">{organizationError}</p>}
      <div className="oc-achievements-heading"><h3>组织内查看与排名</h3>{status?.binding && <details><summary>重新连接成员凭据</summary><div className="oc-achievements-form">
          <p>服务地址：{status.binding.baseUrl}。这份存档继续使用原成员账号。</p>
          <label>新的成员凭据<input style={inputStyle} type="password" autoComplete="off" value={credential} onChange={e=>setCredential(e.target.value)}/></label>
          <button type="button" style={btnSmall} disabled={busy||!credential} onClick={()=>void action(async()=>{try{accept(await host.connect(status.binding!.baseUrl,credential,true));}finally{setCredential('');}})}>重新连接</button>
        </div></details>}</div>
      <p className="oc-achievements-disclosure">参与后，同一组织的成员可以查看你的八项累计统计和精选奖章。退出保留个人进度。</p>
      {!status?.binding ? <div className="oc-achievements-form">
        <label>排名服务地址<input style={inputStyle} value={baseUrl} onChange={e=>setBaseUrl(e.target.value)} placeholder="https://ops.example.com" /></label>
        <label>成员凭据<input style={inputStyle} type="password" autoComplete="off" value={credential} onChange={e=>setCredential(e.target.value)} /></label>
        <label><input type="checkbox" checked={claim} onChange={e=>setClaim(e.target.checked)} />将这份本地使用记录归属到我的成员账号</label>
        <button type="button" style={btnSmall} disabled={busy||!baseUrl||!credential} onClick={()=>void action(async()=>{try{accept(await host.connect(baseUrl,credential,claim));}finally{setCredential('');}})}>连接成员账号</button>
      </div> : <>
        <div className="oc-achievements-account"><span><strong>{status.binding.member.displayName}</strong> · {pendingVisibility?'参与选择待服务确认':status.binding.member.participating?'已参加组织排名':'尚未参加排名'} · 待同步 {status.pending} 项{status.profilePending?'，展示选择待同步':''}</span>
        <div className="oc-achievements-actions"><button type="button" style={btnSmall} disabled={busy} onClick={()=>void action(async()=>{try{await host.participation(!status.binding!.member.participating);}finally{accept(await host.status());setProfile(undefined);setRanking(undefined);}await ranks();})}>{status.binding.member.participating?'退出排名':'同意公开八项统计并参与'}</button>
          <button type="button" style={btnSmall} disabled={busy} onClick={()=>void action(async()=>{accept(await host.sync());await ranks();})}>立即同步</button>
          <button type="button" style={btnSmall} disabled={busy} onClick={()=>void action(async()=>{setProfile(await host.profile(status.binding!.member.userId));})}>查看我的资料</button></div></div>
        {pendingVisibility && <p role="status">当前参与选择尚未同步。离线退出时，公开资料可能仍可见，请恢复网络后同步确认。</p>}

      </>}
    {status?.binding && <div className="oc-achievements-ranking">
      <div className="oc-achievements-actions"><label>分类榜 <select style={inputStyle} value={kind} disabled={busy} onChange={e=>setKind(e.target.value as MedalKind)}>{catalog.map(item=><option key={item.kind} value={item.kind}>{item.name}</option>)}</select></label><button type="button" style={btnSmall} disabled={busy} onClick={()=>void action(()=>ranks())}>刷新排名</button></div>
      {ranking && <><div className="oc-achievements-table-wrap"><table className="oc-achievements-table"><caption className="oc-achievements-table-caption">{definition?.name} · 累计榜</caption><thead><tr><th>名次</th><th>成员</th><th>累计数量</th><th>精选奖章</th></tr></thead><tbody>{ranking.entries.map(entry=><tr key={entry.profile.userId}><td>{entry.rank}</td><td><span className="oc-companion-public-card">{entry.profile.companion && <PixelSprite species={entry.profile.companion.speciesId} palette={entry.profile.companion.paletteId} name={entry.profile.companion.name} scale={1} animate={false}/>}<button type="button" className="oc-achievements-link" disabled={busy} onClick={()=>void action(async()=>setProfile(await host.profile(entry.profile.userId)))}>{entry.profile.displayName}</button></span></td><td>{entry.count} {definition?.unit}</td><td><span className="oc-companion-public-medals oc-achievements-ranking-medals">{entry.profile.medals.map(m=><span key={m.kind} role="img" aria-label={`${catalog.find(c=>c.kind===m.kind)?.name??m.kind} ${labels[m.tier]}`} title={`${catalog.find(c=>c.kind===m.kind)?.name??m.kind} ${labels[m.tier]}`}><PixelMedal kind={m.kind} tier={m.tier} small/></span>)}</span>{!entry.profile.medals.length && '未选择'}</td></tr>)}</tbody></table></div>
        {ranking.total===0 && <p>这个分类还没有参与成员的有效记录。</p>}
        <div className="oc-achievements-actions oc-achievements-pagination"><button type="button" style={btnSmall} disabled={busy||ranking.offset===0} onClick={()=>void action(()=>ranks(Math.max(0,ranking.offset-20),ranking.snapshotId))}>上一页</button><span>{ranking.total?`${ranking.offset+1}–${Math.min(ranking.offset+20,ranking.total)} / ${ranking.total}`:'0 位成员'}</span><button type="button" style={btnSmall} disabled={busy||ranking.offset+20>=ranking.total} onClick={()=>void action(()=>ranks(ranking.offset+20,ranking.snapshotId))}>下一页</button></div>
      </>}
    </div>}
    </section>}
    {organizationEnabled && profile && <section className="oc-achievements-card oc-achievements-member" aria-label="成员资料"><div className="oc-achievements-heading"><h3>{profile.displayName}</h3><button type="button" style={btnSmall} onClick={()=>setProfile(undefined)}>收起资料</button></div>
      {profile.companion ? <><div className="oc-companion-public-scene"><PixelSprite species={profile.companion.speciesId} palette={profile.companion.paletteId} name={profile.companion.name} action="greet" scale={2}/></div><p>伙伴：{profile.companion.name} · {profile.companion.level ? `Lv.${profile.companion.level}` : '等级待更新'}</p></> : <p>尚未领养伙伴</p>}
      <div className="oc-companion-public-medals">{profile.medals.map(m=><span key={m.kind}><PixelMedal kind={m.kind} tier={m.tier} small/><span>{catalog.find(c=>c.kind===m.kind)?.name??m.kind} · {labels[m.tier]}</span></span>)}{!profile.medals.length && '尚未展示奖章'}</div>
      <dl className="oc-achievements-profile">{profile.achievements.progress.map(item=><div key={item.kind}><dt>{catalog.find(c=>c.kind===item.kind)?.name}</dt><dd>{item.count} {catalog.find(c=>c.kind===item.kind)?.unit}{item.nextThreshold && <small className="oc-achievements-caption">下一个目标：{item.nextThreshold} {catalog.find(c=>c.kind===item.kind)?.unit}</small>}</dd></div>)}</dl>
    </section>}
  </div>;
}
