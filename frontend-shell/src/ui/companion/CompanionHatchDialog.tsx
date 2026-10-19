import React, { useEffect, useId, useRef, useState } from 'react';
import type { CompanionSnapshot } from '../../companion/types';
import { speciesInfo } from '../../companion/species';
import { CompanionEgg, HATCH_OPEN_DURATION_MS } from './CompanionEgg';
import { PixelSprite } from './PixelSprite';

export function CompanionHatchInvitation({ snapshot, disabled, onOpen }: {
  snapshot: CompanionSnapshot; disabled: boolean; onOpen(): void;
}) {
  const remaining = Math.max(0, snapshot.hatchCost - (snapshot.pet?.xp ?? 0));
  const tooltipId = useId();
  const description = `重新开蛋：消耗 ${snapshot.hatchCost} 点经验，随机获得另一种伙伴。${remaining ? `当前 ${snapshot.pet?.xp ?? 0} 点，还差 ${remaining} 点。` : '名字、奖章和功能统计保留。'}`;
  return <span className="oc-hatch-entry">
    <button type="button" className="oc-hatch-invitation" aria-label="重新开蛋" aria-describedby={tooltipId} aria-haspopup="dialog" disabled={disabled} onClick={onOpen}>
      <span aria-hidden="true"><CompanionEgg compact/></span>
    </button>
    <span id={tooltipId} role="tooltip" className="oc-hatch-entry-tooltip">{description}</span>
  </span>;
}

/** The native modal isolates the background; Tab boundaries also stay inside
 * the experience on desktop and in a Teams shadow-root settings surface. */
