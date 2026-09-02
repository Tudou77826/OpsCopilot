/**
 * TerminalChannel：一条终端的数据面通道（PTY 字节流 WebSocket）。
 *
 * 协议（docs/workbench-shell-plugin-plan.md §2）：
 *   - 挂载后服务端先发环形缓冲重放（首帧二进制），随后持续推送实时输出；
 *   - 客户端发二进制帧 = 用户键入；resize 走控制面不走数据面。
 */
import { SidecarClient } from './sidecarClient';
import { PtyDecoder } from './ptyDecode';

export interface TerminalChannelOptions {
  client: SidecarClient;
  /** 数据面 base：dev 为 ws://127.0.0.1:<port>；平台模式由适配层提供 */
  wsBase: string;
  token: string;
  terminalId: string;
}

export class TerminalChannel {
  private ws: WebSocket | null = null;
  private decoder = new PtyDecoder();
  private onData: ((text: string) => void) | null = null;
  private lastError: string | null = null;
  private closedByUser = false;
  /** 服务端/网络断开（非用户主动）；上层据此提示重连。参数为失败原因。 */
  onLost: ((reason: string) => void) | null = null;

  constructor(private readonly options: TerminalChannelOptions) {}

  /** 建立数据面连接并开始投递（含重放）。 */
  open(onData: (text: string) => void): Promise<void> {
    this.onData = onData;
    this.closedByUser = false;
    const url = `${this.options.wsBase}/terminals/${encodeURIComponent(this.options.terminalId)}?token=${encodeURIComponent(this.options.token)}`;
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      ws.binaryType = 'arraybuffer';
      this.ws = ws;
      ws.addEventListener('open', () => resolve());
      ws.addEventListener('error', () => {
        // 浏览器不给 WS 失败的具体原因；close 事件随后到达。
        this.lastError = '数据面 WebSocket 连接失败（open 前）';
      });
      ws.addEventListener('close', (ev) => {
        if (this.closedByUser) return;
        const reason = this.lastError ?? `数据面连接关闭（code=${ev.code} clean=${ev.wasClean}）`;
        this.onLost?.(reason);
      });
      ws.addEventListener('message', (event) => this.handleMessage(event));
    });
  }

  private handleMessage(event: MessageEvent) {
    if (typeof event.data === 'string') {
      return; // 数据面只承载二进制；字符串帧容错忽略
    }
    const text = this.decoder.push(new Uint8Array(event.data as ArrayBuffer));
    if (text && this.onData) this.onData(text);
  }

  send(data: string): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(new TextEncoder().encode(data));
  }

  close(): void {
    this.closedByUser = true;
    this.ws?.close();
    this.ws = null;
  }
}
