import React from 'react'
import { createRoot } from 'react-dom/client'
import { OpsEntry } from './local-ops-setup'
import { TeamsOpsClient } from './browser-client'
import { ShellSurface } from '../../../frontend-shell/src/ui/Surface'
import { ToastProvider } from '../../../frontend-shell/src/ui/feedback/Toast'
import { ConfirmDialogInternal } from '../../../frontend-shell/src/ui/feedback/ConfirmDialog'
import '../../../frontend-shell/src/ui/styles.css'
import '../../../frontend/src/style.css'
import './workspace.css'

declare const OPS_STYLES: string
export const version = '0.1.0'
export const contributions = [
  { id: 'ops-nav', slot: 'navigation', title: 'OpsCopilot', href: '/plugins/opscopilot' },
  { id: 'ops-workspace', slot: 'page', title: 'OpsCopilot' },
  { id: 'ops-settings', slot: 'settings', title: 'OpsCopilot 设置' },
  { id: 'ops-status', slot: 'command', title: 'OpsCopilot 状态', command: 'runtime.status' },
]
export function mount(container: HTMLElement, context?: { bundleId: string }) {
  const host = document.createElement('div'); host.style.cssText = 'height:100%;min-height:0;display:flex;flex:1;min-width:0'; host.dataset.theme = 'light'
  const shadow = host.attachShadow({ mode: 'open' }), style = document.createElement('style'), main = document.createElement('div'), portals = document.createElement('div')
  style.textContent = OPS_STYLES; main.className = 'ops-root'; portals.className = 'ops-portals'; shadow.append(style, main, portals); container.append(host)
  const client = new TeamsOpsClient(context?.bundleId || 'opscopilot'), root = createRoot(main)
  root.render(<ShellSurface.Provider value={{ portalRoot: portals, styleRoot: shadow }}><ToastProvider><ConfirmDialogInternal/><OpsEntry client={client} surface={host}/></ToastProvider></ShellSurface.Provider>)
  return () => { root.unmount(); client.dispose(); host.remove() }
}
export default function ui(_context: unknown, config: { register(value: unknown): () => void }) { return contributions.map(config.register) }
