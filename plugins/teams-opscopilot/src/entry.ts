import type { Context } from '@deepseek-ai/cordis'
import { isAbsolute } from 'node:path'
import { createOpsPlugin, version } from './business.js'
import { SidecarRuntime } from './sidecar-runtime.js'
import { LocalInstallation } from './local-installation.js'
export { version }
export default async function plugin(ctx: Context, config: { host?: { protocol: number; bundleId: string; version: string; artifactDirectory: string; dataDirectory: string } }) {
 const host=config?.host
 if(!host||host.protocol!==1||host.bundleId!=='opscopilot'||host.version!==version||!isAbsolute(host.dataDirectory)) throw new Error('OpsCopilot requires the versioned Native host context')
 if(process.platform!=='win32') throw new Error('首版支持 Windows 本地 Ops')
 const installation=new LocalInstallation(host.dataDirectory)
 await installation.load()
 const runtime=new SidecarRuntime(()=>installation.resolve())
 await createOpsPlugin(runtime,host.bundleId,{terminalTransport:true,installation})(ctx)
}
