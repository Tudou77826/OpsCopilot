import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SidecarClient } from './sidecarClient';
import { PtyDecoder } from './ptyDecode';

// ---- WebSocket mock：可手动投递消息/触发事件 ----
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static OPEN = 1;
  static CONNECTING = 0;
  readyState = 0;
  binaryType = '';
  url: string;
  sent: (string | ArrayBuffer)[] = [];
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }
  addEventListener(type: string, cb: any) {
    if (type === 'open') this.onopen = cb;
    if (type === 'close') this.onclose = cb;
    if (type === 'error') this.onerror = cb;
    if (type === 'message') this.onmessage = cb;
  }
  removeEventListener() {}
  send(data: string | ArrayBuffer) { this.sent.push(data); }
  close() { this.readyState = 3; this.onclose?.(); }
  // 测试辅助
  fakeOpen() { this.readyState = 1; this.onopen?.(); }
  fakeMessage(data: unknown) { this.onmessage?.({ data }); }
}

describe('SidecarClient', () => {
  let originalWS: unknown;
  beforeEach(() => {
    originalWS = (globalThis as any).WebSocket;
    MockWebSocket.instances = [];
    (globalThis as any).WebSocket = MockWebSocket;
  });
  afterEach(() => {
    (globalThis as any).WebSocket = originalWS;
  });

  it('请求/应答按 id 配对；通知路由到订阅者', async () => {
    const client = new SidecarClient({ url: 'ws://x/rpc' });
    const opened = client.open();
    const ws = MockWebSocket.instances[0];
    ws.fakeOpen();
    await opened;

    // 通知：terminal/exited
    const seen: any[] = [];
    client.on('terminal/exited', (p: any) => seen.push(p));

    const callPromise = client.connect({ host: 'h', user: 'u', password: 'p' });
    const frame = JSON.parse(ws.sent[0] as string);
    expect(frame.method).toBe('shell.connect');
    ws.fakeMessage(JSON.stringify({ jsonrpc: '2.0', id: frame.id, result: { connectionId: 'conn-1' } }));
    await expect(callPromise).resolves.toEqual({ connectionId: 'conn-1' });

    ws.fakeMessage(JSON.stringify({ jsonrpc: '2.0', method: 'terminal/exited', params: { terminalId: 'term-1' } }));
    expect(seen).toEqual([{ terminalId: 'term-1' }]);
  });

  it('初始化返回当前数据面地址及 token', async () => {
    const client = new SidecarClient({ url: 'ws://localhost:45963/rpc?token=test' });
    const opened = client.open();
    const ws = MockWebSocket.instances[0];
    ws.fakeOpen();
    await opened;
    const pending = client.initialize();
    const frame = JSON.parse(ws.sent[0] as string);
    expect(frame.method).toBe('initialize');
    const result = { protocol: 1, version: 'dev', wsBase: 'ws://127.0.0.1:45963', token: 'test' };
    ws.fakeMessage(JSON.stringify({ jsonrpc: '2.0', id: frame.id, result }));
    await expect(pending).resolves.toEqual(result);
    client.close();
  });

  it('错误应答 reject；关闭后未决请求全部失败', async () => {
    const client = new SidecarClient({ url: 'ws://x/rpc' });
    const opened = client.open();
    const ws = MockWebSocket.instances[0];
    ws.fakeOpen();
    await opened;

    const errPromise = client.resize('term-9', 80, 24);
    const frame = JSON.parse(ws.sent[0] as string);
    ws.fakeMessage(JSON.stringify({ jsonrpc: '2.0', id: frame.id, error: { code: -32000, message: '终端不存在' } }));
    await expect(errPromise).rejects.toThrow('终端不存在');

    const hangPromise = client.closeTerminal('term-1');
    ws.onclose?.();
    await expect(hangPromise).rejects.toThrow('连接已关闭');
  });
});

describe('PtyDecoder（流式 UTF-8 拼接）', () => {
  it('跨帧拆开的中文不乱码', () => {
    const decoder = new PtyDecoder();
    const full = new TextEncoder().encode('仓库状态：干净');
    // 第一帧只有前 7 字节（"仓库状态：" 尾字节被切在中间）
    const a = decoder.push(full.slice(0, 7));
    expect(a).toBe('仓库');
    expect(!a.includes('\uFFFD')).toBe(true);
    const b = decoder.push(full.slice(7));
    expect((a + b)).toBe('仓库状态：干净');
  });

  it('二进制残留不丢', () => {
    const decoder = new PtyDecoder();
    const enc = new TextEncoder();
    let out = decoder.push(enc.encode('aaa'));
    out += decoder.push(enc.encode('日'));
    // "日" 的 3 字节完整到达
    expect(out).toBe('aaa日');
  });
});
