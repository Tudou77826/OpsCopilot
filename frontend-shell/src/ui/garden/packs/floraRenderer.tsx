import React from 'react'
import { stageOf, NUM_STAGES, MAX_LEVEL } from '../pack'
import { gardenScene as palette } from '../palette'

/**
 * 参数化分层 SVG 渲染器（官方默认画法的实现）。
 *
 * 同一份几何按 (阶段, 等级内进度, 闪光) 组合出全部视觉状态，因此 6 档形态 × 30 级
 * 不需要 150 张图。画法是"运行时提供的渲染器"，包只提供几何参数——这是
 * docs/garden-presentation-architecture.md 4.2 节里 svg 画法的落地方式。
 *
 * 花园使用固定色板，不随工作台主题/皮肤变化（garden-design.md §4）：
 * 场景是收藏的一部分，桌面暗色、桌面亮色和 Teams 里看到的必须是同一座花园。
 */


/** 每个物种的形态基调：换题材时只改这里（动物/建筑同理）。 */
type SpeciesShape = {
  /** 主茎高度系数（阶段越高越高的基线）。 */
  height: number
  /** 叶/枝的横向展开系数。 */
  spread: number
  /** 花/果的颜色。 */
  accent: string
  /** 成熟期是否结果。 */
  fruit: boolean
}

const shapes: Record<string, SpeciesShape> = {
  'session-tree': { height: 1.35, spread: 1.0, accent: palette.bloom, fruit: false },
  'cmd-mint': { height: 0.8, spread: 1.2, accent: palette.leaf, fruit: false },
  'script-vine': { height: 1.1, spread: 1.45, accent: palette.bloom, fruit: false },
  'transfer-fern': { height: 0.95, spread: 1.3, accent: palette.transferFrond, fruit: false },
  'guard-orchid': { height: 1.0, spread: 0.9, accent: palette.guardPetal, fruit: false },
  'knowledge-tree': { height: 1.25, spread: 1.15, accent: palette.knowledgeCanopy, fruit: true },
}

const fallbackShape: SpeciesShape = { height: 1, spread: 1, accent: palette.bloom, fruit: false }

/** 阶段名：用于详情与无障碍标签。 */
export const stageNames = ['发芽', '幼苗', '分枝', '繁茂', '开花', '成熟']

export interface SpecimenVisualProps {
  speciesId: string
  level: number
  shiny: boolean
  /** 画布尺寸；场景自适应时由调用方给出。 */
  size?: number
  /** 无障碍名称（形如 "会话杉 7 级 幼苗 闪光"）。由调用方拼好，渲染器只负责挂上。 */
  label?: string
}

/**
 * 由 (阶段, 等级内进度) 推出几何参数。
 * 等级内进度 = 当前等级在本阶段内的相对位置，用来做"细微变化"（新叶、枝条、花苞）。
 */
export function visualParams(level: number, shiny: boolean) {
  const stage = stageOf(level)
  const levelsIntoStage = (level - 1) % 5
  const growth = stage / (NUM_STAGES - 1)            // 0..1 形态进度
  const within = levelsIntoStage / 5                 // 0..1 阶段内进度
  const evergreen = level >= MAX_LEVEL
  return { stage, growth, within, evergreen, shiny }
}

/** 主渲染组件：一个物种实例的画。 */
export default function FloraSpecimen({ speciesId, level, shiny, size = 72, label }: SpecimenVisualProps) {
  const shape = shapes[speciesId] ?? fallbackShape
  const { stage, growth, within, evergreen } = visualParams(level, shiny)
  const h = (14 + growth * 30) * shape.height * (1 + within * 0.08)
  const spread = (6 + growth * 22) * shape.spread
  const stemW = 1.6 + growth * 1.4
  const leafCount = stage >= 1 ? Math.min(2 + stage, 6) : 0
  const hasBloom = stage >= 4
  const hasFruit = shape.fruit && stage >= 5
  const baseY = size - 10

  const leaves = Array.from({ length: leafCount }, (_, index) => {
    const side = index % 2 === 0 ? -1 : 1
    const y = baseY - (h * (0.3 + index * 0.12))
    const len = spread * (0.55 + index * 0.05)
    return (
      <ellipse
        key={`leaf-${index}`}
        cx={size / 2 + side * len * 0.5}
        cy={y}
        rx={len * 0.5}
        ry={len * 0.22}
        fill={index % 3 === 0 ? palette.leafDeep : palette.leaf}
        transform={`rotate(${side * -18} ${size / 2 + side * len * 0.5} ${y})`}
      />
    )
  })

  return (
    <svg role="img" aria-label={label} width={size} height={size} viewBox={`0 0 ${size} ${size}`} data-stage={stage} data-shiny={shiny ? 'true' : 'false'}>
      {shiny ? <circle cx={size / 2} cy={baseY - h * 0.6} r={spread * 0.75} fill={palette.shinyGlow} opacity={0.35} /> : null}
      {/* 花盆/土地：固定的场景元素，让 6 个阶段都有稳定的落脚点 */}
      <ellipse cx={size / 2} cy={baseY + 4} rx={spread * 0.55 + 8} ry={4} fill={palette.soilDark} />
      <path d={`M ${size / 2 - spread * 0.5 - 6} ${baseY} h ${spread + 12} l -3 9 h ${-(spread + 6)} z`} fill={palette.pot} />
      {/* 主茎 */}
      <path d={`M ${size / 2} ${baseY} C ${size / 2 - 2} ${baseY - h * 0.5}, ${size / 2 + 2} ${baseY - h * 0.7}, ${size / 2} ${baseY - h}`} stroke={stage <= 1 ? palette.stemYoung : palette.stem} strokeWidth={stemW} fill="none" strokeLinecap="round" />
      {leaves}
      {hasBloom ? (
        <g>
          {[0, 72, 144, 216, 288].map(deg => (
            <ellipse
              key={`petal-${deg}`}
              cx={size / 2}
              cy={baseY - h - 5}
              rx={4.2}
              ry={7}
              fill={shape.accent}
              transform={`rotate(${deg} ${size / 2} ${baseY - h - 5})`}
            />
          ))}
          <circle cx={size / 2} cy={baseY - h - 5} r={2.6} fill={palette.bloomCore} />
        </g>
      ) : null}
      {hasFruit ? <circle cx={size / 2 + spread * 0.35} cy={baseY - h * 0.55} r={3.2} fill={palette.fruit} /> : null}
      {evergreen ? <circle cx={size / 2 - spread * 0.3} cy={baseY - h * 0.75} r={2.2} fill={palette.bloomCore} /> : null}
    </svg>
  )
}
