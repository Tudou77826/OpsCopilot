import React, { useEffect, useState } from 'react'
import type { TeamsOpsClient } from './browser-client'
import { OpsApp } from './app'
import { productSettingsStyles as styles } from '../../../frontend-shell/src/ui/settings/productSettingsStyles'
import { btnPrimary, btnSecondary, pageContainer } from '../../../frontend-shell/src/ui/settings/settingsStyles'

type Installation = { configured: boolean; executable: string; state: string; message: string }

/** Host-specific installation controls, using the product's existing settings styles. */
export function LocalOpsSetup({ client, onReady }: { client: TeamsOpsClient; onReady(): void }) {
  const [path, setPath] = useState(''), [message, setMessage] = useState(''), [busy, setBusy] = useState(false)
  useEffect(() => { let live = true; void client.call<Installation>('installation.status').then(value => { if (live) { setPath(value.executable); setMessage(value.message) } }).catch(error => { if (live) setMessage(error.message) }); return () => { live = false } }, [client])
  const perform = async (work: () => Promise<void>) => { setBusy(true); setMessage(''); try { await work() } catch (error) { setMessage((error as Error).message) } finally { setBusy(false) } }
  return <section style={styles.card} aria-label="本地 Ops 接入">
    <h2 style={styles.cardTitle}>连接本地 OpsCopilot</h2>
    <p style={styles.rowDesc}>选择你已安装或解压的 OpsCopilot.exe。Teams 会启动它的无界面插件进程，沿用本地连接、快捷命令、脚本和设置，不复制配置文件。</p>
    <label style={styles.rowLabel} htmlFor="local-ops-exe">OpsCopilot 程序位置</label>
    <div style={styles.row}>
      <input id="local-ops-exe" style={{ ...styles.inputWide, flex: 1, minWidth: 0 }} value={path} disabled={busy} placeholder="D:\Apps\OpsCopilot\OpsCopilot.exe" onChange={event => setPath(event.target.value)} />
      <button style={btnSecondary} disabled={busy} onClick={() => void perform(async () => { const result = await client.call<{ executable: string; cancelled: boolean }>('installation.choose'); if (!result.cancelled) setPath(result.executable) })}>浏览…</button>
    </div>
    <p style={styles.rowDesc}>仅选择你信任的本地 Ops 程序。首次需要支持 Teams 插件模式的新版；升级后请先退出仍在运行的旧版。以后记住此位置，更换程序前请结束活动连接。</p>
    <button style={btnPrimary} disabled={busy || !path.trim()} onClick={() => void perform(async () => { await client.call('installation.configure', { executable: path.trim(), confirmed: true }); onReady() })}>{busy ? '处理中…' : '保存并启动'}</button>
    {message && <p role="alert">{message}</p>}
  </section>
}

export function OpsEntry({ client, surface }: { client: TeamsOpsClient; surface: HTMLElement }) {
  const [ready, setReady] = useState(false), [loading, setLoading] = useState(true), [generation, setGeneration] = useState(0)
  useEffect(() => { let live = true; void client.call<Installation>('installation.status').then(value => { if (live) setReady(value.state === 'ready') }).finally(() => { if (live) setLoading(false) }).catch(() => {}); return () => { live = false } }, [client])
  const started = () => { setReady(true); setGeneration(value => value + 1) }
  if (loading) return <div role="status">正在读取本地 Ops 接入状态…</div>
  if (!ready) return <div style={{ ...pageContainer, width: '100%', overflow: 'auto' }}><LocalOpsSetup client={client} onReady={started}/></div>
  return <OpsApp key={generation} client={client} surface={surface} hostSettings={<LocalOpsSetup client={client} onReady={started}/>}/>
}
