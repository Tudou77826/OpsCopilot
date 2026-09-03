import React, { useEffect, useRef, useState } from 'react';
import { TbRobot, TbSun, TbPalette, TbKeyboard, TbSettings } from 'react-icons/tb';
import type { TerminalConfig, HighlightRule } from '../Terminal/highlightTypes';
import type { Theme } from '../appearanceTypes';
import type { AIConfigRuntime } from './AIConfigCard';
import { assessPattern } from '../Terminal/highlight/regexSafety';
import { ProductSettingsFrame, type SettingsNavGroup } from './ProductSettingsFrame';
import { ProductShellSettingsPage } from './ProductShellSettingsPage';
import { ProductLLMSettings, type ProductLLMConfig } from './ProductLLMSettings';
import { CompletionDelayCard } from './CompletionDelayCard';

export interface ShellSettings {
    revision?: string;
    theme: Theme;
    terminal: TerminalConfig;
    completionDelay: number;
    highlightRules: HighlightRule[];
    /** 命令查询（AI 生成命令）快捷键，如 "Ctrl+K"。 */
    commandQueryShortcut?: string;
}

export interface ShellSettingsRuntime {
    load(): Promise<ShellSettings>;
    save(next: ShellSettings): Promise<void>;
}

export interface ShellSettingsModalProps {
    hostSettings?: React.ReactNode;
    isOpen: boolean;
    embedded?: boolean;
    onClose: () => void;
    runtime: ShellSettingsRuntime;
    /** 宿主实时应用回调（主题/终端/补全/高亮）；保存由 runtime 负责 */
    onApply: (next: ShellSettings) => void;
    /** 初始值（宿主已加载的当前态），避免弹窗打开时闪烁 */
    initial?: ShellSettings | null;
    /** AI 接入配置（独立持久化域）。未注入时 AI 配置卡不渲染（能力边界纪律）。 */
    aiRuntime?: AIConfigRuntime;
}