export function CompanionHatchDialog({ quote, opening, ready, result, onConfirm, onClose, onName }: {
  quote: CompanionSnapshot; opening: boolean; ready: boolean; result?: CompanionSnapshot;
  onConfirm(): void; onClose(): void; onName(name: string, revision: number): Promise<void>;
}) {
  const dialog = useRef<HTMLDialogElement>(null), cancel = useRef<HTMLButtonElement>(null);
  const nameInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState(''), [saving, setSaving] = useState(false), [error, setError] = useState('');
  const submitting = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null), descriptionId = useId(), nameId = useId();
  const pet = result?.pet, info = pet ? speciesInfo(pet.speciesId) : undefined;
  const firstMeeting = !quote.pet;
  const remaining = Math.max(0, quote.hatchCost - (quote.pet?.xp ?? 0));
  useEffect(() => {
    const element = dialog.current!;
    const surface = element.getRootNode() as Document | ShadowRoot;
    const previous = surface.activeElement instanceof HTMLElement ? surface.activeElement : undefined;
    element.showModal();
    cancel.current?.focus();
    return () => { element.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  useEffect(() => {
    if (pet) setName(pet.name);
  }, [pet]);
  useEffect(() => {
    if (pet && !saving) nameInput.current?.focus();
    else if (opening || pet) heading.current?.focus();
  }, [opening, pet, saving]);
  const dismiss = () => { if (!opening && !submitting.current) onClose(); };
  async function saveName(event: React.FormEvent) {
    event.preventDefault();
    if (!result || !name.trim() || submitting.current) return;
    submitting.current = true; setSaving(true); setError('');
    try { await onName(name.trim(), result.revision); }
    catch (cause) { setError(String(cause instanceof Error ? cause.message : cause)); }
    finally { submitting.current = false; setSaving(false); }
  }
  return <dialog ref={dialog} className="oc-hatch-dialog" aria-modal="true" aria-label={firstMeeting ? '初次遇见伙伴' : '确认重新开蛋'} aria-describedby={descriptionId}
    onCancel={event => { event.preventDefault(); dismiss(); }} onKeyDown={event => {
      // Settings has window shortcuts of its own. They must not run behind this modal.
      event.stopPropagation();
      if (event.key === 'Escape') { event.preventDefault(); dismiss(); }
      if (event.key === 'Tab') {
        const buttons = [...dialog.current!.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])')];
        const surface = dialog.current!.getRootNode() as Document | ShadowRoot;
        const boundary = event.shiftKey ? buttons[0] : buttons[buttons.length - 1];
        if (!buttons.length || surface.activeElement === boundary) {
          event.preventDefault();
          (event.shiftKey ? buttons[buttons.length - 1] : buttons[0])?.focus();
        }
      }
    }}>
    <button type="button" className="oc-hatch-close" aria-label="关闭开蛋仪式" title={pet ? `放弃修改，保留名字「${pet.name}」` : '关闭开蛋仪式'} disabled={opening || saving} onClick={dismiss}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="m6 6 12 12M6 18 18 6"/></svg></button>
    <div className="oc-hatch-visual" data-result={!!pet} data-phase={pet ? 'revealed' : opening ? ready ? 'cracking' : 'waiting' : 'offer'} style={{'--oc-hatch-duration': `${HATCH_OPEN_DURATION_MS}ms`} as React.CSSProperties}>
      <span className="oc-hatch-halo"/>
      <span className="oc-hatch-orbit"/>
      <span className="oc-hatch-ripple"/>
      <span className="oc-hatch-flash" aria-hidden="true"/>
      <span className="oc-hatch-sparks">{[[-78,-45],[4,-76],[77,-39],[88,24],[44,70],[-12,74],[-73,49],[-92,3]].map(([x,y],index) => <i key={index} style={{'--spark-x': `${x}px`, '--spark-y': `${y}px`, '--spark-delay': `${index % 3 * 55}ms`} as React.CSSProperties}/>)}</span>
      {pet ? <PixelSprite species={pet.speciesId} palette={pet.paletteId} name={pet.name} scale={3}/> : firstMeeting && !opening ? <button ref={cancel} type="button" className="oc-hatch-trigger" aria-label="开启伙伴蛋" onClick={onConfirm}><CompanionEgg/></button> : <span className="oc-hatch-egg-frame" aria-hidden="true"><CompanionEgg opening={opening} shaking={ready}/></span>}
    </div>
    <div className="oc-hatch-dialog-content">
      <span className="oc-hatch-caption">{pet ? firstMeeting ? '初次相遇' : '新的伙伴' : opening ? '正在相遇' : firstMeeting ? '初次相遇' : '重新开蛋'}</span>
      <h2 ref={heading} tabIndex={-1}>{pet ? `遇见了${info!.name}` : opening ? '有什么，正在悄悄醒来' : firstMeeting ? '一枚蛋，一次相遇' : '另一次相遇'}</h2>
      <p id={descriptionId}>{pet ? `${info!.type} · ${info!.character}` : opening ? '让这份小小的惊喜，慢慢靠近。' : firstMeeting ? '轻触这枚蛋，和里面的小家伙打个招呼。' : '蛋里会是另一种伙伴。开启后，新的形象将替换当前形象。'}</p>
      {pet ? !firstMeeting && <p className="oc-hatch-settlement">已消耗 {quote.hatchCost} 点经验 · 剩余 {pet.xp} 点 · Lv.{result!.level}</p> : !opening && !firstMeeting && <>
        <div className="oc-hatch-cost"><span>本次相遇</span><strong>{quote.hatchCost}<small>点经验</small></strong></div>
        <p className="oc-hatch-balance">{remaining ? `当前 ${quote.pet!.xp} 点，还差 ${remaining} 点经验` : `成长经验 ${quote.pet!.xp} → ${quote.pet!.xp - quote.hatchCost} 点，等级按剩余经验重新计算`}</p>
        <p className="oc-hatch-keeps">名字、奖章和功能统计保留。</p>
      </>}
    </div>
    <footer className="oc-hatch-dialog-footer">
      {pet ? <form className="oc-hatch-naming" aria-busy={saving} onSubmit={event => void saveName(event)}>
        <label htmlFor={nameId}>给伙伴起个名字</label>
        <div className="oc-hatch-name-row">
          <input id={nameId} ref={nameInput} value={name} onChange={event => setName(event.target.value)} maxLength={24} placeholder="输入伙伴名字" disabled={saving} autoComplete="off"/>
          <div className="oc-hatch-name-actions">
            <button type="submit" className="oc-hatch-name-icon" aria-label="就叫这个名字" title={saving ? '正在保存…' : '确认这个名字'} disabled={saving || !name.trim()}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg></button>
          </div>
        </div>
        <p>{saving ? '正在保存名字…' : '1–24 个字，以后也可以修改。'}</p>
        {error && <p role="alert">{error}</p>}
      </form> : opening ? <p className="oc-hatch-progress" data-ready={ready} role="status">正在开启，请稍候…</p> : firstMeeting ? <p className="oc-hatch-touch-hint">点击伙伴蛋，开始相遇</p> : <>
        <button type="button" className="oc-hatch-primary" disabled={!quote.canHatch} aria-label={`确认消耗 ${quote.hatchCost} 点并开蛋`} onClick={onConfirm}>{remaining ? `还需 ${remaining} 点经验` : `消耗 ${quote.hatchCost} 点经验，开启新蛋`}</button>
        <button ref={cancel} type="button" className="oc-hatch-cancel" onClick={onClose}>保留当前伙伴</button>
      </>}
    </footer>
  </dialog>;
}
