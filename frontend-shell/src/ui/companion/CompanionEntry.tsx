import React, { useEffect, useId, useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { TbMessageChatbot } from 'react-icons/tb';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';
import { packForSpecies } from '../../companion/assets';
import { CompanionEgg } from './CompanionEgg';
import { CompanionAvatar } from './CompanionAvatar';
import { useCompanion } from './useCompanion';
import { AssistantMedals } from './AssistantMedals';
import { SceneCompanion } from './SceneCompanion';
import { companionDisplayScale, visibleFrameHeightRatio, visibleFrameWidthRatio, type CompanionAction } from '../../companion/content';
import { useCompanionPresentation } from './useCompanionPresentation';

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

export function CompanionAssistantHeader({ host, socialHost, enabled, visible, onManage, actions }: {
  host: CompanionHost; socialHost?: SocialHost; enabled: boolean; visible: boolean; onManage(): void; actions?: ReactNode;
}) {
  const presentation = useCompanionPresentation(enabled, visible);
  const { expanded } = presentation;
  const [action, setAction] = useState<CompanionAction>('idle');
  const timer = useRef<ReturnType<typeof setTimeout>>(), touches = useRef(0);
  useEffect(() => { if (!enabled || !visible || !expanded) { if (timer.current) clearTimeout(timer.current); setAction('idle'); } return () => { if (timer.current) clearTimeout(timer.current); }; }, [enabled, visible, expanded]);
  const interact = () => { touches.current++; setAction(touches.current % 2 ? 'greet' : 'happy'); if (timer.current) clearTimeout(timer.current); timer.current = setTimeout(() => setAction('idle'), 6000); };
  const finishInteraction = () => { if (timer.current) clearTimeout(timer.current); setAction('idle'); };
  const bodyId = useId();
  const progressId = useId();
  if (!enabled || !visible) return <section ref={presentation.root} className="oc-companion-assistant-header" aria-label="对话操作区"><header className="oc-assistant-companion-toolbar">{actions}</header></section>;
  const content = (snapshot?: CompanionSnapshot) => {
    const pet = snapshot?.pet, stage = 'child' as const;
    const pack = packForSpecies(pet?.speciesId);
    const idle = pack.stages[stage].idle;
    const frame = pack.assets[idle.frames[0]];
    const fit = visibleFrameHeightRatio(pack, stage);
    const scale = companionDisplayScale(pack, stage);
    const bounds = pack.contractVersion === 2 ? pack.sceneBounds : undefined;
    // The stage owns its complete height. The medal dock occupies a separate
    // grid column, so a short scene never pays for an empty row above the pet.
    const figureSize = `max(0px, min(calc((100cqh - 16px) / ${fit}), calc((100cqw - 24px) / ${scale * visibleFrameWidthRatio(pack, stage)})))`;
    const level = snapshot?.level ?? 1, required = snapshot?.levelXPRequired ?? 10;
    const progressText = level<100 ? `${snapshot?.levelXP??0} / ${required} 经验 · 升至 Lv.${level+1}` : `Lv.100 · 已满级 · 成长经验 ${pet?.xp??0} 经验`;
    return <section ref={presentation.root} className="oc-companion-assistant-header" data-expanded={expanded} data-resizing={presentation.dragging} aria-label="伙伴展示" style={{
      '--oc-companion-scene-height': `${presentation.height}px`,
      '--oc-companion-figure-size': figureSize,
      '--oc-companion-foot-offset': (frame.height - idle.anchor.y - ((bounds?.bottom ?? frame.height) - idle.anchor.y) * scale) / frame.height,
    } as CSSProperties}>
      <header className="oc-assistant-companion-toolbar">
        <div className="oc-assistant-companion-identity">
          <button type="button" className="oc-assistant-companion-toggle" aria-label={expanded ? '收起伙伴展示' : '展开伙伴展示'} aria-describedby={pet ? progressId : undefined} aria-expanded={expanded} aria-controls={bodyId} onClick={presentation.toggle}>
            {!pet ? <CompanionEgg/> : <CompanionAvatar species={pet.speciesId} level={level} palette={pet.paletteId} name={pet.name}/>}
            <strong>{pet?.name ?? '等待相遇'}</strong>
            <small>{pet ? `Lv.${level}` : '尚未开蛋'}</small>
            <svg className="oc-assistant-companion-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d={expanded ? 'm6 15 6-6 6 6' : 'm6 9 6 6 6-6'}/></svg>
          </button>
          {pet && <span className="oc-assistant-companion-progress" id={progressId} role="tooltip">
            <span>{progressText}</span>
            <progress aria-label="伙伴升级进度" title={progressText} value={level<100 ? snapshot?.levelXP??0 : 1} max={level<100 ? required : 1}/>
          </span>}
        </div>
        {actions}
      </header>
      {expanded && <div id={bodyId} className="oc-assistant-companion-body">
        <div className="oc-assistant-companion-scene">
          <div className="oc-assistant-companion-stage">
            {enabled && !pet ? <CompanionEgg/> : <SceneCompanion species={pet?.speciesId} stage={stage} palette={pet?.paletteId} name={pet?.name ?? '微光'} action={action} size="var(--oc-companion-figure-size)" visible={visible} onInteract={interact} onComplete={finishInteraction}/>}
          </div>
          {enabled && pet && socialHost ? visible && <AssistantMedals host={socialHost} onManage={onManage}/> : <div className="oc-assistant-companion-start"><button type="button" onClick={onManage}>{enabled ? (pet ? '养成与奖章' : '去开蛋') : '开启养成'}</button></div>}
        </div>
        <div role="separator" tabIndex={0} className="oc-assistant-companion-resize" aria-label="调整伙伴场景高度" aria-orientation="horizontal" aria-controls={bodyId} aria-valuemin={presentation.minimum} aria-valuemax={presentation.maximum} aria-valuenow={presentation.height} aria-valuetext={`${presentation.height} 像素`} title="拖动调整高度 · 双击恢复默认 · 方向键微调" {...presentation.separator}><span/></div>
      </div>}
    </section>;
  };
  return enabled && visible ? <PersonalIdentity host={host}>{content}</PersonalIdentity> : content();
}
