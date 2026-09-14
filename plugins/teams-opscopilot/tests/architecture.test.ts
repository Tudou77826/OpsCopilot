import assert from 'node:assert/strict'
import { readFile, readdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'
import ts from 'typescript'

const root = fileURLToPath(new URL('../../../', import.meta.url))
const parse = async (path: string) => ts.createSourceFile(path, await readFile(path, 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
function visit(node: ts.Node, inspect: (node: ts.Node) => void) { inspect(node); ts.forEachChild(node, child => visit(child, inspect)) }

// Check actual imports and usages, not a comment containing the shared component name.
test('desktop and Teams use the same product frame, toolbar, navigation and controllers', async () => {
  for (const entry of ['frontend/src/App.tsx', 'plugins/teams-opscopilot/src/app.tsx']) {
    const path = resolve(root, entry), source = await parse(path)
    const required = new Map([
      ['ProductFrame', 'ProductChrome'], ['ProductToolbar', 'ProductChrome'], ['ProductNavigation', 'ProductChrome'],
      ['useProductNavigation', 'useProductNavigation'], ['useCommandQuery', 'useCommandQuery'],
    ])
    const bindings = new Map<string, string>(), used = new Set<string>()
    visit(source, node => {
      if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
        const imports = node.importClause?.namedBindings
        if (imports && ts.isNamedImports(imports)) for (const item of imports.elements) {
          const original = (item.propertyName ?? item.name).text, module = required.get(original)
          if (module) {
            assert.equal(resolve(dirname(path), node.moduleSpecifier.text), resolve(root, 'frontend-shell/src/ui/product', module), `${entry}: ${original} must come from shared product source`)
            bindings.set(item.name.text, original)
          }
        }
      }
      const name = ts.isCallExpression(node) ? node.expression : ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node) ? node.tagName : undefined
      if (name && ts.isIdentifier(name)) used.add(name.text)
    })
    for (const original of required.keys()) assert([...bindings].some(([local, imported]) => imported === original && used.has(local)), `${entry} must actually use ${original}`)
  }
})

test('shared product controllers stay independent of desktop and Teams transports', async () => {
  const dir = resolve(root, 'frontend-shell/src/ui/product')
  for (const file of await readdir(dir)) {
    if (!/^use.*\.ts$/.test(file)) continue
    const source = await parse(join(dir, file))
    visit(source, node => {
      if (ts.isStringLiteral(node) && (ts.isImportDeclaration(node.parent) || ts.isCallExpression(node.parent))) {
        assert(!/wails|shell-adapter|teams-opscopilot|sidecar|node:|child_process/.test(node.text), `${file}: host-specific dependency ${node.text}`)
      }
      if (ts.isIdentifier(node)) assert(!['window', 'globalThis'].includes(node.text) || !ts.isPropertyAccessExpression(node.parent) || !['go', 'runtime'].includes(node.parent.name.text), `${file}: direct Wails access`)
    })
  }
})
