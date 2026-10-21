import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import AIChatPanel from './AIChatPanel';
import type { AssistantConversationSnapshot, AssistantRuntime, AssistantTurnState } from '../../../../frontend-shell/src/assistant/runtime';

const completed = (output = '助手的回答'): AssistantTurnState => ({turnId:'turn',target:{tabId:'',connectionId:''},phase:'completed',message:'本轮已完成',output});
function runtime(): AssistantRuntime {
    return {newConversation:vi.fn(async()=>undefined),start:vi.fn(async()=>completed()),state:vi.fn(async()=>completed()),stop:vi.fn(async()=>undefined),respond:vi.fn(async()=>undefined)};
}
const submit = (text: string) => {
    fireEvent.change(screen.getByPlaceholderText('输入问题…'), {target:{value:text}});
    fireEvent.click(screen.getByTitle('发送'));
};

describe('统一 AI 对话', () => {
    beforeEach(() => { Element.prototype.scrollIntoView = vi.fn(); });
    it('没有 SSH 也能直接聊天，不再提供知识问答模式', async () => {
        const host = runtime();
        render(<AIChatPanel runtime={host}/>);
        expect(screen.queryByRole('combobox', {name:'助手模式'})).not.toBeInTheDocument();
        expect(screen.queryByText('知识问答')).not.toBeInTheDocument();
        submit('你好');
        expect(await screen.findByText('助手的回答')).toBeVisible();
        expect(host.start).toHaveBeenCalledWith('你好',undefined);
        expect(screen.queryByText('请先打开一个 SSH 终端')).not.toBeInTheDocument();
    });
    it('默认适配器也把无目标消息交给同一个会话入口', async () => {
        const start = vi.fn(async()=>completed('没有连接也能回答'));
        const legacy = vi.fn();
        window.go = { main:{ App:{RestoreAssistantConversation:vi.fn(async()=>({id:'test',messages:[],truncated:false})),StartAssistantTurn:start,AskCompanionAI:legacy,StopAssistantTurn:vi.fn(async()=>undefined)} } } as any;
        render(<AIChatPanel/>);
        await waitFor(()=>expect(screen.getByPlaceholderText('输入问题…')).toBeEnabled());
        submit('你是谁？');
        expect(await screen.findByText('没有连接也能回答')).toBeVisible();
        expect(start).toHaveBeenCalledWith('你是谁？','','');
        expect(legacy).not.toHaveBeenCalled();
    });
    it('活动终端只是可选工具目标，发送后切换页面不会改掉本轮目标', async () => {
        let resolve!: (state:AssistantTurnState)=>void;
        const host = runtime();
        host.start = vi.fn(()=>new Promise<AssistantTurnState>(value=>{resolve=value;}));
        const view = render(<AIChatPanel runtime={host} activeTerminalTitle="原终端" activeTerminalTarget={{tabId:'one',connectionId:'connection'}}/>);
        submit('看下日志');
        view.rerender(<AIChatPanel runtime={host} activeTerminalTitle="另一终端" activeTerminalTarget={{tabId:'two',connectionId:'another'}}/>);
        expect(host.start).toHaveBeenCalledWith('看下日志',{tabId:'one',connectionId:'connection'});
        expect(screen.getByTitle('本轮终端 · 原终端')).toBeVisible();
        expect(screen.getByLabelText('另一终端')).toBeVisible();
        resolve(completed());
        await screen.findByText('助手的回答');
    });
    it('纯聊天也能停止，不需要终端页签', async () => {
        let state:AssistantTurnState = {...completed(),phase:'running',output:''};
        const host = runtime();
        host.start=vi.fn(async()=>state);
        host.state=vi.fn(async()=>state);
        host.stop=vi.fn(async()=>{state={...state,phase:'stopped',message:'已停止'};});
        render(<AIChatPanel runtime={host}/>);
        submit('请解释一下');
        const stop = await screen.findByRole('button',{name:'停止本轮'});
        await waitFor(()=>expect(stop).toBeEnabled());
        fireEvent.click(stop);
        await waitFor(()=>expect(host.stop).toHaveBeenCalledWith('turn'));
        await waitFor(()=>expect(screen.getByRole('button',{name:'新建对话'})).toBeEnabled());
    });
    it('新建对话清空已完成的消息', async () => {
        const host = runtime();
        render(<AIChatPanel runtime={host}/>);
        submit('一个问题');
        await screen.findByText('助手的回答');
        fireEvent.click(screen.getByRole('button',{name:'新建对话'}));
        await waitFor(()=>expect(screen.queryByTestId('message-item')).not.toBeInTheDocument());
        expect(host.newConversation).toHaveBeenCalledOnce();
        expect(screen.getByPlaceholderText('输入问题…')).toHaveValue('');
    });
    it('刷新后恢复后台原文、停止状态，并允许真正新建对话', async () => {
        const host = runtime();
        host.restoreConversation = vi.fn(async():Promise<AssistantConversationSnapshot>=>({id:'old',truncated:false,messages:[
            {id:'one:user',role:'user',content:'刷新前的问题'},
            {id:'one:ai',role:'ai',content:'刷新前已收到的回答',phase:'stopped'},
        ]}));
        render(<AIChatPanel runtime={host}/>);
        expect(await screen.findByText('刷新前的问题')).toBeVisible();
        expect(screen.getByText('刷新前已收到的回答')).toBeVisible();
        expect(screen.getByText('本轮已停止，未自动续跑')).toBeVisible();
        expect(host.start).not.toHaveBeenCalled();
        expect(host.respond).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button',{name:'新建对话'}));
        expect(await screen.findByText('有什么需要一起排查？')).toBeVisible();
        expect(screen.queryByText('刷新前的问题')).not.toBeInTheDocument();
        expect(host.newConversation).toHaveBeenCalledOnce();
    });
    it('恢复失败时禁止误发并支持重试，不把错误伪装成新对话', async () => {
        const host = runtime();
        host.restoreConversation = vi.fn().mockRejectedValueOnce(Error('连接中断')).mockResolvedValueOnce({id:'old',messages:[{id:'one:ai',role:'ai',content:'恢复成功'}],truncated:true});
        render(<AIChatPanel runtime={host}/>);
        expect(await screen.findByRole('alert')).toHaveTextContent('连接中断');
        expect(screen.getByPlaceholderText('输入问题…')).toBeDisabled();
        fireEvent.click(screen.getByRole('button',{name:'重试恢复对话'}));
        expect(await screen.findByText('恢复成功')).toBeVisible();
        expect(screen.getByText('仅显示最近的消息，助手仍可能记得更早的上下文。')).toBeVisible();
        expect(screen.getByPlaceholderText('输入问题…')).toBeEnabled();
    });
    it('审批卡片位于状态与停止操作上方', async () => {
        const host = runtime();
        const state:AssistantTurnState = {...completed(),phase:'waiting',prompt:{id:'approval',kind:'approval',title:'确认测试操作',description:'仅用于测试布局',multiple:false,expires:'',choices:[]}};
        host.start = vi.fn(async()=>state);
        host.state = vi.fn(async()=>state);
        const view = render(<AIChatPanel runtime={host}/>);
        submit('测试确认布局');
        await screen.findByRole('heading',{name:'确认测试操作'});
        const card = view.container.querySelector('.oc-assistant-interaction')!;
        const activity = view.container.querySelector('.ai-chat-activity')!;
        expect(card.compareDocumentPosition(activity) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
    it('后端重置失败时保留原对话，避免界面与模型上下文不一致', async () => {
        const host = runtime();
        host.newConversation = vi.fn(async()=>{throw Error('仍在停止');});
        render(<AIChatPanel runtime={host}/>);
        submit('一个问题');
        await screen.findByText('助手的回答');
        fireEvent.click(screen.getByRole('button',{name:'新建对话'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('仍在停止');
        expect(screen.getByText('助手的回答')).toBeVisible();
    });
    it('运行时回车加入队列，停止后队列暂停且不会自动执行', async () => {
        let state:AssistantTurnState = {...completed(),phase:'running',output:''};
        const host = runtime();
        host.start=vi.fn(async()=>state);
        host.state=vi.fn(async()=>state);
        host.stop=vi.fn(async()=>{state={...state,phase:'stopped',message:'已停止'};});
        render(<AIChatPanel runtime={host}/>);
        submit('第一条问题');
        const stop = await screen.findByRole('button',{name:'停止本轮'});
        await waitFor(()=>expect(stop).toBeEnabled());
        const input = screen.getByPlaceholderText('输入问题…');
        expect(input).toBeEnabled();
        fireEvent.change(input,{target:{value:'下一条草稿'}});
        fireEvent.keyDown(input,{key:'Enter'});
        expect(host.start).toHaveBeenCalledTimes(1);
        expect(host.stop).not.toHaveBeenCalled();
        expect(screen.queryByRole('button',{name:'添加上下文'})).not.toBeInTheDocument();
        fireEvent.click(stop);
        await waitFor(()=>expect(screen.getByLabelText('待发送消息')).toHaveTextContent('队列已暂停'));
        expect(screen.getByLabelText('待发送消息')).toHaveTextContent('下一条草稿');
        expect(input).toHaveValue('');
        expect(screen.getByRole('button',{name:'加入队列'})).toBeDisabled();
        expect(host.start).toHaveBeenCalledTimes(1);
        expect(host.stop).toHaveBeenCalledWith('turn');
    });
    it('立即发送先停止旧轮次，然后在同一对话执行选中的消息', async () => {
        let state:AssistantTurnState = {...completed(),phase:'running',output:''};
        const host = runtime();
        host.start=vi.fn(async()=>state);
        host.state=vi.fn(async()=>state);
        host.stop=vi.fn(async()=>{state={...state,phase:'stopped',message:'已停止'};});
        render(<AIChatPanel runtime={host}/>);
        submit('第一条问题');
        fireEvent.change(screen.getByPlaceholderText('输入问题…'),{target:{value:'只看今天的日志'}});
        fireEvent.click(screen.getByRole('button',{name:'加入队列'}));
        host.start=vi.fn(async()=>completed('按新要求继续'));
        fireEvent.click(screen.getByRole('button',{name:'立即发送'}));
        expect(await screen.findByText('按新要求继续')).toBeVisible();
        expect(host.stop).toHaveBeenCalledWith('turn');
        expect(host.start).toHaveBeenCalledWith('只看今天的日志',undefined);
        expect(host.newConversation).not.toHaveBeenCalled();
    });
    it('阅读历史时新回答不抢滚动，用户可主动回到最新', async () => {
        const host = runtime();
        render(<AIChatPanel runtime={host}/>);
        submit('第一条问题');
        await screen.findByText('助手的回答');
        let resolve!:(state:AssistantTurnState)=>void;
        host.start = vi.fn(()=>new Promise<AssistantTurnState>(value=>{resolve=value;}));
        submit('第二条问题');
        const history = screen.getByLabelText('对话消息');
        Object.defineProperties(history,{scrollHeight:{configurable:true,value:1000},clientHeight:{configurable:true,value:200}});
        history.scrollTop=100;
        fireEvent.scroll(history);
        resolve(completed('新的回答'));
        await screen.findByText('新的回答');
        expect(history.scrollTop).toBe(100);
        fireEvent.click(screen.getByRole('button',{name:'回到最新'}));
        expect(history.scrollTop).toBe(1000);
        expect(screen.queryByRole('button',{name:'回到最新'})).not.toBeInTheDocument();
    });
});
