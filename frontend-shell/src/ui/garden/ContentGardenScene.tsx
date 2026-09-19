import React, { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import GardenActor from './GardenActor'
import { useGardenRuntime } from './useGardenRuntime'
import { resolveSpecimenVisual, stageLabel, stageOf, type GardenPack, type GardenSpecimen } from './pack'

interface PlacedContent {
  spec: GardenSpecimen
  visual: NonNullable<ReturnType<typeof resolveSpecimenVisual>>
  slot: GardenPack['scene']['slots'][number]
  page: number
}

/**
 * 按活动区域独立分页：地面动物、固定建筑和空中元素可以共用一页，
 * 同一区域超出格位后才进入下一页。算法只读取包数据，不识别具体题材。
 */
export function placePackContent(pack: GardenPack, specimens: GardenSpecimen[]): { placed: PlacedContent[]; unavailable: number } {
  const counters = new Map<string, number>()
  const placed: PlacedContent[] = []
  let unavailable = 0
  for (const spec of [...specimens].sort((a, b) => a.acquiredAt.localeCompare(b.acquiredAt) || a.instanceId.localeCompare(b.instanceId))) {
    const element = pack.elements[spec.itemId]
    const visual = resolveSpecimenVisual(pack, spec.itemId, spec.level, spec.shiny, spec.instanceId)
    if (!element || !visual) {
      unavailable++
      continue
    }
    if (!spec.placement.placed) continue
    const compatible = pack.scene.slots.filter(slot => slot.zone === element.placement.zone)
    if (!compatible.length) {
      unavailable++
      continue
    }
    const offset = counters.get(element.placement.zone) ?? 0
    counters.set(element.placement.zone, offset + 1)
    const authored = compatible[offset % compatible.length]
    // 个体尺寸归元素自身的 defaultSize；格位只提供图层，不能把某株植物的尺寸
    // 偶然套给另一种元素。这样动物、建筑等内容包也只需声明资源与参数。
    placed.push({ spec, visual, slot: { ...authored, x: spec.placement.x, y: spec.placement.y, size: spec.placement.scale }, page: 0 })
  }
  return { placed, unavailable }
}

function layerIndex(pack: GardenPack, layer: string): number {
  return Math.max(0, pack.scene.layers.indexOf(layer))
}

function fitValue(fit: GardenPack['scene']['fit']): CSSProperties['objectFit'] {
  return fit === 'stretch' ? 'fill' : fit
}

/** 单一内容场景运行时：植物、动物、建筑都走这里，差异只来自包清单与图片。 */
export default function ContentGardenScene({ pack, specimens, selectedId, editing = false, onOpen, onPlace, onStow }: {
  pack: GardenPack
  specimens: GardenSpecimen[]
  selectedId?: string
  editing?: boolean
  onOpen(id: string): void
  onPlace?(id: string, x: number, y: number): void
  onStow?(id: string): void
}) {
  const [page, setPage] = useState(0)
  const viewport = useRef<HTMLDivElement>(null)
  const [worldSize, setWorldSize] = useState<{ width: number; height: number }>()
  useEffect(() => {
    if (pack.scene.framing !== 'fit' || !viewport.current) return
    const node = viewport.current
    const measure = () => {
      const height = Math.min(node.clientHeight, node.clientWidth / pack.scene.aspectRatio)
      setWorldSize({ width: height * pack.scene.aspectRatio, height })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [pack.scene.framing, pack.scene.aspectRatio])
  const [failedAssets, setFailedAssets] = useState<Set<string>>(() => new Set())
  const layout = useMemo(() => placePackContent(pack, specimens), [pack, specimens])
  const pages = Math.max(1, ...layout.placed.map(item => item.page + 1))
  const active = Math.min(page, pages - 1)
  const visible = useMemo(() => layout.placed.filter(item => item.page === active), [active, layout.placed])
  const actorInputs = useMemo(() => visible.map(item => ({ instanceId: item.spec.instanceId, itemId: item.spec.itemId, slot: item.slot })), [visible])
  const runtime = useGardenRuntime(pack, actorInputs)
  const actors = useMemo(() => new Map(runtime.actors.map(actor => [actor.instanceId, actor])), [runtime.actors])
  const day = pack.assets[pack.scene.dayAsset]
  const night = pack.assets[pack.scene.nightAsset]
  const markFailed = (assetId: string) => setFailedAssets(current => new Set(current).add(assetId))
  const style = {
    '--image-ratio': pack.scene.aspectRatio,
    '--image-night-brightness': pack.scene.lighting.nightBrightness,
    '--image-night-saturation': pack.scene.lighting.nightSaturation,
    '--backdrop-day-brightness': pack.scene.backdrop?.day.brightness ?? 1,
    '--backdrop-day-saturation': pack.scene.backdrop?.day.saturation ?? 1,
    '--backdrop-day-contrast': pack.scene.backdrop?.day.contrast ?? 1,
    '--backdrop-day-opacity': pack.scene.backdrop?.day.opacity ?? 1,
    '--backdrop-night-brightness': pack.scene.backdrop?.night.brightness ?? 1,
    '--backdrop-night-saturation': pack.scene.backdrop?.night.saturation ?? 1,
    '--backdrop-night-contrast': pack.scene.backdrop?.night.contrast ?? 1,
    '--backdrop-night-opacity': pack.scene.backdrop?.night.opacity ?? 1,
    '--garden-world-height': pack.scene.framing === 'fit' && worldSize ? `${worldSize.height}px` : 'calc(100cqh - var(--garden-scene-inset))',
  } as CSSProperties

  return <div className="garden-image-exhibit" style={style} data-pack-id={pack.id} data-pack-version={pack.version} data-art-style={pack.art?.style} data-framing={pack.scene.framing} data-runtime-running={runtime.running ? 'true' : 'false'}>
    <div ref={viewport} className="garden-image-scroll" tabIndex={0} aria-label={pack.scene.framing === 'fit' ? '收藏庭院' : '收藏场景，可左右滚动'}>
      <div className="garden-image-world" role="group" aria-label={`${pack.presentation.title}场景`} style={pack.scene.framing === 'fit' && worldSize ? worldSize : undefined}>
        <img className="garden-image-background garden-day-only" src={day.src} alt="" decoding="async" style={{ objectFit: fitValue(pack.scene.fit) }} onError={() => markFailed(day.id)} />
        <img className="garden-image-background garden-night-only" src={night.src} alt="" decoding="async" style={{ objectFit: fitValue(pack.scene.fit) }} onError={() => markFailed(night.id)} />
        {pack.scene.decorations.map((decoration, index) => {
          const asset = pack.assets[decoration.asset]
          const anchor = decoration.anchor ?? { x: 0.5, y: 1 }
          return <img key={`${decoration.asset}-${index}`} className="garden-image-decoration" src={asset.src} alt="" aria-hidden="true"
            style={{ left: `${decoration.x * 100}%`, top: `${decoration.y * 100}%`, width: `calc(var(--garden-world-height) * ${decoration.size})`, zIndex: 5 + layerIndex(pack, decoration.layer), transform: `translate(${-anchor.x * 100}%, ${-anchor.y * 100}%)` }}
            onError={() => markFailed(asset.id)} />
        })}
        {pack.scene.ambience?.map((effect, index) => <span key={index} className="garden-ambience" data-effect={effect.kind} aria-hidden="true" style={{ left: `${effect.x * 100}%`, top: `${effect.y * 100}%`, width: `${effect.size * 100}%`, animationDuration: `${effect.durationMs}ms`, animationDelay: `-${effect.delayMs}ms` }} />)}
        {visible.map(({ spec, visual, slot }) => {
          const element = pack.elements[spec.itemId]
          const label = `${element.name} ${spec.level} 级 ${stageLabel(pack, stageOf(spec.level), spec.itemId)}${spec.shiny ? ' 闪光' : ''}`
          const actor = actors.get(spec.instanceId)
          if (!actor) return null
          return <GardenActor key={spec.instanceId} pack={pack} element={element} specimen={spec} visual={visual}
            actor={actor} now={runtime.now} reducedMotion={runtime.reducedMotion} selected={selectedId === spec.instanceId}
            layer={layerIndex(pack, slot.layer)} size={slot.size} label={label}
            editing={editing} onOpen={onOpen} onMove={onPlace} onStow={onStow} onReact={runtime.react} onAssetError={markFailed} />
        })}
      </div>
    </div>
    {failedAssets.size > 0 && <p role="alert" className="garden-message" data-kind="error">内容包有 {failedAssets.size} 个素材加载失败，请检查或重新安装内容包。</p>}
    <div className="garden-image-footer">
      <span>{editing ? '拖动元素调整位置 · × 收回背包' : pack.scene.framing === 'fit' ? '点击查看收藏' : '左右游览 · 点击查看收藏'}{layout.unavailable ? ` · ${layout.unavailable} 项未被当前内容包收录` : ''}</span>
      {pages > 1 && <span><button aria-label="上一组收藏" disabled={active === 0} onClick={() => setPage(active - 1)}>‹</button> {active + 1}/{pages} <button aria-label="下一组收藏" disabled={active === pages - 1} onClick={() => setPage(active + 1)}>›</button></span>}
    </div>
  </div>
}
