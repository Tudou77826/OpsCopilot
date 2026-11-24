import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CommandQueryOverlay from '../command/CommandQueryOverlay';
import { useProductNavigation } from './useProductNavigation';
import { useCommandQuery, type CommandQueryHost } from './useCommandQuery';

afterEach(cleanup);
const host = (): CommandQueryHost => ({generate:vi.fn(async()=>({command:'pwd',explanation:'directory'})),type:vi.fn(),copy:vi.fn(async()=>{}),warn:vi.fn()});
describe('shared product controllers used by desktop and Teams',()=>{
  for(const initial of [false,true]) it(`keeps tab toggle and quick-panel rules with initial=${initial}`,()=>{
    const {result}=renderHook(()=>useProductNavigation({sidebarOpen:initial,quickOpen:initial}));
    act(()=>result.current.toggleSidebar('script'));
    expect(result.current.sidebarOpen).toBe(true);expect(result.current.tab).toBe('script');
    act(()=>result.current.toggleSidebar('script'));expect(result.current.sidebarOpen).toBe(false);
    act(()=>result.current.toggleSidebar('script'));expect(result.current.sidebarOpen).toBe(true);
    expect(result.current.quickOpen).toBe(initial);
  });
  it('requires an active terminal and leaves editing shortcuts alone, including Shadow DOM',()=>{
    const runtime=host();const {result,rerender}=renderHook(({id})=>useCommandQuery(runtime,id),{initialProps:{id:null as string|null}});
    fireEvent.keyDown(window,{key:'k',ctrlKey:true});expect(runtime.warn).toHaveBeenCalledOnce();expect(result.current.visible).toBe(false);
    rerender({id:'terminal'});
    const surface=document.createElement('div');document.body.append(surface);const shadow=surface.attachShadow({mode:'open'});const input=document.createElement('input');shadow.append(input);
    fireEvent.keyDown(input,{key:'k',ctrlKey:true,composed:true});expect(result.current.visible).toBe(false);
    fireEvent.keyDown(window,{key:'k',ctrlKey:true});expect(result.current.visible).toBe(true);surface.remove();
  });
  it('generates once, preserves explanation, copies and types without executing',async()=>{
    const runtime=host();const {result}=renderHook(()=>useCommandQuery(runtime,'terminal'));
    await act(()=>result.current.generate('  where am I  '));
    expect(runtime.generate).toHaveBeenCalledWith('where am I');expect(result.current.result?.explanation).toBe('directory');
    await act(()=>result.current.copy());expect(runtime.copy).toHaveBeenCalledWith('pwd');
    act(()=>result.current.type());expect(runtime.type).toHaveBeenCalledWith('pwd');expect(result.current.visible).toBe(false);
  });
  it('submits the query from mouse clicks, Enter, and regeneration', async () => {
    const runtime = host();
    function Overlay() {
      const query = useCommandQuery(runtime, 'terminal');
      return <CommandQueryOverlay
        visible={query.visible}
        query={query.query}
        loading={query.loading}
        result={query.result}
        error={query.error}
        onQueryChange={query.setQuery}
        onGenerate={query.generate}
        onRegenerate={query.generate}
        onCopy={query.copy}
        onType={query.type}
        onClose={() => query.setVisible(false)}
      />;
    }
    render(<Overlay />);
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: '  show the current directory  ' } });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '生成', exact: true })); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '重新生成' })); });
    await act(async () => { fireEvent.keyDown(input, { key: 'Enter' }); });
    expect(runtime.generate).toHaveBeenCalledTimes(3);
    for (const call of vi.mocked(runtime.generate).mock.calls) {
      expect(call).toEqual(['show the current directory']);
    }
  });
  it('ignores stale responses after close or terminal change',async()=>{
    let resolve!: (value:{command:string})=>void;const runtime=host();runtime.generate=vi.fn(()=>new Promise<{command:string}>(r=>{resolve=r}));
    const {result,rerender}=renderHook(({id})=>useCommandQuery(runtime,id),{initialProps:{id:'first'}});
    let pending!:Promise<void>;act(()=>{pending=result.current.generate('query')});
    rerender({id:'second'});await act(async()=>{resolve({command:'stale'});await pending});
    expect(result.current.result).toBeNull();expect(result.current.loading).toBe(false);
    act(()=>{pending=result.current.generate('another')});act(()=>result.current.setVisible(false));
    await act(async()=>{resolve({command:'closed'});await pending});expect(result.current.result).toBeNull();
  });
  it('surfaces generation and dispatch errors instead of claiming success',async()=>{
    const runtime=host();runtime.generate=vi.fn(async()=>{throw new Error('model unavailable')});
    const {result}=renderHook(()=>useCommandQuery(runtime,'terminal'));
    await act(()=>result.current.generate('query'));expect(result.current.error).toBe('model unavailable');expect(result.current.loading).toBe(false);
  });
});
