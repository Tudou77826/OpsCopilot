import { build } from 'esbuild'
import { mkdir, writeFile, readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
const root = import.meta.dirname
const alias = { react: resolve(root, 'node_modules/react'), 'react-dom': resolve(root, 'node_modules/react-dom') }
const worker = await build({ entryPoints: [resolve(root, '../../frontend-shell/src/ui/Terminal/highlight/matcher.worker.ts')], bundle: true, write: false, format: 'esm', platform: 'browser', minify: true })
const workerURL = 'data:application/javascript;base64,' + Buffer.from(worker.outputFiles[0].contents).toString('base64')
const inlineWorker = { name: 'inline-shell-worker', setup(plugin: any) { plugin.onLoad({ filter: /[/\\]RuleMatcher\.ts$/ }, async (args: any) => ({ contents: (await readFile(args.path, 'utf8')).replace("new URL('./matcher.worker.ts', import.meta.url)", JSON.stringify(workerURL)), loader: 'ts', resolveDir: resolve(args.path, '..') })) } }
const base = { entryPoints: [resolve(root, 'src/ui.tsx')], bundle: true, write: false, format: 'esm' as const, platform: 'browser' as const, outfile: resolve(root, 'dist/ui.js'), alias, plugins: [inlineWorker], loader: { '.png': 'dataurl' as const, '.woff2': 'dataurl' as const, '.woff': 'dataurl' as const, '.ttf': 'dataurl' as const }, define: { 'process.env.NODE_ENV': '"production"' }, minify: true }
const first = await build({ ...base, define: { ...base.define, OPS_STYLES: '""' } })
const styles = first.outputFiles!.find(f => f.path.endsWith('.css'))!.text.replace(/:root\[([^\]]+)\]/g, ':host([$1])').replace(/:root\b/g, ':host')
const output = await build({ ...base, define: { ...base.define, OPS_STYLES: JSON.stringify(styles) } })
await mkdir(resolve(root, 'dist'), { recursive: true })
await writeFile(resolve(root, 'dist/ui.js'), output.outputFiles!.find(f => f.path.endsWith('.js'))!.contents)
console.log('Built isolated product UI')
