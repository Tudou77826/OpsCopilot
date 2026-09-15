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
 * 填充）；杉/藤/树尚无素材，保留收藏详情入口，不在绘本场景中混用 SVG。
 */
export const botanicalPack: GardenPack = {
  id: 'botanical-image',
  name: '植物图鉴（绘本）',
  author: 'OpsCopilot',
  art: 'image',
  schemaVersions: '1',
  species: {
    'session-tree': {
      name: '会话杉', latin: 'Picea connexa', meaning: '稳定连接与持续值守',
      description: '挺拔的针叶从不停歇生长，像一条始终在线的会话，安静地守着每一次登录与值守。',
    },
    'cmd-mint': {
      name: '命令薄荷', latin: 'Mentha', meaning: '把重复工作变得轻巧',
      description: '一簇清新的绿意，记录那些被认真整理、反复复用的快捷命令。',
    },
    'script-vine': {
      name: '脚本藤', latin: 'Vitis scripta', meaning: '自动化能力持续延展',
      description: '蜿蜒的藤蔓沿着支架不断伸展，每一片新叶都是一段被固化的自动化流程。',
    },
    'transfer-fern': {
      name: '传输蕨', latin: 'Pteridophyta', meaning: '信息在环境间安全流动',
      description: '舒展的羽叶与卷曲的新芽，记录每一次可靠完成的文件传输。',
    },
    'guard-orchid': {
      name: '守护兰', latin: 'Orchidaceae', meaning: '克制、边界与安全操作',
      description: '淡紫花瓣沿着花茎舒展，为认真守护操作边界的你而绽放。',
    },
    'knowledge-tree': {
      name: '知识树', latin: 'Arbor scientiae', meaning: '把经验沉淀为可复用能力',
      description: '年轮一圈圈生长，把排查过的故障与沉淀的答案都收进树冠里。',
    },
  },
  stageLabels: ['发芽', '幼苗', '分枝', '繁茂', '开花', '成熟'],
  presentation: { title: '花园', unit: '株', empty: '建立一次连接、完成一次传输或跑完一次脚本，让这里长出第一株绿意。', maxLevel: '常青' },
  images: {
    scene: { day: dayScene, night: nightScene },
    layout: { aspectRatio: 3, slots: [{ x: 19, y: 86, size: .76 }, { x: 51, y: 73, size: .79 }, { x: 85, y: 91, size: .9 }], nightBrightness: .58, nightSaturation: .65, sway: true },
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
