// 从 iCode Teams 仓库提取 --ui-* 契约的名字快照，供外观契约测试断言"皮肤引用的宿主变量都存在"。
//
// 用法（宿主仓库不在本仓库内，所以路径必须显式给）：
//   ICODE_TEAMS_REPO=/path/to/icode-teams node tools/checks/extract-teams-host-contract.mjs
//   node tools/checks/extract-teams-host-contract.mjs --host-repo=/path --revision=origin/main
//
// 宿主改名或删变量后重跑本脚本会更新快照，随后契约测试会指出映射里哪些名字已失效；
// 反向（宿主新增变量）不会失败，正是想要的行为——新增是否要映射由人决定。
import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'

const repoRoot = join(import.meta.dirname, '..', '..')
const output = join(repoRoot, 'frontend-shell', 'src', 'ui', 'styles', 'teams-host-contract.json')

const args = process.argv.slice(2)
const argument = (name, fallback) => {
  const found = args.find((value) => value.startsWith(`--${name}=`))
  return found ? found.slice(name.length + 3) : fallback
}

const hostRepo = argument('host-repo', process.env.ICODE_TEAMS_REPO)
const revision = argument('revision', 'origin/main')
const hostFile = 'src/client/ui/design-system.ts'

if (!hostRepo) {
  console.error('缺少宿主仓库路径：用 --host-repo=<path> 或环境变量 ICODE_TEAMS_REPO 指定。')
  process.exit(1)
}

const git = (gitArgs) => execFileSync('git', gitArgs, { cwd: hostRepo, encoding: 'utf8' })

let revisionHash
let text
try {
  revisionHash = git(['rev-parse', revision]).trim()
  text = git(['show', `${revision}:${hostFile}`])
} catch (error) {
  console.error(`读取 ${hostRepo} 的 ${revision}:${hostFile} 失败：${error.message}`)
  process.exit(1)
}

// 该文件是一份模板字符串，内含 :root{} 与 :root[data-theme='dark']{} 两段，
// 所以直接对整份文本提取变量名，而不是解析某个 :root 块。
const names = [...new Set([...text.matchAll(/--ui-[a-zA-Z0-9-]+/g)].map((match) => match[0]))].sort()

const snapshot = {
  _note: 'iCode Teams 宿主 --ui-* 契约的名字快照。皮肤块的每个 var(--ui-*) 引用都必须在这里；宿主改名或删变量时重跑提取脚本会让契约测试失败，从而提醒我们同步映射。',
  host: 'icode-teams',
  hostFile,
  hostRevision: revisionHash,
  updateCommand: 'ICODE_TEAMS_REPO=<宿主仓库路径> node tools/checks/extract-teams-host-contract.mjs',
  count: names.length,
  variables: names,
}
writeFileSync(output, JSON.stringify(snapshot, null, 2) + '\n')
console.log(`已写入 ${names.length} 个变量名（${revision} = ${revisionHash.slice(0, 12)}）`)
