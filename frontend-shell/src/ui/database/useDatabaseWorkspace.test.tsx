import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useDatabaseWorkspace } from './useDatabaseWorkspace';
import { emptyProfile } from './ConnectionDialog';
import type {
  DatabaseRuntime,
  DatabaseSession,
  DatabaseStructure,
} from './types';

const first = { ...emptyProfile(), id: 'p1', name: '连接一', user: 'ops' };
const second = { ...first, id: 'p2', name: '连接二' };
const session = (id: string): DatabaseSession => ({
  id: `s-${id}`,
  profileId: id,
  version: '8.4',
  database: 'orders',
  readOnly: true,
});
function runtime() {
  return {
    profiles: vi.fn(async () => [first, second]),
    groups: vi.fn(async () => []),
    connect: vi.fn(async (id: string) => session(id)),
    disconnect: vi.fn(async () => {}),
    schemas: vi.fn(async () => ['orders']),
    tables: vi.fn(async () => []),
    query: vi.fn(async () => ({
      sets: [],
      durationMs: 1,
      error: '',
      cancelled: false,
      disconnected: false,
    })),
    cancel: vi.fn(async () => true),
  } as unknown as DatabaseRuntime;
}
afterEach(cleanup);
describe('database session ownership', () => {
  it('does not let an old query failure clear the replacement session query', async () => {
    const host=runtime();let counter=0;
    host.connect=vi.fn(async id=>({...session(id),id:`physical-${++counter}`}));
    let rejectOld!:(error:Error)=>void;
    let resolveNew!:(value:Awaited<ReturnType<DatabaseRuntime['query']>>)=>void;
    host.query=vi.fn().mockImplementationOnce(()=>new Promise((_,reject)=>{rejectOld=reject;})).mockImplementationOnce(()=>new Promise(resolve=>{resolveNew=resolve;}));
    const {result}=renderHook(()=>useDatabaseWorkspace(host));
    await act(async()=>{await result.current.openConnection(first);});
    const tab={...result.current.tabs[0],sql:'SELECT 1'};
    let old!:Promise<void>;let next!:Promise<void>;
    act(()=>{old=result.current.execute(tab);});
    await act(async()=>{await result.current.disconnect(first.id);await result.current.connect(first);});
    act(()=>{next=result.current.execute(tab);});
    expect(host.query).toHaveBeenCalledTimes(2);
    await act(async()=>{rejectOld(Error('old connection closed'));await old;});
    expect(result.current.tabs[0].pending).toBeTruthy();
    expect(result.current.tabs[0].error).toBe('');
    await act(async()=>{resolveNew({sets:[],durationMs:777,error:'',cancelled:false,disconnected:false});await next;});
    expect(result.current.tabs[0].result?.durationMs).toBe(777);
  });
  it('keeps the live profile snapshot when a saved edit cannot disconnect', async () => {
    const host=runtime();const {result}=renderHook(()=>useDatabaseWorkspace(host));
    await act(async()=>{await result.current.openConnection(first);});
    host.disconnect=vi.fn(async()=>{throw Error('已取消断开');});
    await act(async()=>{await result.current.saved({...first,host:'new-endpoint'},true);});
    expect(host.connect).toHaveBeenCalledTimes(1);
    expect(result.current.profiles.find(p=>p.id===first.id)?.host).toBe(first.host);
    expect(result.current.error).toContain('当前会话继续使用原配置');
  });
  it('keeps the old session and stops reconnect when disconnect is cancelled', async () => {
    const host = runtime();
    const { result } = renderHook(() => useDatabaseWorkspace(host));
    await act(async () => { await result.current.openConnection(first); });
    host.disconnect = vi.fn(async () => { throw Error('已取消断开'); });
    await act(async () => { await result.current.reconnect(first); });
    expect(host.connect).toHaveBeenCalledTimes(1);
    expect(result.current.connections.p1.session.id).toBe('s-p1');
    expect(result.current.error).toBe('已取消断开');
  });
  it('deduplicates openings per profile while connecting other profiles independently', async () => {
    const host = runtime();
    let finish!: (s: DatabaseSession) => void;
    host.connect = vi.fn((id) =>
      id === 'p1'
        ? new Promise<DatabaseSession>((resolve) => {
            finish = resolve;
          })
        : Promise.resolve(session(id)),
    );
    const { result } = renderHook(() => useDatabaseWorkspace(host));
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.connect(first);
      void result.current.connect(first);
    });
    await act(async () => {
      await result.current.connect(second);
    });
    expect(result.current.connections.p2.session.id).toBe('s-p2');
    expect(result.current.connections.p1).toBeUndefined();
    await act(async () => {
      finish(session('p1'));
      await pending;
    });
    expect(host.connect).toHaveBeenCalledTimes(2);
    expect(Object.keys(result.current.connections)).toHaveLength(2);
  });
  it('closes a connection which finishes after the owning workspace is unmounted', async () => {
    const host = runtime();
    let finish!: (s: DatabaseSession) => void;
    host.connect = vi.fn(
      () =>
        new Promise<DatabaseSession>((resolve) => {
          finish = resolve;
        }),
    );
    const { result, unmount } = renderHook(() => useDatabaseWorkspace(host));
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.connect(first);
    });
    unmount();
    await act(async () => {
      finish(session('p1'));
      await pending;
    });
    expect(host.disconnect).toHaveBeenCalledWith('s-p1');
  });
  it('prevents queries from overlapping metadata work on the same physical session', async () => {
    const host = runtime();
    let finish!: (
      value: Awaited<ReturnType<DatabaseRuntime['structure']>>,
    ) => void;
    host.structure = vi.fn(
      () =>
        new Promise<DatabaseStructure>((resolve) => {
          finish = resolve;
        }),
    );
    const { result } = renderHook(() => useDatabaseWorkspace(host));
    await act(async () => {
      await result.current.openConnection(first);
    });
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.loadStructure('p1', 'orders', 'events');
    });
    await waitFor(() => expect(host.structure).toHaveBeenCalledOnce());
    const tab = { ...result.current.tabs[0], sql: 'SELECT 1' };
    await act(async () => {
      await result.current.execute(tab);
    });
    expect(host.query).not.toHaveBeenCalled();
    await act(async () => {
      finish({
        columns: {
          statement: '',
          columns: [],
          rows: [],
          affectedRows: 0,
          truncated: false,
        },
        indexes: {
          statement: '',
          columns: [],
          rows: [],
          affectedRows: 0,
          truncated: false,
        },
        ddl: '',
      });
      await pending;
      await result.current.execute(tab);
    });
    expect(host.query).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: 's-p1', database: 'orders' }),
    );
  });
});
