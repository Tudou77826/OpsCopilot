// OpsCopilot 样式门禁，规范见 docs/theme-design.md 第 8 节。
//
// 检查 1 裸色值扫描：除令牌文件、终端配色数据、皮肤块与测试外，TS/TSX/CSS 不得出现字面色值。
//         写死的颜色意味着这个元素只在一种主题下成立，而且皮肤覆盖不到它——皮肤只能改令牌。
// 检查 2 双主题完整性：shell-theme.css 的 :root 是暗色全套，:root[data-theme="light"] 必须同名
//         覆写每个颜色与投影令牌；漏一个就会出现"只在暗色下成立"的元素。反向也查：只写在亮色块里
//         的令牌在暗色下没有值，同样算缺陷。
//
// 检查 3 FlexLayout 第三方调色板映射：库把调色板声明在 .flexlayout__layout 上，我们靠同名选择器
//         覆盖。映射文件必须覆盖库声明的每个 --color-* 变量，且样式入口不得再引入库的暗色样式
//         （引入它会在亮色下留下未覆盖的暗色面）。库升级新增变量时这里会失败，而不是静默漏色。
//
// 检查 4 尺寸令牌引用完整性：--space-*/--radius-*/--font-size-* 的每个 var() 引用都必须在令牌文件
//         里有声明，且尺寸令牌只能声明在 :root（尺寸与明暗无关）。写错名字时浏览器不报错、只是静默
//         丢掉整条声明（自定义属性解析失败，属性回落初始值），页面会塌掉，所以必须静态查。
//
// 用法：
//   node tools/checks/check-style-tokens.mjs                          扫默认的两个源码根
//   node tools/checks/check-style-tokens.mjs <相对仓库根的目录> ...    只扫指定目录（fixture 自测用）
//   node tools/checks/check-style-tokens.mjs --token-file=<路径|none>  指定令牌文件；none 表示跳过检查 2 与 4
//   node tools/checks/check-style-tokens.mjs --library-css=<路径> --mapping-file=<路径>  覆盖检查 3 的输入
//
// 与模式无关的令牌（例如在两种模式下同值的遮罩）可在声明行尾加注释 mode-independent 豁免，
// 每次运行都会把豁免项打印出来，避免它变成静默的例外。
//
// 已知不覆盖：CSS 具名颜色（white/black 等）。当前代码库为 0 处，出现时再补规则。
// 检查 1 与检查 4 都跳过 *.test.ts(x)：测试里有刻意冻结的字面量与 fixture 字符串，不是渲染代码。
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

const repoRoot = join(import.meta.dirname, '..', '..')
const defaultRoots = ['frontend-shell/src', 'frontend/src']
const defaultTokenFile = 'frontend-shell/src/ui/styles/shell-theme.css'
const flexlayoutLibraryCss = 'frontend-shell/node_modules/flexlayout-react/style/light.css'
const flexlayoutMappingFile = 'frontend-shell/src/ui/styles/flexlayout-tokens.css'
const flexlayoutEntries = ['frontend-shell/src/ui/styles.css', 'frontend/src/main.tsx']

const args = process.argv.slice(2)
const argument = (name, fallback) => {
  const found = args.find((value) => value.startsWith(`--${name}=`))
  return found ? found.slice(name.length + 3) : fallback
}
const tokenFile = argument('token-file', defaultTokenFile)
const libraryCss = argument('library-css', flexlayoutLibraryCss)
const mappingFile = argument('mapping-file', flexlayoutMappingFile)
const rootArgs = args.filter((value) => !value.startsWith('--'))
const scanRoots = (rootArgs.length ? rootArgs : defaultRoots).map((root) => ({
  dir: join(repoRoot, ...root.split(/[/\\]/).join('/').split('/')),
  label: root.split(/[/\\]/).join('/'),
}))

// 这些文件是"自成体系的色板"，不引用工作台令牌，因此豁免裸色值扫描：
// - shell-theme.css 定义令牌本体；
// - terminalSchemes.ts 是终端配色方案（终端必须能表达任意 ANSI 色，与主题无关）；
// - garden/palette.ts 是花园场景的固定色板（花园不随工作台主题变化，见
//   docs/garden-design.md §4：三套皮肤下必须是同一座花园）。
// 每次运行都会打印豁免项，避免它变成静默的例外。
const tokenFiles = new Set([
  'frontend-shell/src/ui/styles/shell-theme.css',
  'frontend-shell/src/ui/terminalSchemes.ts',
  'frontend-shell/src/ui/garden/palette.ts',
])
const skinFilePattern = /(^|\/)skin-[^/]+\.css$/
const testFilePattern = /\.test\.tsx?$/
const sourceFilePattern = /\.(ts|tsx|css)$/

