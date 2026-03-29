import { useEffect, useMemo, useState } from 'react'
import FilesPanel from '../../frontend/src/components/Sidebar/FilesPanel'

type TerminalSessionLite = {
  id: string
  title: string
}

function App() {
  const [sessions, setSessions] = useState<TerminalSessionLite[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [msg, setMsg] = useState('')

  const backend = useMemo(() => window.go.main.FTPApp, [])

  useEffect(() => {
    const init = async () => {
      try {
        const [rawSessions, startup] = await Promise.all([
          backend.ListSessions(),
          backend.GetStartupSession()
        ])
        const parsed = JSON.parse(rawSessions || '[]') as TerminalSessionLite[]
        setSessions(parsed)
        if (startup && parsed.find(s => s.id === startup)) {
          setActiveSessionId(startup)
        } else if (parsed.length > 0) {
          setActiveSessionId(parsed[0].id)
        } else {
          setMsg('未获取到可用会话，请先在主程序建立连接。')
        }
      } catch (e: any) {
        setMsg('初始化失败: ' + e.toString())
      }
    }
    init()
  }, [backend])

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column' }}>
      {msg ? (
        <div style={{ padding: '8px 12px', color: '#aaa', fontSize: '12px', borderBottom: '1px solid #333' }}>{msg}</div>
      ) : null}
      <div style={{ flex: 1, minHeight: 0 }}>
        <FilesPanel activeTerminalId={activeSessionId} terminals={sessions} backend={backend} />
      </div>
    </div>
  )
}

export default App
