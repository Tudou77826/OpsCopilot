import React, { useCallback, useEffect, useRef, useState } from 'react';
import type { AssistantSettings, AssistantSettingsHost } from '../../assistant/settings';
import { btnSmall } from './settingsStyles';
import './persona-settings.css';
import { companionSpecies } from '../../companion/species';

export function usePersonaSettings(host: AssistantSettingsHost | undefined, open: boolean) {
  const [saved, setSaved] = useState<AssistantSettings>();
  const [draft, setDraft] = useState(''), [error, setError] = useState('');
  const request = useRef(0);
  const accept = (next: AssistantSettings) => { setSaved(next); setDraft(next.personaPrompt || next.defaultPrompt); setError(''); };
  const reload = useCallback(async () => {
    if (!host) return;
    const id = ++request.current;
    try { const next = await host.load(); if (id === request.current) accept(next); }
    catch (cause) { if (id === request.current) setError(String(cause instanceof Error ? cause.message : cause)); }
  }, [host]);
  useEffect(() => {
    setSaved(undefined); setDraft(''); setError('');
    if (open) void reload();
    return () => { request.current++; };
  }, [open, reload]);
  const dirty = !!saved && draft !== (saved.personaPrompt || saved.defaultPrompt);
  const save = async () => {
    if (!host || !saved) throw new Error('人设设置尚未读取，请重新加载');
    if (Array.from(draft.trim()).length > 4000) throw new Error('人设提示词最多 4000 字');
    const id = request.current;
    const next = await host.save(draft, saved.revision);
    if (id === request.current) accept(next);
  };
  return { saved, draft, setDraft, error, reload, dirty, save };
}
export default function PersonaSettingsPage({ state, disabled = false }: { state: ReturnType<typeof usePersonaSettings>; disabled?: boolean }) {
  const [template, setTemplate] = useState('lumen');
  return <section className="oc-persona-settings" aria-label="伙伴人设设置">
    <div className="oc-persona-heading"><span>性格、称呼与回答风格 · 保存后下次问答生效</span><button type="button" style={btnSmall} disabled={disabled || !state.saved} onClick={() => state.setDraft(state.saved!.defaultPrompt)}>恢复默认</button></div>
    <div className="oc-persona-templates"><label htmlFor="companion-persona-template">性格模板</label><select id="companion-persona-template" value={template} onChange={event => setTemplate(event.target.value)}>{companionSpecies.map(species => <option key={species.id} value={species.id}>{species.name} · {species.character}</option>)}</select><button type="button" style={btnSmall} disabled={disabled || !state.saved} onClick={() => state.setDraft(companionSpecies.find(species => species.id === template)!.persona)}>使用模板</button></div>
    {state.error && <div role="alert">{state.error}<button type="button" style={btnSmall} onClick={() => void state.reload()}>重新加载人设</button></div>}
    <label htmlFor="companion-persona-prompt">人设提示词</label>
    <textarea id="companion-persona-prompt" value={state.draft} disabled={disabled || !state.saved} maxLength={8000} rows={6} onChange={event => state.setDraft(event.target.value)} aria-describedby="companion-persona-help" placeholder="例如：你是一位耐心的运维伙伴，先给出结论，再解释原因……"/>
    <div className="oc-persona-footer"><span>{Array.from(state.draft).length}/4000 字{state.dirty ? ' · 尚未保存' : state.saved ? '' : ' · 正在读取'}</span></div>
    <p id="companion-persona-help">只保存在本机，不随排名资料公开。问答时会随请求发送给已配置的模型服务。留空使用当前伙伴的默认性格；自定义人设优先，切换伙伴不会覆盖。知识检索、引用和工具权限继续遵循功能规则。</p>
  </section>;
}

export function PersonaSettingsDisclosure(props: Parameters<typeof PersonaSettingsPage>[0]) {
  const { state } = props;
  return <details className="oc-persona-disclosure">
    <summary>伙伴人设 <small>{state.dirty ? '尚未保存' : state.error ? '读取失败' : state.saved?.personaPrompt ? '自定义' : '默认性格'}</small></summary>
    <PersonaSettingsPage {...props}/>
  </details>;
}
