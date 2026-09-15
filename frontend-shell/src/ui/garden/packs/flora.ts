import type { GardenPack } from '../pack'

/**
 * 官方默认呈现包：参数化分层 SVG。
 *
 * 为什么是"参数化"而不是 150 张图（docs/garden-design.md §16.1 的建议）：
 * 6 个阶段 × 5 档等级内变化用图层开合 + 尺寸/颜色插值表达，一个物种只需一份几何。
 * 换题材（动物/建筑）时替换本文件的几何即可，状态层与运行时都不用改。
 *
 * 包内不含可执行内容：这里只是资源与清单，渲染由运行时统一执行。
 */
export const floraPack: GardenPack = {
  id: 'flora-svg',
  name: '花园（默认）',
  author: 'OpsCopilot',
  art: 'svg',
  schemaVersions: '1',
  species: {
    'session-tree': { name: '会话杉', meaning: '稳定连接与持续值守' },
    'cmd-mint': { name: '命令薄荷', meaning: '把重复工作变得轻巧' },
    'script-vine': { name: '脚本藤', meaning: '自动化能力持续延展' },
    'transfer-fern': { name: '传输蕨', meaning: '信息在环境间安全流动' },
    'guard-orchid': { name: '守护兰', meaning: '克制、边界与安全操作' },
    'knowledge-tree': { name: '知识树', meaning: '把经验沉淀为可复用能力' },
  },
}
