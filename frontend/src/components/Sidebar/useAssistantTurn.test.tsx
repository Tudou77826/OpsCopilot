import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { useAssistantTurn } from './useAssistantTurn';
import type { AssistantTurnState, AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';

const state = (phase: string): AssistantTurnState => ({turnId:'original-turn', target:{tabId:'original-tab',connectionId:'original-connection'},phase,message:phase,output:phase==='completed'?'原终端结果':undefined});
describe('terminal assistant turn fences', () => {
    it('手动接管撤权后仍等待宿主取消收尾，不能提前显示轮次已结束', async () => {
        let cleanup!: () => void;
        const draining = new Promise<void>(resolve => { cleanup = resolve; });
        const runtime:AssistantRuntime={newConversation:vi.fn(),start:vi.fn(async()=>state('stopped')),state:vi.fn(),stop:vi.fn(()=>draining),respond:vi.fn()};
        const {result}=renderHook(()=>useAssistantTurn(runtime));
        let settled=false;
        let request!:ReturnType<typeof result.current.ask>;
        act(()=>{request=result.current.ask('新要求').then(value=>{settled=true;return value;});});
        await waitFor(()=>expect(runtime.stop).toHaveBeenCalledWith('original-turn'));
        expect(settled).toBe(false);
        expect(result.current.state?.phase).toBe('stopped');
        await act(async()=>{cleanup();await request;});
        expect(settled).toBe(true);
        expect(result.current.state).toBeUndefined();
    });
    it('binds the initial target and returns the matching final output', async () => {
        const runtime: AssistantRuntime = {newConversation:vi.fn(async()=>undefined),start:vi.fn(async()=>state('running')),state:vi.fn(async()=>state('completed')),stop:vi.fn(async()=>undefined),respond:vi.fn(async()=>undefined)};
        const {result} = renderHook(()=>useAssistantTurn(runtime));
        const target = {tabId:'original-tab',connectionId:'original-connection'};
        let answer = '';
        await act(async()=>{const request=result.current.ask('查看日志',target);target.tabId='user-switched-tab';answer=(await request).output;});
        expect(runtime.start).toHaveBeenCalledWith('查看日志',{tabId:'original-tab',connectionId:'original-connection'});
        expect(answer).toBe('原终端结果');
    });
    it('stops the exact late-started turn after unmount', async () => {
        let resolveStart!:(value:AssistantTurnState)=>void;
        const runtime: AssistantRuntime = {newConversation:vi.fn(async()=>undefined),start:vi.fn(()=>new Promise<AssistantTurnState>(resolve=>{resolveStart=resolve;})),state:vi.fn(),stop:vi.fn(async()=>undefined),respond:vi.fn()};
        const {result,unmount}=renderHook(()=>useAssistantTurn(runtime));
        const request=result.current.ask('查看日志',{tabId:'tab',connectionId:'connection'}).catch(error=>String(error));
        unmount();resolveStart(state('running'));await request;
        expect(runtime.stop).toHaveBeenCalledWith('original-turn');
        expect(runtime.state).not.toHaveBeenCalled();
    });
    it('responds only to the pending request and keeps stop available', async () => {
        const waiting={...state('waiting'),prompt:{id:'question',kind:'input',title:'哪一天？',description:'',multiple:false,expires:'',choices:[]}};
        let latest:AssistantTurnState=waiting;
        const runtime:AssistantRuntime={newConversation:vi.fn(async()=>undefined),start:vi.fn(async()=>waiting),state:vi.fn(async()=>latest),respond:vi.fn(async()=>{latest=state('completed');}),stop:vi.fn(async()=>{latest=state('stopped');})};
        const {result}=renderHook(()=>useAssistantTurn(runtime));
        let request!:ReturnType<typeof result.current.ask>;
        act(()=>{request=result.current.ask('查看日志',{tabId:'tab',connectionId:'connection'});});
        await waitFor(()=>expect(result.current.state?.prompt?.id).toBe('question'));
        await act(async()=>{await result.current.respond({text:'今天',selected:[]});await request;});
        expect(runtime.respond).toHaveBeenCalledWith('original-turn','question',{text:'今天',selected:[]});
    });
});
