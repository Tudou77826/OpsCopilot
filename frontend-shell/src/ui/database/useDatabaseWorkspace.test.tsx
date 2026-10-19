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
