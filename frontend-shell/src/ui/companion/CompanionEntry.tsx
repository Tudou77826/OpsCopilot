import React, { useEffect, useId, useRef, useState } from 'react';
import { TbMessageChatbot } from 'react-icons/tb';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';
import { packForSpecies } from '../../companion/assets';
import { CompanionEgg } from './CompanionEgg';
import { CompanionAvatar } from './CompanionAvatar';
import { useCompanion } from './useCompanion';
import { AssistantMedals } from './AssistantMedals';
import { SceneCompanion } from './SceneCompanion';
import { visibleFrameHeightRatio, type CompanionAction } from '../../companion/content';

/** Personalization reads are mounted only after the user enables collection. */
function PersonalIdentity({ host, children }: { host: CompanionHost; children(snapshot?: CompanionSnapshot): React.ReactNode }) {
  const { snapshot } = useCompanion(host);
  return <>{children(snapshot)}</>;
}

export function CompanionEntry({ host, enabled = false, selected, onOpen, label = '伙伴助手' }: {
  host?: CompanionHost; enabled?: boolean; selected: boolean; onOpen(): void; label?: string;
}) {
  const content = (snapshot?: CompanionSnapshot) => enabled && !snapshot?.pet ? <CompanionEgg/> : <CompanionAvatar species={snapshot?.pet?.speciesId} level={snapshot?.level} palette={snapshot?.pet?.paletteId} name={snapshot?.pet?.name ?? '伙伴助手'}/>;
  const entryLabel = enabled ? label : 'AI 助手';
  return <button type="button" className="oc-companion-entry" title={entryLabel} aria-label={entryLabel} aria-pressed={selected} data-state={enabled ? 'active' : 'quiet'} data-testid="nav-icon-companion" onClick={onOpen}>
    {enabled ? (host ? <PersonalIdentity host={host}>{content}</PersonalIdentity> : content()) : <TbMessageChatbot aria-hidden="true"/>}
  </button>;
}

export function CompanionAssistantHeader({ host, socialHost, enabled, visible, onManage }: {
  host: CompanionHost; socialHost?: SocialHost; enabled: boolean; visible: boolean; onManage(): void;
}) {
  const [expanded, setExpanded] = useState(true);
  const [action, setAction] = useState<CompanionAction>('idle');
  const timer = useRef<ReturnType<typeof setTimeout>>(), touches = useRef(0);
  useEffect(() => { if (!enabled || !visible || !expanded) { if (timer.current) clearTimeout(timer.current); setAction('idle'); } return () => { if (timer.current) clearTimeout(timer.current); }; }, [enabled, visible, expanded]);
  const interact = () => { touches.current++; setAction(touches.current % 2 ? 'greet' : 'happy'); if (timer.current) clearTimeout(timer.current); timer.current = setTimeout(() => setAction('idle'), 6000); };
  const finishInteraction = () => { if (timer.current) clearTimeout(timer.current); setAction('idle'); };
  const bodyId = useId();
  if (!enabled) return null;
  const content = (snapshot?: CompanionSnapshot) => {
    const pet = snapshot?.pet, stage = 'child' as const;
    const fit=visibleFrameHeightRatio(packForSpecies(pet?.speciesId),stage);
    const figureSize=`min(320px, calc((var(--oc-companion-scene-height) - 16px) / ${fit}))`;
    const level = snapshot?.level ?? 1, required = snapshot?.levelXPRequired ?? 10;
    return <section className="oc-companion-assistant-header" data-expanded={expanded} aria-label="伙伴展示">
      <div className="oc-assistant-companion-title">
        {!expanded && (enabled && !pet ? <CompanionEgg/> : <CompanionAvatar species={pet?.speciesId} level={pet?level:undefined} palette={pet?.paletteId} name={pet?.name ?? '伙伴助手'}/>)}
        <div className="oc-assistant-companion-identity">
          <strong title={pet?.name}>{pet?.name ?? (enabled ? '等待相遇' : '运维伙伴')}</strong>
          <small>{pet ? `Lv.${level}` : enabled ? '尚未开蛋' : '未开启养成'}</small>
          {pet && <progress aria-label="伙伴升级进度" title={level<100 ? `${snapshot?.levelXP??0} / ${required} 经验 · 升至 Lv.${level+1}` : `Lv.100 · 已满级 · 成长经验 ${pet.xp} 经验`} value={level<100 ? snapshot?.levelXP??0 : 1} max={level<100 ? required : 1}/>}
        </div>
        <button type="button" className="oc-assistant-companion-fold" title={expanded ? '收起伙伴展示' : '展开伙伴展示'} aria-label={expanded ? '收起伙伴展示' : '展开伙伴展示'} aria-expanded={expanded} aria-controls={bodyId} onClick={() => setExpanded(value => !value)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d={expanded ? 'm6 15 6-6 6 6' : 'm6 9 6 6 6-6'}/></svg></button>
      </div>
      {expanded && <div id={bodyId} className="oc-assistant-companion-body">
        <div className="oc-assistant-companion-scene">
          {enabled && !pet ? <CompanionEgg/> : <SceneCompanion species={pet?.speciesId} stage={stage} palette={pet?.paletteId} name={pet?.name ?? '微光'} action={action} size={figureSize} visible={visible} onInteract={interact} onComplete={finishInteraction}/>}
          {enabled && pet && socialHost ? visible && <AssistantMedals host={socialHost} onManage={onManage}/> : <div className="oc-assistant-companion-start"><button type="button" onClick={onManage}>{enabled ? (pet ? '养成与奖章' : '去开蛋') : '开启养成'}</button></div>}
        </div>
      </div>}
    </section>;
  };
  return enabled && visible ? <PersonalIdentity host={host}>{content}</PersonalIdentity> : content();
}
