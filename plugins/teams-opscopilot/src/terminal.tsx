import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import TerminalComponent, { type TerminalRef } from '../../../frontend-shell/src/ui/Terminal/Terminal'
import type { ShellSettings } from '../../../frontend-shell/src/ui/settings/ShellSettingsModal'
import type { TeamsOpsClient } from './browser-client'

export function TeamsTerminal({ client, id, settings, attachRef, register }: {
  client: TeamsOpsClient; id: string; settings: ShellSettings;
  attachRef(value: TerminalRef | null): void;
  register(id: string, send?: (data: string) => void): void;
}) {
  const term = useRef<TerminalRef | null>(null), channel = useRef<Awaited<ReturnType<TeamsOpsClient['attach']>>>()
  const [error, setError] = useState(''), [attempt, setAttempt] = useState({ n: 0, takeover: false })
  const write = useCallback((data: string) => { try { if (!channel.current) throw new Error('终端未附着，输入未发送'); channel.current.send(data) } catch (e) { setError((e as Error).message) } }, [])
  const runtime = useMemo(() => ({
    resize: (terminalId: string, cols: number, rows: number) => client.call('terminals.resize', { terminalId, cols, rows }).catch(e => setError(e.message)),
    getCompletions: (input: string, cursor: number) => client.call('completion', { input, cursor }),
  }), [client])
  useEffect(() => {
    let disposed = false
    setError('')
    term.current?.write('\r\n\x1b[2m[附着终端，以下可能包含历史输出]\x1b[0m\r\n')
    void client.attach(id, text => term.current?.write(text), () => setError('数据通道已断开或被脚本/其他窗口接管；不会自动重连'), attempt.takeover).then(value => {
      if (disposed) { value.close(); return }
      channel.current = value; register(id, write)
    }).catch(e => { if (!disposed) setError(e.message) })
    return () => { disposed = true; register(id); channel.current?.close(); channel.current = undefined }
  }, [client, id, attempt, register, write])
  return <div className="ops-terminal">
    {error && <div className="ops-terminal-alert" role="alert">{error} <button onClick={() => setAttempt(v => ({ n: v.n + 1, takeover: false }))}>重新附着</button><button onClick={() => { if (window.confirm('接管会撤销其他窗口的输入权限，是否继续？')) setAttempt(v => ({ n: v.n + 1, takeover: true })) }}>接管</button></div>}
    <TerminalComponent id={id} sessionID={id} ref={value => { term.current = value; attachRef(value) }} runtime={runtime} onData={write}
      theme={settings.theme} terminalConfig={settings.terminal} completionDelay={settings.completionDelay} highlightRules={settings.highlightRules}/>
  </div>
}
