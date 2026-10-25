import { useEffect, useState } from 'react';
import type { InteractionAnswer, InteractionPrompt } from './types';
import './assistantInteraction.css';

interface Props {
    prompt: InteractionPrompt;
    onRespond(answer: InteractionAnswer): Promise<void>;
    onStop(): void;
    showStop?: boolean;
}
const locationLabels = new Set(['执行位置', '目标节点', '执行身份', '当前目录', '传输方向', '本机文件', '远端文件', '本地目录']);
const primaryLabels = new Set([...locationLabels, 'AI 自审', '自审方式', '规则检查', '快速判定', '执行影响', '确认原因']);
export function AssistantInteractionCard({ prompt, onRespond, onStop, showStop = true }: Props) {
    const [selected, setSelected] = useState<string[]>([]);
    const [text, setText] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    useEffect(() => { setSelected([]); setText(''); setError(''); setBusy(false); }, [prompt.id]);
    async function send(answer: InteractionAnswer) {
        if (busy) return;
        setBusy(true); setError('');
        try { await onRespond(answer); }
        catch (reason) { setError(String(reason)); setBusy(false); }
    }
    const location = prompt.details?.filter(detail => locationLabels.has(detail.label)) ?? [];
    const assessment = prompt.details?.filter(detail => primaryLabels.has(detail.label) && !locationLabels.has(detail.label)) ?? [];
    const more = prompt.details?.filter(detail => !primaryLabels.has(detail.label)) ?? [];
    const details = (items: typeof location) => <dl>{items.map((detail, index) => <div key={index}><dt>{detail.label}</dt><dd>{detail.value}</dd></div>)}</dl>;
    return <section className="oc-assistant-interaction" aria-label={prompt.title}>
        <header><span className="oc-interaction-eyebrow">{prompt.kind === 'approval' ? '需要你的确认' : '助手有一个问题'}</span>{showStop && <button type="button" onClick={onStop}>停止本轮</button>}</header>
        <h3>{prompt.title}</h3>
		{prompt.kind === 'approval' && <p className="oc-interaction-expiry">等待至 {new Date(prompt.expires).toLocaleTimeString()}；超时不会授权</p>}
        {!!location.length && <div className="oc-interaction-location">{details(location)}</div>}
        {prompt.command && <pre tabIndex={0} aria-label="将要执行的命令"><code>{prompt.command}</code></pre>}
        {!!assessment.length && details(assessment)}
        {prompt.description && <p>{prompt.description}</p>}
        {!!more.length && <details className="oc-interaction-more"><summary>更多操作详情</summary>{details(more)}</details>}
        {prompt.kind === 'approval' ? <footer>
            <span>仅批准本次操作，不扩大已有授权范围</span>
            <button type="button" disabled={busy} onClick={() => void send({ text: '', selected: ['reject'] })}>拒绝</button>
            <button className="oc-interaction-primary" type="button" disabled={busy} onClick={() => void send({ text: '', selected: ['allow'] })}>{busy ? '正在提交…' : prompt.choices?.find(choice => choice.id === 'allow')?.label || '允许本次'}</button>
        </footer> : <form onSubmit={event => { event.preventDefault(); void send({ text: text.trim(), selected }); }}>
            {!!prompt.choices?.length && <fieldset disabled={busy}><legend>{prompt.multiple ? '可选择多项' : '选择一项，或补充回答'}</legend>{prompt.choices.map(choice => <label key={choice.id}>
                <input type={prompt.multiple ? 'checkbox' : 'radio'} name={prompt.id} checked={selected.includes(choice.id)} onChange={event => setSelected(previous => prompt.multiple ? event.target.checked ? [...previous, choice.id] : previous.filter(id => id !== choice.id) : [choice.id])} />{choice.label}
            </label>)}</fieldset>}
            <textarea aria-label="补充回答" placeholder="输入你的回答…" disabled={busy} value={text} onChange={event => setText(event.target.value)} maxLength={8192} rows={2} />
            <footer><span>等待你回答，不会自动选择</span><button className="oc-interaction-primary" type="submit" disabled={busy || (!selected.length && !text.trim())}>{busy ? '正在提交…' : '提交回答'}</button></footer>
        </form>}
        {error && <div className="oc-interaction-error" role="alert">{error}</div>}
    </section>;
}
