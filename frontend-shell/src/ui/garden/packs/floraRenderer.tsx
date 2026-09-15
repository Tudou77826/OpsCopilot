import React, { useId } from 'react'
import { stageOf, NUM_STAGES, MAX_LEVEL } from '../pack'
import { gardenScene as palette } from '../palette'

/**
 * 参数化分层 SVG 渲染器（官方默认画法的实现）。
 *
 * 同一份几何按 (阶段, 等级内进度, 闪光) 组合出全部视觉状态，因此 6 档形态 × 30 级
 * 不需要 150 张图。画法是"运行时提供的渲染器"，包只提供几何参数——这是
 * docs/garden-presentation-architecture.md 4.2 节里 svg 画法的落地方式。
 *
 * 坐标约定：全部几何画在 72×72 的基准盒里（落地线 y=62），渲染时整体缩放到
 * size——场景里的巨株与抽屉里的小株共用同一份几何，比例恒定。
 * spread 的量纲是基准盒像素，约 [6..28]，随生长进度增大。
 * 每个物种有独立的形态画法（杉/薄荷/藤/蕨/兰/树），阶段驱动枝叶花果的数量。
 * 植株本体用固定美术色板（palette.ts），昼夜通用；闪光株自带光晕与闪星。
 */

/** 两种落地形态：potted 带花盆（详情抽屉），grounded 直接种在场景地里。 */
export type SpecimenVariant = 'potted' | 'grounded'

