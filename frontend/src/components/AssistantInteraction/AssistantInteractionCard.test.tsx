import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AssistantInteractionCard } from './AssistantInteractionCard';
import type { InteractionPrompt } from './types';
const approval: InteractionPrompt = { id: 'p1', kind: 'approval', title: '确认操作', description: '等待时终端可以操作', command: "printf 'hello'", expires: new Date(Date.now() + 60000).toISOString(), multiple: false };
describe('助手交互卡片', () => {
    it('回退原因和快速模型建议在确认卡片中默认可见', () => {
        const notice='快速模型访问失败，本次自审改用隔离的 iCode 会话。建议在「设置 → 模型服务」配置可用的快速模型。';
        render(<AssistantInteractionCard prompt={{...approval,details:[{label:'AI 自审',value:'需要人工确认'},{label:'自审方式',value:notice}]}} onRespond={vi.fn()} onStop={vi.fn()}/>);
        expect(screen.getByText(notice)).toBeVisible();
        expect(screen.queryByText('更多操作详情')).not.toBeInTheDocument();
    });
    it('传输两端路径、方向和身份默认可见', () => {
        const prompt: InteractionPrompt = { ...approval, command: undefined, details: [
            {label:'传输方向',value:'本机 → 服务器'}, {label:'本机文件',value:'C:\\packages\\plugin.zip'},
            {label:'远端文件',value:'/opt/packages/plugin.zip'}, {label:'执行身份',value:'operator'},
        ] };
        render(<AssistantInteractionCard prompt={prompt} onRespond={vi.fn()} onStop={vi.fn()} />);
        for (const detail of prompt.details!) expect(screen.getByText(detail.value)).toBeVisible();
        expect(screen.queryByText('更多操作详情')).toBeNull();
    });
    it('明确允许一次，显示原命令并防止重复提交', async () => {
        const respond = vi.fn().mockReturnValue(new Promise(() => {}));
        const stop = vi.fn();
        render(<AssistantInteractionCard prompt={approval} onRespond={respond} onStop={stop} />);
        expect(screen.getByLabelText('将要执行的命令')).toHaveTextContent("printf 'hello'");
        expect(screen.getByText('仅批准本次操作，不扩大已有授权范围')).toBeVisible();
        fireEvent.click(screen.getByRole('button', { name: '允许本次' }));
        await waitFor(() => expect(respond).toHaveBeenCalledWith({ text: '', selected: ['allow'] }));
        expect(screen.getByRole('button', { name: '正在提交…' })).toBeDisabled();
        fireEvent.click(screen.getByRole('button', { name: '停止本轮' }));
        expect(stop).toHaveBeenCalledOnce();
    });
    it('将 AI 目的与宿主快速判断分开展示', () => {
        const prompt: InteractionPrompt = { ...approval, details: [{label:'AI 操作目的（供参考）',value:'确认日志目录'}, {label:'快速判定',value:'无法自动判定'}, {label:'确认原因',value:'复合语法需要核对最终影响'}] };
        render(<AssistantInteractionCard prompt={prompt} onRespond={vi.fn()} onStop={vi.fn()} />);
        expect(screen.getByText('AI 操作目的（供参考）')).not.toBeVisible();
        expect(screen.getByText('无法自动判定')).toBeVisible();
        expect(screen.getByText('复合语法需要核对最终影响')).toBeVisible();
        expect(screen.getByLabelText('将要执行的命令')).toBeVisible();
        fireEvent.click(screen.getByText('更多操作详情'));
        expect(screen.getByText('AI 操作目的（供参考）')).toBeVisible();
    });
    it('拒绝与允许分开处理，提交失败可以重试', async () => {
        const respond = vi.fn().mockRejectedValue(Error('这条确认已结束'));
        render(<AssistantInteractionCard prompt={approval} onRespond={respond} onStop={vi.fn()} />);
        fireEvent.click(screen.getByRole('button', { name: '拒绝' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('这条确认已结束');
        expect(respond).toHaveBeenCalledWith({ text: '', selected: ['reject'] });
        expect(screen.getByRole('button', { name: '拒绝' })).not.toBeDisabled();
    });
    it('多选和补充回答直接返回同一个问题', async () => {
        const respond = vi.fn().mockResolvedValue(undefined);
        const prompt: InteractionPrompt = { ...approval, id: 'q1', kind: 'input', command: undefined, title: '哪些节点？', multiple: true, choices: [{ id: 'a', label: 'A 节点' }, { id: 'b', label: 'B 节点' }] };
        render(<AssistantInteractionCard prompt={prompt} onRespond={respond} onStop={vi.fn()} />);
        expect(screen.getByRole('button', { name: '提交回答' })).toBeDisabled();
        fireEvent.click(screen.getByLabelText('A 节点')); fireEvent.click(screen.getByLabelText('B 节点'));
        fireEvent.change(screen.getByLabelText('补充回答'), { target: { value: ' 今天的日志 ' } });
        fireEvent.click(screen.getByRole('button', { name: '提交回答' }));
        await waitFor(() => expect(respond).toHaveBeenCalledWith({ selected: ['a', 'b'], text: '今天的日志' }));
    });
});
