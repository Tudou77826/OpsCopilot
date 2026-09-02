/**
 * 终端输入注册器：TerminalView 挂载数据面后把"写入该终端"的能力登记在这里，
 * 快捷命令/脚本等外部动作据此写入当前激活终端。
 * key = terminalId。keep-alive 语义下注册/注销与视图生命周期一致。
 */
const senders = new Map<string, (data: string) => void>();

export function registerTerminalInput(terminalId: string, send: (data: string) => void): void {
  senders.set(terminalId, send);
}

export function unregisterTerminalInput(terminalId: string): void {
  senders.delete(terminalId);
}

/** 向指定终端写入；终端不存在时返回 false。 */
export function sendToTerminal(terminalId: string, data: string): boolean {
  const send = senders.get(terminalId);
  if (!send) return false;
  send(data);
  return true;
}
