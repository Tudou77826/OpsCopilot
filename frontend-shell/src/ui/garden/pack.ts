// 养成呈现运行时：读取状态快照（无呈现语义），交给一个"呈现包"渲染。
//
// 架构约束（docs/garden-presentation-architecture.md）：
// - 呈现层是状态的纯函数：同一份快照 + 同一个包 = 同一个画面，不依赖隐藏状态；
// - 题材与画法都是资源内容，不是代码。建筑包、动物包与植物包走同一份清单格式；
// - 包内不含可执行内容（清单 + 资源 + 可选局部样式），自定义能力的安全边界在此。

/** 状态快照里的一个养成实例（与 Go 侧 pkg/garden.Specimen 的 JSON 形状一致）。 */
export interface GardenSpecimen {
  speciesId: string
  instanceId: string
  level: number
  xp: number
  shiny: boolean
  acquiredAt: string
  lastGrewAt: string
  milestones?: string[]
  slotId?: string
}

/** 未领取的反馈（发现/升级/溢出）。 */
export interface GardenPending {
  kind: string
  instanceId?: string
  speciesId?: string
  level?: number
  at: string
}

export interface GardenSnapshot {
  schemaVersion: number
  ruleVersion: number
  gardenLevel: number
  specimens: GardenSpecimen[] | null
  pitySinceShiny: number
  pending: GardenPending[] | null
}

/** 画法类型。第一版内建四种；包只能声明其中之一，不能自带渲染代码。 */
export type GardenArt = 'svg' | 'pixel' | 'glyph' | 'image'

export interface GardenPackSpecies {
  /** 展示名（包可改；状态层只有稳定的 speciesId）。 */
  name: string
  /** 该物种代表的行为含义。 */
  meaning: string
}

/**
 * image 画法的单物种资源。形状刻意**题材无关**：植物（幼苗→开花）、动物（幼崽→成年）、
 * 建筑（地基→落成）都是同一条数据——"每个形态阶段一张图 + 闪光替代图 + 落地锚点"。
 * 状态层与场景层不解读图片内容，只按 (speciesId, stage, shiny, 姿态序号) 取图。
 */
export interface GardenImageSpecies {
  /** 与 NUM_STAGES 等长的阶段图列表；空串表示该阶段缺图 → 回退内置 SVG 画法。 */
  stages: string[]
  /** 闪光替代图（可选）：与 stages 按下标对齐；缺省用普通图。 */
  shinyStages?: string[]
  /** 最高阶段的姿态变体（可选，纯装饰不加等级）；按 instanceId 确定性挑选。 */
  matureVariants?: string[]
  /** 落地锚点：图片内的横向/纵向百分比（根/地基所在位置），默认 50% / 88%。 */
  anchor?: { x: number; y: number }
}

export interface GardenPack {
  id: string
  name: string
  author?: string
  art: GardenArt
  /** 支持的快照 schemaVersion 区间，闭区间。 */
  schemaVersions: string
  /** 物种语义位 → 该包的形象与文案。状态层不感知这里的任何内容。 */
  species: Record<string, GardenPackSpecies>
  /** 形态阶段标签（题材语言：发芽…成熟 / 地基…落成），缺省用内置词表。 */
  stageLabels?: string[]
  /**
   * image 画法的资源清单（其余画法忽略）。scene 是题材自有的场景背景
   * （花园/草原/城市天际线……）；species 未覆盖的物种回退内置 SVG 画法。
   */
  images?: {
    scene?: { day?: string; night?: string }
    species: Record<string, GardenImageSpecies>
  }
}

/** 快照里的等级上限与阶段数，与 Go 侧常量镜像；呈现只读不写。 */
export const MAX_LEVEL = 30
export const STAGE_PER_LEVEL = 5
export const NUM_STAGES = 6
/** 闪光保底的合格事件计数，与 Go 侧 garden.PityAt 镜像（头部进度条用）。 */
export const PITY_AT = 40

/** 由等级推出形态阶段（0–5），与 pkg/garden.StageOf 保持一致。 */
export function stageOf(level: number): number {
  if (level < 1) return 0
  return Math.min(Math.floor((level - 1) / STAGE_PER_LEVEL), NUM_STAGES - 1)
}

/** 缺省形态阶段词表（植物语汇）；包可用 stageLabels 换成自己的题材语言。 */
export const DEFAULT_STAGE_LABELS = ['发芽', '幼苗', '分枝', '繁茂', '开花', '成熟']

/** 包内的阶段标签；越界/缺省回退缺省词表。 */
export function stageLabel(pack: GardenPack | undefined, stage: number): string {
  const labels = pack?.stageLabels ?? DEFAULT_STAGE_LABELS
  return labels[Math.max(0, Math.min(stage, labels.length - 1))] ?? DEFAULT_STAGE_LABELS[stage]
}

/** FNV-1a → [0, mod)：成熟姿态等"按株确定性挑选"用。 */
function hashMod(text: string, mod: number): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0) % Math.max(mod, 1)
}

/**
 * 取某株在包里的图片资源；物种未被包覆盖或该阶段缺图时返回 undefined
 * （场景层回退内置 SVG 画法）。闪光优先取 shinyStages，缺位回退普通图。
 * 最高阶段且有 matureVariants 时按 instanceId 确定性挑一张姿态（同一株永不变脸）。
 */
export function resolveSpecimenImage(
  pack: GardenPack | undefined,
  speciesId: string,
  level: number,
  shiny: boolean,
  instanceId: string,
): { src: string; anchor: { x: number; y: number } } | undefined {
  const entry = pack?.images?.species[speciesId]
  if (!entry) return undefined
  const stage = stageOf(level)
  const at = (list: string[] | undefined, index: number): string => (list && list[index]) || ''
  let src = shiny ? at(entry.shinyStages, stage) : ''
  if (!src) src = at(entry.stages, stage)
  if (!src) return undefined
  if (stage === NUM_STAGES - 1 && entry.matureVariants && entry.matureVariants.length > 0) {
    src = entry.matureVariants[hashMod(instanceId, entry.matureVariants.length)]
  }
  return { src, anchor: entry.anchor ?? { x: 50, y: 88 } }
}

/** 快照是否与包声明的版本区间兼容。不兼容时前台显示错误而不是画错形象。 */
export function packSupportsSnapshot(pack: GardenPack, snapshot: GardenSnapshot): boolean {
  const [min, max] = pack.schemaVersions.split('-').map(value => Number(value.trim()))
  if (!Number.isFinite(min)) return false
  const upper = Number.isFinite(max) ? max : min
  return snapshot.schemaVersion >= min && snapshot.schemaVersion <= upper
}

/** 内置包注册表。阶段一只有官方默认包；阶段三再加用户数据目录里的本地包。 */
const packs = new Map<string, GardenPack>()

export function registerPack(pack: GardenPack): void {
  packs.set(pack.id, pack)
}

export function getPack(id: string): GardenPack | undefined {
  return packs.get(id)
}

export function listPacks(): GardenPack[] {
  return [...packs.values()]
}

/** 默认包 id：官方绘本画法包（PNG 素材，题材=植物图鉴）。 */
export const DEFAULT_PACK_ID = 'botanical-image'
