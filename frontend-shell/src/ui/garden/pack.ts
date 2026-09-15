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

export interface GardenPack {
  id: string
  name: string
  author?: string
  art: GardenArt
  /** 支持的快照 schemaVersion 区间，闭区间。 */
  schemaVersions: string
  /** 物种语义位 → 该包的形象与文案。状态层不感知这里的任何内容。 */
  species: Record<string, GardenPackSpecies>
}

/** 快照里的等级上限与阶段数，与 Go 侧常量镜像；呈现只读不写。 */
export const MAX_LEVEL = 30
export const STAGE_PER_LEVEL = 5
export const NUM_STAGES = 6

/** 由等级推出形态阶段（0–5），与 pkg/garden.StageOf 保持一致。 */
export function stageOf(level: number): number {
  if (level < 1) return 0
  return Math.min(Math.floor((level - 1) / STAGE_PER_LEVEL), NUM_STAGES - 1)
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

/** 默认包 id：官方参数化花卉包。 */
export const DEFAULT_PACK_ID = 'flora-svg'