const hexPattern = /#[0-9a-fA-F]{3,8}\b/g
const colorFunctionPattern = /\b(?:rgba?|hsla?)\(([^)]*)\)/g

// color-mix(in srgb, var(--accent) 20%, transparent) 这类"用令牌派生的半透明变体"不算裸色值：
// 它随令牌走。混入字面色值的写法会被 hex 或 rgb()/hsl() 规则单独抓到。
// 全透明（rgba(0,0,0,0)、rgb(0 0 0 / 0%)）等价于 transparent，不构成主题依赖，放行。
function isFullyTransparent(call) {
  const args = call.slice(call.indexOf('(') + 1, -1)
  const parts = args.includes('/') ? args.split('/') : args.split(',')
  if (parts.length !== 2 && parts.length !== 4) return false
  const alpha = parts[parts.length - 1].trim()
  return alpha === '0' || alpha === '0%' || alpha === '0.0'
}

function* walk(directory) {
  let entries
  try {
    entries = readdirSync(directory, { withFileTypes: true })
  } catch {
    return
  }
  for (const entry of entries) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules') continue
      yield* walk(path)
    } else if (sourceFilePattern.test(entry.name)) {
      yield path
    }
  }
}

const violations = []
const exemptions = []
const deviations = []

// ---- 检查 1：裸色值扫描（同时收集检查 4 要用的尺寸令牌引用）----

