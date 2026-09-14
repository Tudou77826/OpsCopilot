import type { Context } from '@deepseek-ai/cordis'
import { isAbsolute } from 'node:path'
import { createOpsPlugin, version } from './business.js'
import { SidecarRuntime } from './sidecar-runtime.js'
import { LocalInstallation } from './local-installation.js'
export { version }
export default async function plugin(ctx: Context, config?: { dataDir?: string }) {
 const dataDirectory=config?.dataDir
 if(!dataDirectory||!isAbsolute(dataDirectory)) throw new Error('OpsCopilot requires Native host API 6 plugin storage')
 if(process.platform!=='win32') throw new Error('首版支持 Windows 本地 Ops')
 const installation=new LocalInstallation(dataDirectory)
 await installation.load()
 const runtime=new SidecarRuntime(()=>installation.resolve())
 await createOpsPlugin(runtime,'opscopilot',{terminalTransport:true,installation})(ctx)
}
