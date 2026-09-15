import type { GardenPack } from '../../pack'
import dayScene from './assets/day.webp'
import nightScene from './assets/night.webp'
import fernImage from './assets/fern.webp'
import mintImage from './assets/mint.webp'
import orchidImage from './assets/orchid.webp'
import orchidShinyImage from './assets/orchid-shiny.webp'

/**
 * 官方绘本画法包（image 画法，题材=植物图鉴）。
 *
 * 资源形状题材无关（docs/garden-presentation-architecture.md）：每个物种按 6 个形态
 * 阶段各给一张图、可选闪光替代图与成熟姿态变体、一个落地锚点——动物/建筑题材
 * 替换的是这里的资源与文案，运行时与状态层零改动。
 *
 * 现状：薄荷/蕨/兰三已有原画（各阶段暂用同一张成熟体占位，九宫格素材到位后按下标
 * 填充）；杉/藤/树未覆盖 → 场景层自动回退内置 SVG 画法（混合渲染是过渡态，可接受）。
 */
export const botanicalPack: GardenPack = {
  id: 'botanical-image',
  name: '植物图鉴（绘本）',
  author: 'OpsCopilot',
  art: 'image',
  schemaVersions: '1',
  species: {
    'session-tree': { name: '会话杉', meaning: '稳定连接与持续值守' },
    'cmd-mint': { name: '命令薄荷', meaning: '把重复工作变得轻巧' },
    'script-vine': { name: '脚本藤', meaning: '自动化能力持续延展' },
    'transfer-fern': { name: '传输蕨', meaning: '信息在环境间安全流动' },
    'guard-orchid': { name: '守护兰', meaning: '克制、边界与安全操作' },
    'knowledge-tree': { name: '知识树', meaning: '把经验沉淀为可复用能力' },
  },
  stageLabels: ['发芽', '幼苗', '分枝', '繁茂', '开花', '成熟'],
  images: {
    scene: { day: dayScene, night: nightScene },
    species: {
      'cmd-mint': {
        stages: [mintImage, mintImage, mintImage, mintImage, mintImage, mintImage],
        anchor: { x: 50, y: 90 },
      },
      'transfer-fern': {
        stages: [fernImage, fernImage, fernImage, fernImage, fernImage, fernImage],
        anchor: { x: 50, y: 83 },
      },
      'guard-orchid': {
        stages: [orchidImage, orchidImage, orchidImage, orchidImage, orchidImage, orchidImage],
        shinyStages: [orchidShinyImage, orchidShinyImage, orchidShinyImage, orchidShinyImage, orchidShinyImage, orchidShinyImage],
        anchor: { x: 50, y: 93 },
      },
    },
  },
}
