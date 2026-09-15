import React, { useMemo } from 'react'
import FloraSpecimen from './packs/floraRenderer'
import ImageSpecimen from './packs/imageSpecimen'
import ImageGardenScene from './ImageGardenScene'
import { getPack, stageOf, stageLabel, resolveSpecimenImage, MAX_LEVEL, DEFAULT_PACK_ID, type GardenPack, type GardenSpecimen, type GardenPending } from './pack'
import { gardenScene as palette } from './palette'
import './gardenScene.css'

/**
 * 花园场景：把状态快照画成一块有纵深的种植园，而不是卡片列表。
 *
 * 纵深来自三层种植带（远/中/近）：越近越大、越靠下、z 序越高，加上每株的
 * 确定性抖动，形成"错落有致"的自然感。背景是昼夜双景——亮色模式是白天
 * （太阳/白云），暗色模式是夜晚（月亮/星星/萤火虫），由 data-theme 驱动
 * （gardenScene.css）。闪光株有光晕 + 环绕闪星 + 呼吸动画；新发现株有地面
 * 扩散环与悬浮徽标——收藏的高光时刻全部用视觉表达，不靠文字找补。
 *
 * 所有随机（星点、抖动）都是确定性种子：呈现是状态的纯函数，重渲染不闪变。
 */

/** 三层种植带：bottom 是距场景底部百分比，scale 是相对基准的尺寸倍率。 */
export type PlantingRow = { bottom: number; scale: number; z: number }

/** 内置布景（CSS 天空 + SVG 地面）的种植带。 */
const ROWS: readonly PlantingRow[] = [
  { bottom: 33, scale: 0.62, z: 10 },
  { bottom: 18, scale: 0.82, z: 20 },
  { bottom: 4, scale: 1.04, z: 30 },
]

/**
 * 包自带手绘场景时的种植带：贴合画出来的可种植区（前景土路/草地边缘），
 * 整体比内置布景更贴近底边，避免植株踩到画中的岩石与远景。
 */
const IMAGE_SCENE_ROWS: readonly PlantingRow[] = [
  { bottom: 20, scale: 0.58, z: 10 },
  { bottom: 11, scale: 0.78, z: 20 },
  { bottom: 2, scale: 1.0, z: 30 },
]

/** 基准尺寸（近带最大边长，px）。 */
const BASE_SIZE = 132
/** 单株尺寸下限：远排低等级株也不至于小到读不出形象。 */
const MIN_SIZE = 76

/** 字符串哈希 → [0,1)：同一株的抖动在重渲染间保持不变。 */
function hash01(text: string): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return ((hash >>> 0) % 10000) / 10000
}

