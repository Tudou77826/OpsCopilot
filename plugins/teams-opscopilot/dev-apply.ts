// 开发期把当前插件产物挂载到本机 iCode Teams 宿主：复用 pack.ts 的生产打包路径，
// POST 到 Core 的官方装载入口 /api/bundles/apply（dev-plugin.ts 只支持 hostApi 2–5，
// 本插件是 hostApi 6，所以直接走 Core 路由）。宿主需以固定 ICODE_LOCAL_CREDENTIAL 启动。
import { resolve } from 'node:path'
import { pack } from './pack.js'

const credential = process.env.ICODE_LOCAL_CREDENTIAL
if (!credential) throw new Error('ICODE_LOCAL_CREDENTIAL is required')
const coreUrl = process.env.ICODE_CORE_URL ?? 'http://127.0.0.1:45831'

const { manifest, output } = await pack(resolve(import.meta.dirname, 'dist/dev-apply'))
const response = await fetch(`${coreUrl}/api/bundles/apply`, {
  method: 'POST',
  headers: { authorization: `Bearer ${credential}`, 'content-type': 'application/json' },
  body: JSON.stringify({ manifest, artifactPath: output }),
})
if (!response.ok) throw new Error(`Core rejected bundle (${response.status}): ${await response.text()}`)
console.log(JSON.stringify({ ok: true, id: manifest.id, version: manifest.version }))
