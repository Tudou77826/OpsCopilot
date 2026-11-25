import { useState } from 'react';
import type { ConnectionConfig } from '../../types';
import ConnectionPropertiesModal from '../Sidebar/ConnectionPropertiesModal';
import { createSavedConnection } from '../../shell-adapter/wailsSessionRuntime';
import type { InteractionAnswer, InteractionPrompt } from './types';

export function SSHConnectionRequest({ prompt, onRespond }: {
    prompt: InteractionPrompt;
    onRespond(answer: InteractionAnswer): Promise<void>;
}) {
    const [open, setOpen] = useState(false);
    const [savedId, setSavedId] = useState('');
    const [error, setError] = useState('');
    const draft = prompt.data as Partial<ConnectionConfig> | undefined;
    const initialConfig: ConnectionConfig = {
        protocol: 'ssh', host: draft?.host ?? '', port: draft?.port ?? 22,
        user: draft?.user ?? '', name: draft?.name ?? draft?.host ?? '',
    };
    async function respond(id: string) {
        setError('');
        try {
            await onRespond({ text: id, selected: [id ? 'allow' : 'reject'] });
        } catch (reason) {
            setError(String(reason));
        }
    }
    return (
        <section className="oc-assistant-interaction" aria-label={prompt.title}>
            <header><span className="oc-interaction-eyebrow">SSH 连接表单</span></header>
            <h3>{prompt.title}{draft?.host && ` · ${draft.host}`}</h3>
            <p>{prompt.description}</p>
            <footer>
                <button type="button" onClick={() => void respond('')}>{savedId ? '结束请求' : '取消'}</button>
                <button type="button" className="oc-interaction-primary"
                    onClick={() => savedId ? void respond(savedId) : setOpen(true)}>
                    {savedId ? '继续对话' : '填写连接信息'}
                </button>
            </footer>
            {error && <p role="alert">{error}</p>}
            <ConnectionPropertiesModal isOpen={open} mode="create" sshOnly initialConfig={initialConfig}
                onClose={() => setOpen(false)} onSaved={() => setOpen(false)}
                createConnection={async (config, parentId) => {
                    const node = await createSavedConnection(config, parentId);
                    setSavedId(node.id);
                    await respond(node.id);
                }} />
        </section>
    );
}
