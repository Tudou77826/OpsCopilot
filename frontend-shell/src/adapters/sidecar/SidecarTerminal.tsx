import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { TerminalComponent, TerminalRef, Theme } from '../../ui';
import type { TerminalConfig, HighlightRule } from '../../ui/Terminal/highlightTypes';
import type { CompletionData } from '../../ui/Terminal/CompletionOverlay';
import { SidecarClient } from '../../core/sidecarClient';
import { TerminalChannel } from '../../core/terminalChannel';
import { registerTerminalInput, unregisterTerminalInput } from '../../core/terminalRegistry';

export interface SidecarTerminalProps {
  client: SidecarClient;
  terminalId: string;
  wsBase: string;
  token: string;
  theme?: Theme;
  terminalRef?: (value: TerminalRef | null) => void;
  onLost?: (terminalId: string, reason: string) => void;
  /** 终端配置（回滚/搜索/高亮/字体/字号）。与 Wails 链一致，决定搜索/高亮/回滚行为。 */
  terminalConfig?: TerminalConfig;
  /** 补全延迟（ms）。 */
  completionDelay?: number;
  /** 高亮规则。 */
  highlightRules?: HighlightRule[];
  /** 命令补全（shell.completion RPC）。宿主未注入时终端静默关闭补全。 */
  getCompletions?: (input: string, cursor: number) => Promise<CompletionData | null>;
}

/** 将 sidecar PTY 通道适配到共享 Terminal UI。 */
export const SidecarTerminal: React.FC<SidecarTerminalProps> = ({
  client,
  terminalId,
  wsBase,
  token,
  theme,
  terminalRef: attachTerminalRef,
  onLost,
  terminalConfig,
  completionDelay,
  highlightRules,
  getCompletions,
}) => {
  const terminalRef = useRef<TerminalRef | null>(null);
  const channelRef = useRef<TerminalChannel | null>(null);
  // onLost 常为宿主内联箭头（每次渲染新身份），走 ref 避免数据面 WS 被反复关闭重开。
  const onLostRef = useRef(onLost);
  useEffect(() => { onLostRef.current = onLost; });
  const [error, setError] = useState('');
  // 数据面早于终端挂载到达的下行缓冲。
  const pendingRef = useRef<string[]>([]);
  // onData 走稳定回调：共享 Terminal 的 init effect 依赖它，内联箭头会让 xterm 随宿主重渲染被整建清屏。
  const handleData = useCallback((data: string) => channelRef.current?.send(data), []);
  const runtime = useMemo(() => ({
    resize: (sessionId: string, cols: number, rows: number) => client.resize(sessionId, cols, rows),
    ...(getCompletions ? { getCompletions } : {}),
  }), [client, getCompletions]);

  useEffect(() => {
    const channel = new TerminalChannel({ client, wsBase, token, terminalId });
    channelRef.current = channel;
    channel.onLost = (reason) => onLostRef.current?.(terminalId, reason);
    let cancelled = false;

    // 数据面首条消息（ring 回放）可能先于共享 Terminal 挂载到达；未就绪时缓冲，ref 附着时冲刷。
    void channel.open((text) => {
      const term = terminalRef.current;
      if (term) term.write(text);
      else pendingRef.current.push(text);
    })
      .then(() => registerTerminalInput(terminalId, (data) => channel.send(data)))
      .catch((openError) => {
        if (!cancelled) setError((openError as Error).message || '数据面连接失败');
      });

    return () => {
      cancelled = true;
      unregisterTerminalInput(terminalId);
      channel.close();
      channelRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, terminalId, token, wsBase]);

  return (
    <div className="stv-wrap">
      {error && <div className="stv-error" role="alert">{error}</div>}
      <TerminalComponent
        id={terminalId}
        sessionID={terminalId}
        runtime={runtime}
        theme={theme}
        terminalConfig={terminalConfig}
        completionDelay={completionDelay}
        highlightRules={highlightRules}
        onData={handleData}
        ref={(value) => {
          terminalRef.current = value;
          if (value && pendingRef.current.length > 0) {
            const buffered = pendingRef.current;
            pendingRef.current = [];
            for (const text of buffered) value.write(text);
          }
          attachTerminalRef?.(value);
        }}
      />
    </div>
  );
};