const sizeReferencePattern = /var\(\s*(--(?:space|radius|font-size)-[\w-]+)/g
const sizeReferences = []

let scanned = 0
for (const { dir, label } of scanRoots) {
  for (const path of walk(dir)) {
    const rel = relative(repoRoot, path).split(sep).join('/')
    if (testFilePattern.test(rel) || tokenFiles.has(rel) || skinFilePattern.test(rel)) {
      if (!testFilePattern.test(rel)) exemptions.push(rel)
      continue
    }
    scanned += 1
    readFileSync(path, 'utf8')
      .split('\n')
      .forEach((line, index) => {
        const at = `${rel}:${index + 1}`
        for (const match of line.matchAll(hexPattern)) {
          violations.push(`${at} 裸色值 ${match[0]}（应改用 shell-theme.css 中的语义令牌）`)
        }
        for (const match of line.matchAll(colorFunctionPattern)) {
          if (isFullyTransparent(match[0])) continue
          violations.push(`${at} 裸色值 ${match[0]}（应改用 shell-theme.css 中的语义令牌）`)
        }
        for (const match of line.matchAll(sizeReferencePattern)) {
          sizeReferences.push({ at, name: match[1] })
        }
      })
  }
}

// ---- 检查 2：双主题完整性 ----

const isColorish = (name, value) =>
  /shadow|overlay/.test(name) ||
  /rgba?\(|hsla?\(|color-mix\(|#[0-9a-fA-F]{3,8}\b/.test(value) ||
  /^(?:linear|radial|conic)-gradient\(/.test(value)

function tokenBlocks(content) {
  const blocks = { dark: [], light: [] }
  let section = null
  for (const line of content.split('\n')) {
    if (/^:root\s*\{/.test(line)) {
      section = 'dark'
      continue
    }
    if (/^:root\[data-theme="light"\]\s*\{/.test(line)) {
      section = 'light'
      continue
    }
    if (section && line.trim() === '}') {
      section = null
      continue
    }
    if (!section) continue
    const match = line.match(/(--[\w-]+)\s*:\s*([^;]+);/)
    if (!match) continue
    blocks[section].push({ name: match[1], value: match[2].trim(), modeIndependent: /mode-independent/.test(line) })
  }
  return blocks
}

if (tokenFile === 'none') {
  deviations.push('按参数要求跳过双主题完整性检查')
} else {
  const tokenPath = join(repoRoot, ...tokenFile.split('/'))
  let content = null
  try {
    content = readFileSync(tokenPath, 'utf8')
  } catch {
    violations.push(`未找到令牌文件 ${tokenFile}（用 --token-file=none 可显式跳过检查 2）`)
  }
  if (content !== null) {
    const { dark, light } = tokenBlocks(content)
    if (!dark.length || !light.length) {
      violations.push(`${tokenFile} 未解析出 :root 或 :root[data-theme="light"] 令牌块，检查 2 无法执行`)
    }
    const lightNames = new Set(light.map((token) => token.name))
    for (const token of dark) {
      if (!isColorish(token.name, token.value)) continue
      if (token.modeIndependent) {
        exemptions.push(`${tokenFile} ${token.name}（mode-independent）`)
        continue
      }
      if (!lightNames.has(token.name)) {
        violations.push(`${tokenFile} 令牌 ${token.name} 缺少亮色覆写（:root 里是 ${token.value}）`)
      }
    }
    const darkNames = new Set(dark.map((token) => token.name))
    for (const token of light) {
      if (!darkNames.has(token.name)) {
        violations.push(`${tokenFile} 令牌 ${token.name} 只声明在亮色块，暗色下没有取值`)
      }
    }
  }
}

// ---- 检查 3：FlexLayout 第三方调色板映射 ----

const colorVariablesOf = (content) => new Set([...content.matchAll(/(--color-[\w-]+)\s*:/g)].map((match) => match[1]));
const readRepoFile = (relativePath) => {
  try {
    return readFileSync(join(repoRoot, ...relativePath.split('/')), 'utf8')
  } catch {
    return null
  }
};

const libraryContent = readRepoFile(libraryCss)
if (libraryContent === null) {
  deviations.push(`未找到 ${libraryCss}，跳过 FlexLayout 调色板映射检查（依赖未安装时属正常）`)
} else {
  const mappingContent = readRepoFile(mappingFile)
  if (mappingContent === null) {
    violations.push(`未找到 ${mappingFile}：FlexLayout 调色板映射文件缺失`)
  } else {
    const mapped = colorVariablesOf(mappingContent);
    for (const name of colorVariablesOf(libraryContent)) {
      if (!mapped.has(name)) {
        violations.push(`${mappingFile} 未映射库变量 ${name}（库新增或改名后必须同步映射，否则该面会漏回库自带色值）`)
      }
    }
  }
  for (const entry of flexlayoutEntries) {
    const content = readRepoFile(entry)
    if (content === null) continue
    if (content.includes('flexlayout-react/style/dark.css')) {
      violations.push(`${entry} 仍引入 flexlayout-react/style/dark.css（第三方暗色样式会在亮色下留下未覆盖的暗色面）`)
    }
    if (!content.includes('flexlayout-react/style/light.css')) {
      violations.push(`${entry} 未引入 flexlayout-react/style/light.css（FlexLayout 的结构基座）`)
    }
  }
}

// ---- 检查 4：尺寸令牌引用完整性 ----

const sizeTokenDeclarationPattern = /(--(?:space|radius|font-size)-[\w-]+)\s*:/g

if (tokenFile === 'none') {
  deviations.push('按参数要求跳过尺寸令牌引用检查')
} else {
  let tokenContent = null
  try {
    tokenContent = readFileSync(join(repoRoot, ...tokenFile.split('/')), 'utf8')
  } catch {
    // 令牌文件缺失由检查 2 报出，这里不重复报
  }
  if (tokenContent !== null) {
    const declared = new Set([...tokenContent.matchAll(sizeTokenDeclarationPattern)].map((match) => match[1]))
    for (const reference of sizeReferences) {
      if (!declared.has(reference.name)) {
        violations.push(`${reference.at} 引用了未声明的尺寸令牌 ${reference.name}（自定义属性解析失败会静默丢掉整条声明，属性回落初始值）`)
      }
    }
    for (const token of tokenBlocks(tokenContent).light) {
      if (/^--(?:space|radius|font-size)-/.test(token.name)) {
        violations.push(`${tokenFile} 尺寸令牌 ${token.name} 声明在亮色块：尺寸与明暗无关，只在 :root 声明一次`)
      }
    }
  }
}

// ---- 输出 ----

if (exemptions.length) {
  console.log(`样式门禁豁免项（${exemptions.length}）：\n${exemptions.map((item) => `  ${item}`).join('\n')}`)
}
for (const note of deviations) console.log(`样式门禁提示：${note}`)

if (violations.length) {
  console.error(`样式门禁失败，发现 ${violations.length} 处问题（已扫描 ${scanned} 个文件）：\n${violations.join('\n')}`)
  process.exit(1)
}
console.log(`样式门禁通过：已扫描 ${scanned} 个文件，${scanRoots.map((root) => root.label).join(' 与 ')} 无裸色值，令牌双主题完整。`)
