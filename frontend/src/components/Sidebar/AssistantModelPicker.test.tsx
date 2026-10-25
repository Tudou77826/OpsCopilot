import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import type { AssistantModels, AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import { AssistantModelPicker } from './AssistantModelPicker';

const models = [{id:'first',name:'Default',modelId:'model-one',provider:'openai'},{id:'second',name:'Fast',modelId:'model-two',provider:'anthropic'}];
function fixture() {
    const runtime: AssistantRuntime = {newConversation:vi.fn(),start:vi.fn(),state:vi.fn(),stop:vi.fn(),respond:vi.fn(),
        modelOptions:vi.fn(async()=>({models,current:models[0]})), selectModel:vi.fn(async()=>({models,current:models[1]}))};
    return {runtime,onChanging:vi.fn(),onError:vi.fn()};
}
afterEach(() => vi.restoreAllMocks());
it('显示实际模型，切换确认后更新，执行时禁止切换', async () => {
    const props=fixture();
    let finish!: (value: AssistantModels) => void;
    props.runtime.selectModel=vi.fn(()=>new Promise<AssistantModels>(resolve=>{finish=resolve;}));
    const view=render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const select=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(select).toHaveTextContent('Default'));
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    fireEvent.click(select);
    expect(screen.getByRole('option',{name:'Default'})).toHaveAttribute('aria-selected','true');
    fireEvent.click(screen.getByRole('option',{name:'Fast'}));
    expect(props.runtime.selectModel).toHaveBeenCalledWith('second');
    expect(select).toBeDisabled();
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(props.onChanging).toHaveBeenCalledWith(true);
    finish({models,current:models[1]});
    await waitFor(()=>expect(select).toHaveTextContent('Fast'));
    expect(props.onChanging).toHaveBeenLastCalledWith(false);
    view.rerender(<AssistantModelPicker {...props} busy revision={0}/>);
    expect(select).toBeDisabled();
});
it('切换失败保留原模型并呈现错误，读取失败允许重试', async () => {
    const props=fixture();
    props.runtime.selectModel=vi.fn(async()=>{throw Error('未接受切换');});
    render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const select=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(select).toHaveTextContent('Default'));
    fireEvent.click(select);
    fireEvent.click(screen.getByRole('option',{name:'Fast'}));
    await waitFor(()=>expect(props.onError).toHaveBeenCalledWith(expect.stringContaining('未接受切换')));
    expect(select).toHaveTextContent('Default');
    fireEvent.click(screen.getByRole('button',{name:'重新读取 iCode 模型'}));
    await waitFor(()=>expect(props.runtime.modelOptions).toHaveBeenCalledTimes(2));
});

it('菜单定位在触发器上方，并通过独立浮层避开输入框裁切', async () => {
    const props=fixture();
    const view=render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(trigger).toBeEnabled());
    vi.spyOn(trigger,'getBoundingClientRect').mockReturnValue({left:320,right:430,top:700,bottom:728,width:110,height:28,x:320,y:700,toJSON:()=>({})});
    fireEvent.click(trigger);
    const popup=screen.getByRole('listbox',{name:'iCode 模型'});
    expect(popup.parentElement).toBe(document.body);
    expect(view.container).not.toContainElement(popup);
    expect(popup).toHaveStyle({bottom:`${window.innerHeight-694}px`,left:'270px',width:'160px',maxHeight:'300px'});
    expect(trigger).toHaveAttribute('aria-controls',popup.id);
});

it('键盘浏览不会立即切换，确认后更新模型并恢复焦点', async () => {
    const props=fixture();
    render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(trigger).toBeEnabled());
    fireEvent.keyDown(trigger,{key:'ArrowDown'});
    fireEvent.keyDown(trigger,{key:'End'});
    expect(trigger).toHaveAttribute('aria-activedescendant',screen.getByRole('option',{name:'Fast'}).id);
    expect(props.runtime.selectModel).not.toHaveBeenCalled();
    fireEvent.keyDown(trigger,{key:'Escape'});
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(trigger).toHaveTextContent('Default');
    fireEvent.keyDown(trigger,{key:'ArrowUp'});
    fireEvent.keyDown(trigger,{key:'ArrowUp'});
    fireEvent.keyDown(trigger,{key:'Enter'});
    await waitFor(()=>expect(trigger).toHaveTextContent('Fast'));
    expect(props.runtime.selectModel).toHaveBeenCalledExactlyOnceWith('second');
    expect(trigger).toHaveFocus();
});

it('点击外部或开始执行时关闭菜单，点击当前模型不发送切换请求', async () => {
    const props=fixture();
    const view=render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(trigger).toBeEnabled());
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole('option',{name:'Default'}));
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(props.runtime.selectModel).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    fireEvent.pointerDown(document.body);
    expect(trigger).toHaveAttribute('aria-expanded','false');
    fireEvent.click(trigger);
    view.rerender(<AssistantModelPicker {...props} busy revision={0}/>);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(trigger).toBeDisabled();
    expect(props.runtime.selectModel).not.toHaveBeenCalled();
});

it('保留已不在配置列表中的会话模型，不会因打开菜单而偷偷切换', async () => {
    const props=fixture();
    props.runtime.modelOptions=vi.fn(async()=>({models,current:{...models[0],id:'removed',name:'会话别名',modelId:'session-model'}}));
    render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(trigger).toHaveTextContent('会话别名'));
    fireEvent.click(trigger);
    expect(screen.getByRole('option',{name:'会话别名'})).toHaveAttribute('aria-selected','true');
    fireEvent.keyDown(trigger,{key:'Tab'});
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(props.runtime.selectModel).not.toHaveBeenCalled();
});

it('读取失败后可重试，拿到配置前不会显示可选菜单', async () => {
    const props=fixture();
    props.runtime.modelOptions=vi.fn().mockRejectedValueOnce(Error('配置暂不可用')).mockResolvedValueOnce({models,current:models[0]});
    render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    const retry=await screen.findByRole('button',{name:'重新读取 iCode 模型'});
    expect(trigger).toBeDisabled();
    fireEvent.click(retry);
    await waitFor(()=>expect(trigger).toBeEnabled());
    expect(trigger).toHaveTextContent('Default');
});

it('无别名时显示模型名，键盘按别名查找仍然只浏览不切换', async () => {
    const props=fixture();
    const unnamed={...models[0],name:'  '};
    props.runtime.modelOptions=vi.fn(async()=>({models:[unnamed,models[1]],current:unnamed}));
    render(<AssistantModelPicker {...props} busy={false} revision={0}/>);
    const trigger=screen.getByRole('combobox',{name:'助手模型'});
    await waitFor(()=>expect(trigger).toHaveTextContent('model-one'));
    fireEvent.click(trigger);
    expect(screen.getByRole('option',{name:'model-one'})).toHaveAttribute('aria-selected','true');
    expect(screen.getByRole('option',{name:'Fast'})).toHaveAttribute('title','Fast · model-two');
    fireEvent.keyDown(trigger,{key:'f'});
    expect(trigger).toHaveAttribute('aria-activedescendant',screen.getByRole('option',{name:'Fast'}).id);
    expect(props.runtime.selectModel).not.toHaveBeenCalled();
});