type Tab = 'host' | 'llm' | 'appearance' | 'highlight' | 'shortcuts' | 'experimental';
const emptyLLM: ProductLLMConfig = { APIKey: '', BaseURL: '', FastModel: '', ComplexModel: '' };
/** Host persistence adapter; all rendered pages and chrome come from the desktop product. */
export default function ShellSettingsModal({ hostSettings, isOpen, embedded, onClose, runtime, onApply, initial, aiRuntime }: ShellSettingsModalProps) {
 const [settings, setSettings] = useState<ShellSettings | null>(null);
 const [saved, setSaved] = useState<ShellSettings | null>(null);
 const [llm, setLLM] = useState(emptyLLM);
 const [savedLLM, setSavedLLM] = useState(emptyLLM);
 const [activeTab, setActiveTab] = useState<Tab>('llm');
 const [searchQuery, setSearchQuery] = useState('');
 const searchInputRef = useRef<HTMLInputElement>(null);
 const [loading, setLoading] = useState(false);
 const [msg, setMsg] = useState('');
 const [loadError, setLoadError] = useState(false);
 const [showUnsavedConfirm, setShowUnsavedConfirm] = useState(false);
 useEffect(() => {
   if (!isOpen) return;
   let cancelled = false;
   setSettings(null); setSaved(null); setLLM(emptyLLM); setSavedLLM(emptyLLM);
   setMsg(''); setLoadError(false); setSearchQuery(''); setShowUnsavedConfirm(false); setActiveTab(aiRuntime ? 'llm' : 'appearance');
   void Promise.all([runtime.load(), aiRuntime?.status()]).then(([next, ai]) => {
     if (cancelled) return;
     setSettings(next); setSaved(next);
     const model = ai ? { APIKey: '', BaseURL: ai.baseURL, FastModel: ai.fastModel, ComplexModel: ai.complexModel } : emptyLLM;
     setLLM(model); setSavedLLM(model);
   }).catch(e => { if (!cancelled) {setLoadError(true); setMsg('读取设置失败: ' + e.message);} });
   return () => {cancelled = true;};
   // Initial values are captured only when the dialog opens, not on live preview.
   // eslint-disable-next-line react-hooks/exhaustive-deps
 }, [isOpen, runtime, aiRuntime]);
 const dirty = JSON.stringify(settings) !== JSON.stringify(saved) || JSON.stringify(llm) !== JSON.stringify(savedLLM);
 const discard = () => { if (saved) onApply(saved); onClose(); };
 const handleClose = () => { if (dirty) setShowUnsavedConfirm(true); else onClose(); };
 const persistConfig = async (close: boolean) => {
   if (!settings || loadError) return;
   setLoading(true); setMsg('');
   try {
     if (aiRuntime && JSON.stringify(llm) !== JSON.stringify(savedLLM)) {
       const ai = await aiRuntime.save({apiKey: llm.APIKey, baseURL: llm.BaseURL, fastModel: llm.FastModel, complexModel: llm.ComplexModel});
       const model = {APIKey:'',BaseURL:ai.baseURL,FastModel:ai.fastModel,ComplexModel:ai.complexModel};
       setLLM(model); setSavedLLM(model);
     }
     await runtime.save(settings); setSaved(settings); onApply(settings); setMsg('设置已保存');
     if (close) onClose();
   } catch(e) {setMsg('保存失败: ' + (e as Error).message);} finally {setLoading(false);}
 };
 useEffect(() => {
   if (!isOpen) return;
   const keydown = (e: KeyboardEvent) => {
     if ((e.ctrlKey || e.metaKey) && e.key === 's') {e.preventDefault(); if (activeTab !== 'host') void persistConfig(false);}
     if (e.key === 'Escape') handleClose();
     if ((e.ctrlKey || e.metaKey) && e.key === 'f') {e.preventDefault(); searchInputRef.current?.focus();}
   };
   window.addEventListener('keydown', keydown);
   return () => window.removeEventListener('keydown', keydown);
 });
 const nav = [
   ...(aiRuntime ? [{id:'llm' as Tab,label:'模型服务',icon:TbRobot({}),category:'AI',keywords:[]}] : []),
   {id:'appearance' as Tab,label:'外观',icon:TbSun({}),category:'终端',keywords:['终端外观','终端','字体','字号','theme','主题']},
   {id:'highlight' as Tab,label:'突出显示',icon:TbPalette({}),category:'终端',keywords:[]},
   {id:'shortcuts' as Tab,label:'快捷键',icon:TbKeyboard({}),category:'交互',keywords:[]},
   {id:'experimental' as Tab,label:'高级选项',icon:TbSettings({}),category:'系统',keywords:['补全','延迟']},
   ...(hostSettings ? [{id:'host' as Tab,label:'本地 Ops',icon:TbSettings({}),category:'系统',keywords:['安装','路径','程序']}] : []),
 ].filter(item => [item.label,item.category,item.id,...item.keywords].some(text => text.toLowerCase().includes(searchQuery.trim().toLowerCase())));
 useEffect(() => { if (isOpen && searchQuery && nav.length && !nav.some(item=>item.id===activeTab)) setActiveTab(nav[0].id); }, [isOpen,searchQuery,activeTab]);
 useEffect(() => { if (isOpen) searchInputRef.current?.focus(); }, [isOpen,activeTab]);
 if (!isOpen) return null;
 const highlightIssues = (settings?.highlightRules || []).flatMap(rule => {
   const risk = assessPattern(rule.pattern || '');
   const issues = [risk.syntaxError ? '语法错误' : '', risk.level === 'severe' ? '灾难性正则' : ''].filter(Boolean);
   return issues.length ? [{name:rule.name,issues}] : [];
 });
 const groups: SettingsNavGroup<Tab>[] = [];
 for (const item of nav) {
   const next = {...item,badge:item.id==='highlight' && highlightIssues.length>0};
   const last = groups[groups.length-1];
   if (last?.category===item.category) last.items.push(next); else groups.push({category:item.category,items:[next]});
 }
 const update = (next: ShellSettings) => {setSettings(next);onApply(next);};
 return <ProductSettingsFrame<Tab> embedded={embedded} showSaveAction={activeTab !== 'host'} handleClose={handleClose} searchInputRef={searchInputRef}
   searchQuery={searchQuery} setSearchQuery={setSearchQuery} groupedNavItems={groups} activeTab={activeTab} setActiveTab={setActiveTab}
   msg={msg} handleSave={()=>void persistConfig(false)} loading={loading || !settings || loadError}
   showUnsavedConfirm={showUnsavedConfirm} setShowUnsavedConfirm={setShowUnsavedConfirm} onClose={discard} persistConfig={persistConfig}>
   {activeTab === 'host' ? hostSettings : !settings ? (loadError ? '设置暂不可用' : '加载中...') :
     activeTab === 'llm' ? <ProductLLMSettings value={llm} onChange={setLLM} keyDescription={aiRuntime?.persistence==='session'
       ? '仅用于本次运行时，不写入配置文件；留空保留现有密钥。仅发送你主动提交给 AI 的内容。'
       : '密钥保存在本地后台，读取不回明文；留空保留现有密钥。'}/> :
     activeTab === 'experimental' ? <CompletionDelayCard value={settings.completionDelay} onChange={completionDelay=>update({...settings,completionDelay})}/> :
     <ProductShellSettingsPage activeTab={activeTab} config={{terminal:settings.terminal,highlight_rules:settings.highlightRules,command_query_shortcut:settings.commandQueryShortcut || 'Ctrl+K'}}
       setConfig={next=>update({...settings,terminal:next.terminal!,highlightRules:next.highlight_rules!,commandQueryShortcut:next.command_query_shortcut})}
       theme={settings.theme} onThemeChange={theme=>update({...settings,theme})} highlightIssues={highlightIssues}/>}
 </ProductSettingsFrame>;
}
