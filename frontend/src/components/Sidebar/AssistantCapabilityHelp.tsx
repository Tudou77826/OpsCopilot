import { useId, useState } from 'react';
import { TbHelpCircle } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';

const capabilities = [
    { tool: 'workspace_open', title: 'SSH 连接', detail: '查找已保存连接，打开新终端或切换现有终端' },
    { tool: 'workspace_create', title: '新建 SSH 配置', detail: '预填连接表单，由你补全凭据并保存' },
    { tool: 'terminal_context', title: '远程终端', detail: '读取终端输出，接管后执行命令' },
    { tool: 'database_list', title: '数据库', detail: '新建配置、打开连接、查看结构和执行 SQL' },
    { tool: 'file_workspaces', title: '文件', detail: '浏览 SSH 文件目录，上传和下载文件' },
    { tool: 'local_directories', title: '本地项目', detail: '查看项目及工作目录，添加项目目录' },
    { tool: 'knowledge_search', title: '运维资料', detail: '查知识库、Skills 和快捷指令' },
];

export function AssistantCapabilityHelp({ load }: { load(): Promise<string[]> }) {
    const id = useId();
    const [open, setOpen] = useState(false);
    const [tools, setTools] = useState<string[]>();
    const [loading, setLoading] = useState(false);
    const [failed, setFailed] = useState(false);
    function show() {
        setOpen(true);
        if (tools || loading) return;
        setLoading(true);
        setFailed(false);
        let timeout: number;
        const deadline = new Promise<never>((_, reject) => {
            timeout = window.setTimeout(() => reject(new Error('能力列表加载超时')), 5000);
        });
        void Promise.race([Promise.resolve().then(load), deadline])
            .then(setTools, () => setFailed(true))
            .finally(() => {
                window.clearTimeout(timeout);
                setLoading(false);
            });
    }
    const available = capabilities.filter(item => tools?.includes(item.tool));
    return (
        <div className="ai-capability-help" onMouseEnter={show} onMouseLeave={() => setOpen(false)}
            onFocus={show} onBlur={() => setOpen(false)} onKeyDown={event => {
                if (event.key === 'Escape' && open) {
                    event.stopPropagation();
                    setOpen(false);
                }
            }}>
            <IconButton label="Agent 可用能力" aria-describedby={open ? id : undefined} onClick={show}>
                <TbHelpCircle size={18} />
            </IconButton>
            {open && <div id={id} role="tooltip" className="ai-capability-tooltip">
                <strong>OpsCopilot 提供的能力</strong>
                {available.length > 0 ? <>
                    <dl>{available.map(item => <div key={item.tool}><dt>{item.title}</dt><dd>{item.detail}</dd></div>)}</dl>
                    <p>读屏、执行及传输按当前审批模式授权。</p>
                </> : <p>{failed ? '暂时无法读取能力列表' : tools ? '当前会话没有 OpsCopilot 工具' : '正在读取能力列表…'}</p>}
            </div>}
        </div>
    );
}
