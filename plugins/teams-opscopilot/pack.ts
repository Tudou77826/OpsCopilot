import { build } from 'esbuild'
import { createHash, sign } from 'node:crypto'
import { readFile, writeFile, mkdir } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { version } from './src/business.js'
import { compatibility } from './src/compatibility.js'

export async function pack(output: string, signingKey?: string) {
  if (process.platform !== 'win32' || process.arch !== 'x64') throw new Error('首个构件仅支持 Windows x64；其他架构需独立构建和验收')
  const root = import.meta.dirname
  await mkdir(output, { recursive: true })
  await import('./build.js')
  const business = await build({ entryPoints: [join(root, 'src/entry.ts')], bundle: true, write: false, format: 'esm', platform: 'node', packages: 'external' })
  const ui = await readFile(join(root, 'dist/ui.js'), 'utf8')
  const { contributions } = await import('./dist/ui.js' as string)
  const manifest = { id: 'opscopilot', version, hostApi: compatibility.hostApi, business: 'business.js', ui: 'ui.js', contract: 'contract.js', publisher: 'OpsCopilot', contributions }
  const texts = { 'business.js': business.outputFiles[0].text, 'ui.js': ui, 'contract.js': `export const schemaVersion = 1; export const compatibility = ${JSON.stringify(compatibility)};` }
  await Promise.all([writeFile(join(output, 'manifest.json'), JSON.stringify(manifest, null, 2)), ...Object.entries(texts).map(([name, contents]) => writeFile(join(output, name), contents))])
  const artifact = Buffer.from(JSON.stringify({ format: 2, manifest, files: texts }))
  await writeFile(join(output, 'opscopilot.bundle.json'), artifact)
  const sha256 = createHash('sha256').update(artifact).digest('hex')
  const release = { componentId: manifest.id, version, artifact: 'opscopilot.bundle.json', sha256, ...(signingKey ? { signature: sign(null, Buffer.from(`${manifest.id}\n${version}\n${sha256}`), signingKey).toString('base64') } : {}) }
  await writeFile(join(output, signingKey ? 'release.json' : 'release.unsigned.json'), JSON.stringify(release, null, 2))
  return { manifest, output, release }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const key = process.env.OPSCOPILOT_RELEASE_KEY ? await readFile(process.env.OPSCOPILOT_RELEASE_KEY, 'utf8') : undefined
  const result = await pack(resolve(process.argv[2] || 'dist/package-win32-x64'), key)
  console.log(JSON.stringify({ directory: result.output, signed: Boolean(key), version }))
}
