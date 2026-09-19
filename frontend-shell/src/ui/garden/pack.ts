import { validateGardenPack, type GardenPack } from './contentPack'

export * from './contentPack'

/** 状态快照里的一个养成实例（与 Go 侧 pkg/garden.Specimen 的 JSON 形状一致）。 */
export interface GardenSpecimen {
  itemId: string
  instanceId: string
  level: number
  xp: number
  shiny: boolean
  acquiredAt: string
  lastGrewAt: string
  milestones?: string[]
  placement: { placed: boolean; x: number; y: number; scale: number; flipX: boolean }
}

/** 入口轻提示使用的可合并变化游标，不代表未读或待领取任务。 */
export interface GardenChangeSignal {
  revision: number
  kind: 'currency-earned'
  amount?: number
  instanceId?: string
  itemId?: string
  at: string
}

export interface GardenSnapshot {
  schemaVersion: number
  ruleVersion: number
  gardenLevel: number
  balance: number
  earned: number
  spent: number
  specimens: GardenSpecimen[] | null
  changeSignal: GardenChangeSignal | null
}

export const MAX_LEVEL = 100
export const STAGE_PER_LEVEL = 10
export const NUM_STAGES = 6
export const FINAL_STAGE_LEVEL = 50

export function stageOf(level: number): number {
  if (level < 0) return 0
  return Math.min(Math.floor(level / STAGE_PER_LEVEL), NUM_STAGES - 1)
}

export const DEFAULT_STAGE_LABELS = ['阶段一', '阶段二', '阶段三', '阶段四', '阶段五', '最终形态']

export function stageLabel(pack: GardenPack | undefined, stage: number, itemId?: string): string {
  const labels = (itemId ? pack?.elements[itemId]?.stageLabels : undefined) ?? pack?.stageLabels ?? DEFAULT_STAGE_LABELS
  const index = Math.max(0, Math.min(stage, NUM_STAGES - 1))
  return labels[index] ?? DEFAULT_STAGE_LABELS[index]
}

function hashMod(text: string, mod: number): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0) % Math.max(mod, 1)
}

export interface ResolvedGardenVisual {
  src: string
  assetId: string
  anchor: { x: number; y: number }
  hitArea: { x: number; y: number; width: number; height: number }
  defaultSize: number
}

/**
 * 内容包只通过资源 id 描述形象；运行时统一解析为可显示资源。
 * 未声明的元素或缺失资源明确返回 undefined，不跨包回退到其他题材。
 */
export function resolveSpecimenVisual(
  pack: GardenPack | undefined,
  itemId: string,
  level: number,
  shiny: boolean,
  instanceId: string,
): ResolvedGardenVisual | undefined {
  const element = pack?.elements[itemId]
  if (!pack || !element) return undefined
  const stage = stageOf(level)
  let assetId = shiny ? element.shinyStages?.[stage] : undefined
  if (!assetId) assetId = element.stages[stage]
  const variants = shiny ? element.shinyMatureVariants : element.matureVariants
  if (stage === NUM_STAGES - 1 && variants?.length) assetId = variants[hashMod(instanceId, variants.length)]
  const asset = assetId ? pack.assets[assetId] : undefined
  if (!asset) return undefined
  return { src: asset.src, assetId, anchor: element.anchor, hitArea: element.hitArea, defaultSize: element.defaultSize * (element.stageScales?.[stage] ?? 1) }
}

export function packSupportsSnapshot(pack: GardenPack, snapshot: GardenSnapshot): boolean {
  return snapshot.schemaVersion >= pack.stateSchema.min && snapshot.schemaVersion <= pack.stateSchema.max
}

const packs = new Map<string, GardenPack>()

export function registerPack(pack: GardenPack): () => void {
  validateGardenPack(pack)
  const existing = packs.get(pack.id)
  if (existing && existing !== pack) throw new Error(`内容包 ${pack.id} 已登记，不能被静默覆盖`)
  packs.set(pack.id, pack)
  return () => {
    if (packs.get(pack.id) === pack) packs.delete(pack.id)
  }
}

export function getPack(id: string): GardenPack | undefined {
  return packs.get(id)
}

export function listPacks(): GardenPack[] {
  return [...packs.values()]
}

export const DEFAULT_PACK_ID = 'ops-woodland-courtyard'