/** mulberry32：背景星点/萤火虫的确定性伪随机。 */
function mulberry32(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a |= 0; a = (a + 0x6D2B79F5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export interface Placed {
  spec: GardenSpecimen
  /** 横向位置（种植区宽度的百分比）。 */
  x: number
  /** 距底部百分比（含抖动）。 */
  bottom: number
  /** 渲染边长（px）。 */
  size: number
  /** 层带序号（越大越近）。 */
  row: number
}

/**
 * 深度布局：横向用全局格位（等分 + 抖动），深度按"距最新株位次"轮转
 * 远→中→近——相邻植株必然处于不同层带，形成错落有致的种植，而不是三排各自的
 * 等分点竖向叠柱。最新的一株总在最前排，成长的可见性最好。
 */
export function layoutSpecimens(specs: GardenSpecimen[], rows: readonly PlantingRow[] = ROWS): Placed[] {
  const rowOrder = [2, 1, 0] as const
  return specs.map((spec, index) => {
    const distanceFromNewest = specs.length - 1 - index
    const row = rowOrder[distanceFromNewest % 3]
    const info = rows[row]
    const base = ((index + 0.5) / specs.length) * 100
    // 抖动幅度允许相邻株轻微互相压边——零遮挡的"贴纸拼贴"没有前后关系。
    const jitterAmp = Math.min(7, 60 / Math.max(specs.length, 1))
    const jx = (hash01(spec.instanceId + 'x') - 0.5) * 2 * jitterAmp
    const jy = (hash01(spec.instanceId + 'y') - 0.5) * 5
    const levelScale = 0.7 + 0.6 * (Math.min(spec.level, MAX_LEVEL) / MAX_LEVEL)
    return {
      spec,
      // 收敛到 8%–92%：按钮以中心定位，避免宽株在场地边缘被裁掉半边。
      x: Math.min(92, Math.max(8, base + jx)),
      bottom: Math.max(1, info.bottom + jy),
      size: Math.max(MIN_SIZE, Math.round(BASE_SIZE * info.scale * levelScale)),
      row,
    }
  })
}

/** 夜空：43 颗星 + 3 颗大闪星。位置确定性生成。 */
function StarField() {
  const stars = useMemo(() => {
    const rand = mulberry32(7)
    return Array.from({ length: 43 }, (_, i) => ({
      x: rand() * 100,
      y: rand() * 52,
      size: 1 + rand() * 1.6,
      duration: 1.8 + rand() * 3.4,
      delay: rand() * 4,
      big: i % 17 === 5,
    }))
  }, [])
  return (
    <div className="garden-night-only" aria-hidden="true">
      {stars.map((star, i) => star.big ? (
        <span key={i} className="garden-star-star" style={{
          left: `${star.x}%`, top: `${star.y}%`, fontSize: 10,
          animationDuration: `${star.duration}s`, animationDelay: `${star.delay}s`,
        }}>✦</span>
      ) : (
        <span key={i} className="garden-star" style={{
          left: `${star.x}%`, top: `${star.y}%`,
          width: star.size, height: star.size,
          animationDuration: `${star.duration}s`, animationDelay: `${star.delay}s`,
        }} />
      ))}
    </div>
  )
}

/** 夜间的萤火虫：贴地缓飘、明灭。数量是夜晚魔法感的一半。 */
function Fireflies() {
  const flies = useMemo(() => {
    const rand = mulberry32(23)
    return Array.from({ length: 14 }, (_, i) => ({
      x: 3 + rand() * 94,
      y: 50 + rand() * 30,
      duration: 4.2 + rand() * 3.8,
      delay: (i / 14) * 6 + rand() * 2,
      scale: 0.8 + rand() * 0.7,
    }))
  }, [])
  return (
    <div className="garden-night-only" aria-hidden="true">
      {flies.map((fly, i) => (
        <span key={i} className="garden-firefly" style={{
          left: `${fly.x}%`, top: `${fly.y}%`,
          width: 3 * fly.scale, height: 3 * fly.scale,
          animationDuration: `${fly.duration}s`, animationDelay: `${fly.delay}s`,
        }} />
      ))}
    </div>
  )
}

/** 远山与三层地面 + 花园杂件（草丛/野花/石头/围栏），全部确定性生成。 */
function Ground() {
  const details = useMemo(() => {
    const rand = mulberry32(41)
    return {
      tufts: Array.from({ length: 16 }, () => ({ x: 20 + rand() * 960, y: 118 + rand() * 26, s: 0.7 + rand() * 0.6 })),
      flowers: Array.from({ length: 7 }, () => ({
        x: 30 + rand() * 940, y: 122 + rand() * 20,
        c: palette.wildflowers[Math.floor(rand() * palette.wildflowers.length)],
      })),
      stones: Array.from({ length: 3 }, () => ({ x: 60 + rand() * 880, y: 130 + rand() * 10, w: 7 + rand() * 8 })),
      bushes: Array.from({ length: 4 }, (_, i) => ({
        x: 80 + i * 260 + rand() * 120,
        y: 86 + rand() * 14,
        r: 4 + rand() * 3.5,
      })),
      posts: Array.from({ length: 13 }, (_, i) => ({
        x: 40 + i * 78,
        y: 60 + Math.sin((i / 12) * Math.PI) * 7,
      })),
    }
  }, [])
  return (
    <svg className="garden-planting-ground" viewBox="0 0 1000 150" preserveAspectRatio="none" aria-hidden="true">
      {/* 远山与栅栏 */}
      <path d="M0,64 C140,40 300,58 460,44 C640,28 820,52 1000,36 L1000,150 L0,150 Z" fill="var(--g-hill-far)" />
      <g opacity="0.5">
        <rect x="36" y="66" width="928" height="1.6" fill="var(--g-hill-near)" />
        {details.posts.map((post, i) => (
          <rect key={i} x={post.x} y={post.y} width="2.6" height="10" rx="0.8" fill="var(--g-hill-near)" />
        ))}
      </g>
      <path d="M0,92 C180,72 400,98 620,82 C800,68 900,90 1000,76 L1000,150 L0,150 Z" fill="var(--g-hill-near)" />
      {/* 主草地 */}
      <path d="M0,112 C250,102 600,120 1000,108 L1000,150 L0,150 Z" fill="var(--g-ground)" />
      <path d="M0,112 C250,102 600,120 1000,108" fill="none" stroke="var(--g-ground-light)" strokeWidth="2" opacity="0.55" />
      <rect x="0" y="128" width="1000" height="22" fill="var(--g-ground-mid)" />
      <rect x="0" y="142" width="1000" height="8" fill="var(--g-ground-deep)" />
      {/* 远丘灌木：让中远景也有"植被"，支撑纵深 */}
      {details.bushes.map((bush, i) => (
        <g key={i} opacity={0.9}>
          <circle cx={bush.x} cy={bush.y} r={bush.r} fill="var(--g-hill-near)" />
          <circle cx={bush.x + bush.r * 0.7} cy={bush.y + 1} r={bush.r * 0.7} fill="var(--g-ground)" />
        </g>
      ))}
      {/* 草丛 */}
      {details.tufts.map((tuft, i) => (
        <path
          key={i}
          d={`M ${tuft.x} ${tuft.y} q ${-2 * tuft.s} ${-4 * tuft.s} ${-3.4 * tuft.s} ${-4 * tuft.s} M ${tuft.x} ${tuft.y} q 0 ${-5 * tuft.s} 0 ${-6.4 * tuft.s} M ${tuft.x} ${tuft.y} q ${2 * tuft.s} ${-4 * tuft.s} ${3.4 * tuft.s} ${-4 * tuft.s}`}
          stroke="var(--g-tuft)"
          strokeWidth="1.1"
          fill="none"
          opacity="0.75"
        />
      ))}
      {/* 石头 */}
      {details.stones.map((stone, i) => (
        <ellipse key={i} cx={stone.x} cy={stone.y} rx={stone.w} ry={stone.w * 0.42} fill="var(--g-hill-far)" opacity="0.9" />
      ))}
      {/* 野花只在白天开 */}
      <g className="garden-day-only">
        {details.flowers.map((flower, i) => (
          <g key={i}>
            <circle cx={flower.x} cy={flower.y} r="1.9" fill={flower.c} />
            <circle cx={flower.x} cy={flower.y} r="0.7" fill={palette.wildflowerCore} />
          </g>
        ))}
      </g>
    </svg>
  )
}

/** 白天的飞鸟：两三只剪影缓慢掠过天空。 */
function Birds() {
  const birds = useMemo(() => {
    const rand = mulberry32(59)
    return Array.from({ length: 3 }, (_, i) => ({
      top: 6 + rand() * 22,
      scale: 0.7 + rand() * 0.6,
      duration: 55 + rand() * 40,
      delay: -rand() * 60 - i * 18,
    }))
  }, [])
  return (
    <div className="garden-day-only" aria-hidden="true">
      {birds.map((bird, i) => (
        <svg
          key={i}
          className="garden-bird"
          width={22 * bird.scale}
          height={10 * bird.scale}
          viewBox="0 0 22 10"
          style={{ top: `${bird.top}%`, animationDuration: `${bird.duration}s`, animationDelay: `${bird.delay}s` }}
        >
          <path d="M2 8 Q 6 2 11 7 Q 16 2 20 8" stroke="var(--g-bird)" strokeWidth="1.6" fill="none" strokeLinecap="round" />
        </svg>
      ))}
    </div>
  )
}

/** 场景背景：天空渐变（CSS）+ 日月星云萤火 + 远山地面。空花园也复用同一片天地。 */
/** 前景压角的大草叶（虚化框景）：左右各一丛。 */
function FrameGrass() {
  const blades = (flip: boolean) => (
    <svg width={220} height={90} viewBox="0 0 220 90" style={{ transform: flip ? 'scaleX(-1)' : undefined }}>
      <g stroke="var(--g-ground-deep)" strokeWidth="7" fill="none" strokeLinecap="round">
        <path d="M20 90 Q 30 40 12 12" />
        <path d="M55 90 Q 60 46 88 22" />
        <path d="M92 90 Q 90 52 70 30" />
      </g>
      <g stroke="var(--g-ground-mid)" strokeWidth="5" fill="none" strokeLinecap="round">
        <path d="M150 90 Q 158 50 180 26" />
        <path d="M190 90 Q 186 56 204 38" />
      </g>
    </svg>
  )
  return (
    <div aria-hidden="true">
      <div className="garden-frame-grass" style={{ left: -30 }}>{blades(false)}</div>
      <div className="garden-frame-grass" style={{ right: -30 }}>{blades(true)}</div>
    </div>
  )
}

/**
 * 场景背景：包自带昼/夜成对背景图时用它（题材自有世界），否则回退内置植物风布景
 * （CSS 天空 + 日月星云萤火 + SVG 地面）。空花园复用同一片天地。
 */
export function Backdrop({ scene }: { scene?: { day?: string; night?: string } }) {
  if (scene?.day && scene.night) {
    return (
      <div className="garden-sky" aria-hidden="true">
        <img className="garden-backdrop garden-day-only" src={scene.day} alt="" />
        <img className="garden-backdrop garden-night-only" src={scene.night} alt="" />
      </div>
    )
  }
  return (
    <div className="garden-sky" aria-hidden="true">
      <StarField />
      <Birds />
      {/* 日月放左侧，避开右侧信息条与闪光株的光效叠罗汉 */}
      <div className="garden-day-only garden-sun" style={{ left: '13%', top: '9%', width: 96, height: 96 }} />
      <div className="garden-night-only garden-moon" style={{ left: '14%', top: '8%', width: 92, height: 92 }} />
      <div className="garden-day-only garden-cloud" style={{ right: '16%', top: '10%', animationDuration: '130s', animationDelay: '-18s', opacity: 0.95 }} />
      <div className="garden-day-only garden-cloud" style={{ right: '38%', top: '24%', animationDuration: '95s', animationDelay: '-70s', transform: 'scale(0.72)', opacity: 0.8 }} />
      <div className="garden-day-only garden-cloud" style={{ right: '4%', top: '4%', animationDuration: '160s', animationDelay: '-110s', transform: 'scale(1.3)', opacity: 0.9 }} />
      <Fireflies />
      <Ground />
      <FrameGrass />
    </div>
  )
}

export interface GardenSceneProps {
  specimens: GardenSpecimen[]
  /** 未领取的反馈（发现/闪光发现），在有对应植株的位置画新株标记。 */
  pending: GardenPending[]
  packId?: string
  /** 当前打开详情的株（场景里给它一个地面选中环）。 */
  selectedId?: string
  onOpen(instanceId: string): void
  /** 点击新株徽标：标记该条反馈已读。 */
  onDismissPending(at: string): void
}

export default function GardenScene(props: GardenSceneProps) {
  const pack = getPack(props.packId ?? DEFAULT_PACK_ID)
  if (pack?.art === 'image') return <ImageGardenScene {...props} pack={pack} />
  return <LegacyGardenScene {...props} />
}

function LegacyGardenScene({ specimens, pending, packId = DEFAULT_PACK_ID, selectedId, onOpen, onDismissPending }: GardenSceneProps) {
  const pack = getPack(packId)
  // 手绘场景的可种植区更贴底：有包背景时用 IMAGE_SCENE_ROWS。
  const rows = pack?.images?.scene ? IMAGE_SCENE_ROWS : ROWS
  const placed = useMemo(() => layoutSpecimens(specimens, rows), [specimens, rows])
  const pendingByInstance = useMemo(() => {
    const map = new Map<string, GardenPending>()
    pending.forEach(item => { if (item.instanceId) map.set(item.instanceId, item) })
    return map
  }, [pending])
  // 宽度下限：株数多时场地向右延伸，横向滚动查看（而不是挤成一团）。
  const minWidth = Math.max(specimens.length * 92, 560)

  return (
    <div className="garden-planting" role="group" aria-label="花园场景">
      <Backdrop scene={pack?.images?.scene} />
      <div style={{ position: 'absolute', inset: 0, minWidth }}>
        {placed.map(({ spec, x, bottom, size, row }) => {
          const meta = pack?.species[spec.speciesId]
          const name = meta?.name ?? spec.speciesId
          const stage = stageOf(spec.level)
          const label = `${name} ${spec.level} 级 ${stageLabel(pack, stage)}${spec.shiny ? ' 闪光' : ''}`
          // 每物种选画法：包里给了图就走 image 画法，没覆盖的物种回退内置 SVG。
          const image = resolveSpecimenImage(pack, spec.speciesId, spec.level, spec.shiny, spec.instanceId)
          const item = pendingByInstance.get(spec.instanceId)
          return (
            <div
              key={spec.instanceId}
              className="garden-specimen"
              data-depth={row}
              style={{ left: `${x}%`, bottom: `${bottom}%`, width: size, height: size, zIndex: row }}
            >
              {item ? <span className="garden-new-ring" aria-hidden="true" /> : null}
              {selectedId === spec.instanceId ? <span className="garden-selected-ring" aria-hidden="true" /> : null}
              <button
                type="button"
                className="garden-plant-btn"
                onClick={() => onOpen(spec.instanceId)}
                aria-label={label}
                title={label}
                style={{ width: size, height: size, position: 'relative' }}
              >
                {/* 光效强度按画法收敛：绘本图自带金边配色，只留地面光池与少量闪星；SVG 画法保留光晕 */}
                {spec.shiny && !image ? <span className="garden-halo" aria-hidden="true" /> : null}
                {spec.shiny ? <span className="garden-glow-pool" aria-hidden="true" /> : null}
                <span className={spec.shiny ? 'garden-plant-sway garden-shiny-plant' : 'garden-plant-sway'} style={{ display: 'block', animationDelay: `${(hash01(spec.instanceId) * 3).toFixed(2)}s` }}>
                  {image
                    ? <ImageSpecimen src={image.src} anchor={image.anchor} size={size} label={label} />
                    : <FloraSpecimen speciesId={spec.speciesId} level={spec.level} shiny={spec.shiny} size={size} label={label} variant="grounded" />}
                </span>
                {/* 画面层闪星 + 双层盘旋光尘：环绕植株转动的"魔力粒子" */}
                {spec.shiny ? (
                  <>
                    <span className="garden-sparkle" style={{ left: '2%', top: '12%', fontSize: 14, animationDelay: '0.2s' }} aria-hidden="true">✦</span>
                    <span className="garden-sparkle" style={{ right: '4%', top: '4%', fontSize: 10, animationDelay: '1.1s' }} aria-hidden="true">✦</span>
                    {!image ? <span className="garden-sparkle" style={{ right: '10%', bottom: '28%', fontSize: 12, animationDelay: '1.7s' }} aria-hidden="true">✦</span> : null}
                    <span className="garden-mote-orbit" aria-hidden="true">
                      <span className="garden-mote" style={{ left: '50%', top: '4%' }} />
                      <span className="garden-mote" style={{ left: '88%', top: '46%' }} />
                      <span className="garden-mote" style={{ left: '12%', top: '46%' }} />
                    </span>
                    <span className="garden-mote-orbit reverse" aria-hidden="true">
                      <span className="garden-mote" style={{ left: '78%', top: '16%' }} />
                      <span className="garden-mote" style={{ left: '22%', top: '16%' }} />
                    </span>
                  </>
                ) : null}
              </button>
              {item ? (
                <button
                  type="button"
                  className="garden-new-badge"
                  onClick={() => onDismissPending(item.at)}
                  aria-label={item.kind === 'shiny-discovered' ? `新发现的闪光株 ${name}，点击标记已读` : `新发现的 ${name}，点击标记已读`}
                  title="新发现！点击标记已读"
                  style={{ fontSize: 12 }}
                >
                  ✦ {item.kind === 'shiny-discovered' ? '闪光新株' : '新株'}
                </button>
              ) : null}
            </div>
          )
        })}
      </div>
    </div>
  )
}