/** 每个物种的形态基调：换题材时只改这里（动物/建筑同理）。 */
type SpeciesShape = {
  /** 主茎高度系数（阶段越高越高的基线）。 */
  height: number
  /** 叶/枝的横向展开系数（基准盒像素，约 [6..28]）。 */
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

/** 四角闪星路径（以原点为中心）；外层 g 负责定位，CSS 类负责缩放旋转动画。 */
const SPARKLE_PATH = 'M0,-4.6 L1.15,-1.15 L4.6,0 L1.15,1.15 L0,4.6 L-1.15,1.15 L-4.6,0 L-1.15,-1.15 Z'

/** 闪光株的环绕闪星位置（基准盒坐标，相对落地冠层中心），节奏各不相同。 */
const sparkles = [
  { x: -24, y: -30, s: 1.0, delay: '0s' },
  { x: 22, y: -36, s: 0.72, delay: '0.7s' },
  { x: 5, y: -44, s: 0.55, delay: '1.4s' },
  { x: -14, y: -18, s: 0.62, delay: '1.9s' },
]

/** 画笔：几何都在 72 基准盒内，cx=36，baseY=62；h 约[14..59]，spread 约[6..28]。 */
interface Brush {
  cx: number
  baseY: number
  h: number
  spread: number
  stage: number
  within: number
  evergreen: boolean
  shape: SpeciesShape
}

/** 会话杉：主干 + 层叠三角冠（杉树剪影），成熟期挂果苞。 */
function drawFir({ cx, baseY, h, spread, stage, evergreen }: Brush): React.ReactNode {
  const barkH = h * 0.3
  const tiers = Math.min(2 + stage, 5)
  const parts: React.ReactNode[] = []
  parts.push(<path key="bark" d={`M ${cx - 2} ${baseY} L ${cx + 2} ${baseY} L ${cx + 1.2} ${baseY - barkH} L ${cx - 1.2} ${baseY - barkH} Z`} fill={palette.pot} />)
  for (let i = 0; i < tiers; i++) {
    const tierY = baseY - barkH + 3 - (h * 0.52) * (i / tiers)
    const apexY = tierY - h * (0.68 / tiers) - 2
    const halfW = spread * (0.95 - i * 0.13)
    parts.push(<path key={`tier-${i}`} d={`M ${cx - halfW} ${tierY} L ${cx + halfW} ${tierY} L ${cx} ${apexY} Z`} fill={i % 2 === 0 ? palette.leaf : palette.leafDeep} />)
  }
  if (stage >= 5) {
    parts.push(<circle key="cone1" cx={cx + spread * 0.18} cy={baseY - barkH - h * 0.2} r={1.8} fill={palette.bloomCore} />)
    parts.push(<circle key="cone2" cx={cx - spread * 0.22} cy={baseY - barkH - h * 0.34} r={1.6} fill={palette.bloomCore} />)
  }
  if (evergreen) parts.push(<circle key="ever" cx={cx} cy={baseY - barkH - h * 0.62} r={2.4} fill={palette.bloomCore} />)
  return parts
}

/** 命令薄荷：一丛扇形茎 + 对生圆叶，开花期茎顶出现淡紫色小花穗。 */
function drawMint({ cx, baseY, h, spread, stage, evergreen }: Brush): React.ReactNode {
  const stems = 3 + (stage >= 3 ? 2 : 0)
  const parts: React.ReactNode[] = []
  for (let i = 0; i < stems; i++) {
    const angle = (i - (stems - 1) / 2) * 0.42
    const tipX = cx + Math.sin(angle) * spread * 0.62
    const tipY = baseY - h * (0.72 + 0.16 * (1 - Math.abs(angle)))
    const midX = cx + Math.sin(angle) * spread * 0.24
    const midY = baseY - h * 0.4
    parts.push(<path key={`stem-${i}`} d={`M ${cx} ${baseY} Q ${midX} ${midY} ${tipX} ${tipY}`} stroke={palette.stem} strokeWidth={1.7} fill="none" strokeLinecap="round" />)
    const leafPairs = Math.min(1 + stage, 4)
    for (let l = 0; l < leafPairs; l++) {
      const t = 0.32 + l * (0.55 / leafPairs)
      const lx = cx + (tipX - cx) * t
      const ly = baseY + (tipY - baseY) * t
      const side = l % 2 === 0 ? -1 : 1
      parts.push(<ellipse key={`leaf-${i}-${l}`} cx={lx + side * 4.2} cy={ly} rx={4.2} ry={2.8} fill={l % 2 === 0 ? palette.leaf : palette.leafDeep} transform={`rotate(${side * -24} ${lx + side * 4.2} ${ly})`} />)
    }
    if (stage >= 4) {
      for (let d = 0; d < 3; d++) {
        parts.push(<circle key={`fl-${i}-${d}`} cx={tipX + (d - 1) * 2.2} cy={tipY - 2 - (d === 1 ? 2.4 : 0)} r={1.5} fill={palette.guardPetal} opacity={0.9} />)
      }
    }
  }
  if (evergreen) parts.push(<circle key="ever" cx={cx} cy={baseY - h - 6} r={2.2} fill={palette.bloomCore} />)
  return parts
}

/** 三次贝塞尔取点（藤蔓的叶/花沿茎布置用）。 */
function cubicAt(t: number, p0: number, p1: number, p2: number, p3: number): number {
  const u = 1 - t
  return u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3
}

/** 脚本藤：S 形缠绕茎 + 卷须 + 沿茎叶与花，成熟期沿茎开花。 */
function drawVine({ cx, baseY, h, spread, stage, evergreen, shape }: Brush): React.ReactNode {
  const p0x = cx, p0y = baseY
  const p1x = cx - spread * 0.5, p1y = baseY - h * 0.38
  const p2x = cx + spread * 0.56, p2y = baseY - h * 0.72
  const p3x = cx - spread * 0.14, p3y = baseY - h
  const parts: React.ReactNode[] = []
  parts.push(<path key="stem" d={`M ${p0x} ${p0y} C ${p1x} ${p1y}, ${p2x} ${p2y}, ${p3x} ${p3y}`} stroke={palette.stem} strokeWidth={2} fill="none" strokeLinecap="round" />)
  const leafCount = Math.min(2 + stage, 6)
  for (let i = 0; i < leafCount; i++) {
    const t = 0.18 + i * (0.68 / leafCount)
    const lx = cubicAt(t, p0x, p1x, p2x, p3x)
    const ly = cubicAt(t, p0y, p1y, p2y, p3y)
    const side = i % 2 === 0 ? -1 : 1
    parts.push(<ellipse key={`leaf-${i}`} cx={lx + side * 4.8} cy={ly - 1} rx={4.8} ry={2.9} fill={i % 3 === 0 ? palette.leafDeep : palette.leaf} transform={`rotate(${side * -20} ${lx + side * 4.8} ${ly - 1})`} />)
  }
  if (stage >= 2) {
    // 卷须：锚在茎上的实测点（控制点不在曲线上，直接用会把须画悬空）。
    const ax = cubicAt(0.6, p0x, p1x, p2x, p3x)
    const ay = cubicAt(0.6, p0y, p1y, p2y, p3y)
    parts.push(<path key="tendril1" d={`M ${p3x} ${p3y} q 6 -4 10 0 q 3 3 -1 4`} stroke={palette.stemYoung} strokeWidth={1.2} fill="none" strokeLinecap="round" />)
    parts.push(<path key="tendril2" d={`M ${ax} ${ay} q 5 -6 9 -3`} stroke={palette.stemYoung} strokeWidth={1.1} fill="none" strokeLinecap="round" />)
  }
  if (stage >= 4) {
    for (let f = 0; f < 3; f++) {
      const t = 0.42 + f * 0.2
      const fx = cubicAt(t, p0x, p1x, p2x, p3x)
      const fy = cubicAt(t, p0y, p1y, p2y, p3y)
      parts.push(<circle key={`flower-${f}`} cx={fx} cy={fy - 2} r={2.6} fill={shape.accent} />)
      parts.push(<circle key={`flowerc-${f}`} cx={fx} cy={fy - 2} r={1} fill={palette.bloomCore} />)
    }
  }
  if (evergreen) parts.push(<circle key="ever" cx={p3x} cy={p3y - 5} r={2.2} fill={palette.bloomCore} />)
  return parts
}

/** 传输蕨：基部拱出的羽状叶——先升后拱再垂的叶轴，两侧对生短羽片。 */
function drawFern({ cx, baseY, h, spread, stage, evergreen }: Brush): React.ReactNode {
  const fronds = Math.min(3 + stage, 6)
  const parts: React.ReactNode[] = []
  for (let i = 0; i < fronds; i++) {
    const side = i % 2 === 0 ? -1 : 1
    // 越靠外的叶拱越低，形成一束向外倾泻的羽叶
    const lift = 1 - Math.floor((i + 1) / 2) * 0.17
    const tipX = cx + side * spread * (0.5 + (i % 3) * 0.13)
    const peakY = baseY - h * 0.92 * lift
    const tipY = baseY - h * 0.5 * lift
    const c1x = cx + side * spread * 0.06
    parts.push(<path key={`r-${i}`} d={`M ${cx} ${baseY} C ${c1x} ${peakY}, ${tipX} ${peakY - 2}, ${tipX} ${tipY}`} stroke={palette.stemYoung} strokeWidth={1.3} fill="none" strokeLinecap="round" />)
    const leaflets = Math.min(3 + stage, 6)
    for (let l = 1; l <= leaflets; l++) {
      const t = l / (leaflets + 1)
      const px = cubicAt(t, cx, c1x, tipX, tipX)
      const py = cubicAt(t, baseY, peakY, peakY - 2, tipY)
      const len = 4.4 * (1 - t * 0.5)
      parts.push(<path key={`l-${i}-${l}`} d={`M ${px} ${py} l ${side * len * 0.75} ${-len} M ${px} ${py} l ${side * len * 0.95} ${len * 0.45}`} stroke={l % 2 === 0 ? palette.transferFrond : palette.leaf} strokeWidth={1.5} strokeLinecap="round" />)
    }
  }
  if (evergreen) parts.push(<circle key="ever" cx={cx} cy={baseY - h - 4} r={2.2} fill={palette.bloomCore} />)
  return parts
}

/** 守护兰：基生阔叶 + 直茎 + 茎中段叶片，花期依次开出 1–3 朵五瓣兰。 */
function drawOrchid({ cx, baseY, h, spread, stage, within, evergreen, shape }: Brush): React.ReactNode {
  const parts: React.ReactNode[] = []
  // 基生阔叶：三片扇出，让植株下部有体量，不再"花插在棍上"
  for (let i = 0; i < 4; i++) {
    const side = i - 1.5
    parts.push(<ellipse key={`basal-${i}`} cx={cx + side * spread * 0.24} cy={baseY - 3} rx={spread * 0.4} ry={2.4} fill={i % 2 === 0 ? palette.leafDeep : palette.leaf} transform={`rotate(${side * -14} ${cx + side * spread * 0.24} ${baseY - 3})`} />)
  }
  const tipX = cx + spread * 0.08
  const tipY = baseY - h
  parts.push(<path key="stem" d={`M ${cx} ${baseY} C ${cx - 1} ${baseY - h * 0.5}, ${tipX - 2} ${baseY - h * 0.72}, ${tipX} ${tipY}`} stroke={palette.stem} strokeWidth={1.6} fill="none" strokeLinecap="round" />)
  // 茎中段两片抱茎叶
  const midY = baseY - h * 0.45
  parts.push(<ellipse key="stemleaf-l" cx={cx - 4} cy={midY} rx={4.4} ry={2} fill={palette.leaf} transform={`rotate(-26 ${cx - 4} ${midY})`} />)
  parts.push(<ellipse key="stemleaf-r" cx={cx + 4.4} cy={midY - h * 0.14} rx={4} ry={1.8} fill={palette.leafDeep} transform={`rotate(24 ${cx + 4.4} ${midY - h * 0.14})`} />)
  const blooms = stage >= 5 ? 3 : stage >= 4 ? 2 : stage >= 3 ? 1 : 0
  if (blooms === 0) {
    parts.push(<circle key="bud" cx={tipX} cy={tipY - 2} r={2.6 + within} fill={palette.stemYoung} />)
  } else {
    for (let b = 0; b < blooms; b++) {
      const fx = tipX + (b - (blooms - 1) / 2) * spread * 0.34
      const fy = tipY - 3 - (b % 2) * 4.5
      for (let p = 0; p < 5; p++) {
        const deg = p * 72
        parts.push(<ellipse key={`p-${b}-${p}`} cx={fx} cy={fy} rx={2.3} ry={4.1} fill={shape.accent} transform={`rotate(${deg} ${fx} ${fy})`} />)
      }
      parts.push(<circle key={`c-${b}`} cx={fx} cy={fy} r={1.8} fill={palette.bloomCore} />)
    }
  }
  if (evergreen) parts.push(<circle key="ever" cx={tipX + spread * 0.18} cy={tipY} r={2.2} fill={palette.bloomCore} />)
  return parts
}

/** 知识树：主干 + 短枝 + 团状圆冠（阔树剪影），成熟期冠上结果。 */
function drawTree({ cx, baseY, h, spread, stage, evergreen, shape }: Brush): React.ReactNode {
  const barkH = h * 0.34
  const parts: React.ReactNode[] = []
  parts.push(<path key="bark" d={`M ${cx - 2.2} ${baseY} L ${cx + 2.2} ${baseY} L ${cx + 1.3} ${baseY - barkH} L ${cx - 1.3} ${baseY - barkH} Z`} fill={palette.pot} />)
  parts.push(<path key="branch" d={`M ${cx} ${baseY - barkH} q ${spread * 0.22} -3 ${spread * 0.3} -${h * 0.12}`} stroke={palette.pot} strokeWidth={1.5} fill="none" strokeLinecap="round" />)
  const r = spread * 0.5 + stage * 1.1
  const crownY = baseY - barkH - h * 0.34
  parts.push(<circle key="crown-l" cx={cx - spread * 0.18} cy={crownY + 2} r={r * 0.72} fill={palette.leafDeep} />)
  parts.push(<circle key="crown-r" cx={cx + spread * 0.19} cy={crownY + 1.5} r={r * 0.7} fill={palette.leaf} />)
  parts.push(<circle key="crown-t" cx={cx} cy={crownY - r * 0.42} r={r * 0.85} fill={palette.leaf} />)
  parts.push(<circle key="crown-hi" cx={cx - spread * 0.06} cy={crownY - r * 0.55} r={r * 0.4} fill={palette.stemYoung} opacity={0.55} />)
  if (shape.fruit && stage >= 5) {
    for (let f = 0; f < 4; f++) {
      const fx = cx + (f % 2 === 0 ? -1 : 1) * spread * (0.1 + f * 0.04)
      const fy = crownY + (f < 2 ? 1 : -r * 0.5)
      parts.push(<circle key={`fruit-${f}`} cx={fx} cy={fy} r={1.9} fill={palette.fruit} />)
    }
  }
  if (evergreen) parts.push(<circle key="ever" cx={cx} cy={crownY - r * 1.15} r={2.4} fill={palette.bloomCore} />)
  return parts
}

const drawers: Record<string, (brush: Brush) => React.ReactNode> = {
  'session-tree': drawFir,
  'cmd-mint': drawMint,
  'script-vine': drawVine,
  'transfer-fern': drawFern,
  'guard-orchid': drawOrchid,
  'knowledge-tree': drawTree,
}

/** 兜底画法：未知物种画一根带叶的基本茎，保证状态可渲染。 */
function drawFallback({ cx, baseY, h, stage }: Brush): React.ReactNode {
  const parts: React.ReactNode[] = []
  parts.push(<path key="stem" d={`M ${cx} ${baseY} L ${cx} ${baseY - h}`} stroke={palette.stem} strokeWidth={1.8} fill="none" strokeLinecap="round" />)
  for (let i = 0; i < Math.min(2 + stage, 6); i++) {
    const side = i % 2 === 0 ? -1 : 1
    parts.push(<ellipse key={`leaf-${i}`} cx={cx + side * 5} cy={baseY - h * (0.3 + i * 0.12)} rx={5} ry={2.4} fill={palette.leaf} />)
  }
  return parts
}

export interface SpecimenVisualProps {
  speciesId: string
  level: number
  shiny: boolean
  /** 画布尺寸；几何按 72 基准盒等比缩放。 */
  size?: number
  /** 无障碍名称（形如 "会话杉 7 级 幼苗 闪光"）。由调用方拼好，渲染器只负责挂上。 */
  label?: string
  /** potted（默认，带花盆）| grounded（种在场景地面里，只有土丘投影）。 */
  variant?: SpecimenVariant
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

/** 基准盒边长：全部几何以 72×72 为基准，渲染时整体缩放到 size。 */
const BASE_BOX = 72
const BASE_GROUND_Y = 62

/** 主渲染组件：一个物种实例的画。 */
export default function FloraSpecimen({ speciesId, level, shiny, size = 72, label, variant = 'potted' }: SpecimenVisualProps) {
  const shape = shapes[speciesId] ?? fallbackShape
  const { stage, growth, within, evergreen } = visualParams(level, shiny)
  // 同屏可能有多株闪光：渐变 id 必须每实例唯一，否则后者引用到前者的 defs。
  const glowId = `garden-shiny-${useId().replace(/[^a-zA-Z0-9-]/g, '')}`
  const h = (14 + growth * 30) * shape.height * (1 + within * 0.08)
  const spread = (6 + growth * 22) * shape.spread
  const brush: Brush = { cx: BASE_BOX / 2, baseY: BASE_GROUND_Y, h, spread, stage, within, evergreen, shape }
  const draw = drawers[speciesId] ?? drawFallback
  const scale = size / BASE_BOX

  return (
    <svg role="img" aria-label={label} width={size} height={size} viewBox={`0 0 ${size} ${size}`} data-stage={stage} data-shiny={shiny ? 'true' : 'false'}>
      <g transform={`scale(${scale})`}>
        {/* 闪光光晕：径向渐变贴着冠层，中心亮向外透明（抽屉与场景共用） */}
        {shiny ? (
          <>
            <radialGradient id={glowId}>
              <stop offset="0%" stopColor={palette.shinyGlow} stopOpacity="0.5" />
              <stop offset="45%" stopColor={palette.shinyGlow} stopOpacity="0.22" />
              <stop offset="100%" stopColor={palette.shinyGlow} stopOpacity="0" />
            </radialGradient>
            <circle cx={36} cy={BASE_GROUND_Y - h * 0.62} r={spread * 0.7 + 9} fill={`url(#${glowId})`} />
          </>
        ) : null}
        {/* 落脚点：potted 是花盆，grounded 是场景里的土丘投影（颜色跟场景走） */}
        {variant === 'potted' ? (
          <>
            <ellipse cx={36} cy={BASE_GROUND_Y + 4} rx={spread * 0.55 + 8} ry={3.6} fill={palette.soilDark} />
            <path d={`M ${36 - spread * 0.5 - 6} ${BASE_GROUND_Y} h ${spread + 12} l -3 9 h ${-(spread + 6)} z`} fill={palette.pot} />
          </>
        ) : (
          <ellipse cx={36} cy={BASE_GROUND_Y + 3} rx={spread * 0.6 + 7} ry={3.2} fill="var(--g-ground-deep)" opacity={0.55} />
        )}
        {draw(brush)}
        {/* 闪光闪星：外层 g 定位（基准盒坐标），内层 path 由 CSS 动画驱动 */}
        {shiny ? sparkles.map((star, index) => (
          <g key={`sparkle-${index}`} transform={`translate(${36 + star.x} ${BASE_GROUND_Y - h * 0.72 + star.y}) scale(${star.s})`}>
            <path
              className="garden-svg-sparkle"
              d={SPARKLE_PATH}
              fill={palette.shinyGlow}
              style={{ animationDelay: star.delay }}
            />
          </g>
        )) : null}
      </g>
    </svg>
  )
}
