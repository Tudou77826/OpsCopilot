// 门禁自身的验证：它必须在该红的地方红、该绿的地方绿，否则后面每一步的"门禁全绿"都只是没有检查过。
// 用 node --test 运行（不是 vitest）：这个文件在 frontend-shell 的 tsconfig 之外，
// 因此不需要给浏览器代码放开 Node 类型（见 docs/theme-design.md 第 8 节）。
//
//   node --test tools/checks/check-style-tokens.test.mjs
//   npm --prefix frontend-shell run lint:style:self
import { execFileSync } from 'node:child_process'
import assert from 'node:assert/strict'
import test from 'node:test'
import { join } from 'node:path'

const repoRoot = join(import.meta.dirname, '..', '..')
const gate = join(import.meta.dirname, 'check-style-tokens.mjs')
const bareColorFixture = 'tools/checks/__fixtures__/bare-color'
const emptyFixture = 'tools/checks/__fixtures__/scan-empty'
const sizeReferenceFixture = 'tools/checks/__fixtures__/size-references'
const tokenFixture = (name) => `tools/checks/__fixtures__/theme-tokens/${name}`

function run(args) {
  try {
    return { code: 0, stdout: execFileSync(process.execPath, [gate, ...args], { encoding: 'utf8', cwd: repoRoot }), stderr: '' }
  } catch (error) {
    return { code: error.status ?? -1, stdout: error.stdout ?? '', stderr: error.stderr ?? '' }
  }
}

test('裸色值检查命中 hex 与颜色函数，放行令牌派生写法与全透明，跳过测试文件', () => {
  const result = run([bareColorFixture, '--token-file=none'])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /bad\.tsx/)
  assert.match(result.stderr, /#ff0000/)
  assert.match(result.stderr, /rgba\(0, 0, 0, 0\.5\)/)
  assert.doesNotMatch(result.stderr, /good\.tsx/)
  assert.doesNotMatch(result.stderr, /#abcdef/)
})

test('投影令牌缺亮色覆写时失败，非颜色令牌不要求覆写', () => {
  const result = run([emptyFixture, `--token-file=${tokenFixture('missing-light.css')}`])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /--shadow 缺少亮色覆写/)
  assert.doesNotMatch(result.stderr, /--scrollbar-size/)
})

test('只声明在亮色块的令牌失败', () => {
  const result = run([emptyFixture, `--token-file=${tokenFixture('light-only.css')}`])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /--accent-hover 只声明在亮色块/)
})

test('双主题完整时通过，并打印与模式无关的豁免项', () => {
  const result = run([emptyFixture, `--token-file=${tokenFixture('complete.css')}`])
  assert.equal(result.code, 0)
  assert.match(result.stdout, /--scrim（mode-independent）/)
  assert.match(result.stdout, /样式门禁通过/)
})

test('库新增的 FlexLayout 调色板变量没被映射时失败', () => {
  const result = run([
    emptyFixture,
    '--token-file=none',
    '--library-css=tools/checks/__fixtures__/flexlayout/library.css',
    '--mapping-file=tools/checks/__fixtures__/flexlayout/mapping.css',
  ])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /未映射库变量 --color-popup-selected-background/)
  assert.doesNotMatch(result.stderr, /未映射库变量 --color-text/)
})

test('引用未声明的尺寸令牌时失败，已声明的不报', () => {
  const result = run([sizeReferenceFixture, `--token-file=${tokenFixture('size-tokens.css')}`])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /未声明的尺寸令牌 --space-999/)
  assert.match(result.stderr, /未声明的尺寸令牌 --radius-nope/)
  assert.doesNotMatch(result.stderr, /未声明的尺寸令牌 --space-8\b/)
  assert.doesNotMatch(result.stderr, /未声明的尺寸令牌 --radius-sm\b/)
})

test('尺寸令牌只声明在亮色块时失败', () => {
  const result = run([emptyFixture, `--token-file=${tokenFixture('size-light-only.css')}`])
  assert.equal(result.code, 1)
  assert.match(result.stderr, /尺寸令牌 --space-8 声明在亮色块/)
})

test('真实仓库的 FlexLayout 映射完整且入口不再引入第三方暗色样式', () => {
  const result = run([])
  assert.doesNotMatch(result.stderr, /未映射库变量/)
  assert.doesNotMatch(result.stderr, /flexlayout-react\/style\/dark\.css/)
  assert.doesNotMatch(result.stderr, /未引入flexlayout-react\/style\/light\.css/)
  assert.doesNotMatch(result.stderr, /未声明的尺寸令牌/)
  assert.doesNotMatch(result.stderr, /声明在亮色块/)
})
