import React, { useState, type CSSProperties } from 'react'
import ImageSpecimen from './packs/imageSpecimen'
import { resolveSpecimenImage, stageLabel, stageOf, type GardenPack, type GardenSpecimen, type GardenPending } from './pack'

/** 图片包消费资源、格位与文案；缺图收藏保留入口，不自动换成植物 SVG。 */
export default function ImageGardenScene({ pack, specimens, pending, onOpen, onDismissPending }: {
  pack: GardenPack; specimens: GardenSpecimen[]; pending: GardenPending[]
  onOpen(id: string): void; onDismissPending(at: string): void
}) {
  const [page, setPage] = useState(0)
  const [failed, setFailed] = useState(false)
  const layout = pack.images!.layout!
  const scene = pack.images!.scene!
  const resolved = specimens.map(spec => ({ spec, image: resolveSpecimenImage(pack, spec.speciesId, spec.level, spec.shiny, spec.instanceId) }))
  const ready = resolved.filter(item => item.image)
  const missing = resolved.filter(item => !item.image)
  const pages = Math.max(1, Math.ceil(ready.length / layout.slots.length))
  const active = Math.min(page, pages - 1)
  const slots = ready.slice(active * layout.slots.length, (active + 1) * layout.slots.length)
  return <div className="garden-image-exhibit" style={{ '--image-ratio': layout.aspectRatio, '--image-night-brightness': layout.nightBrightness, '--image-night-saturation': layout.nightSaturation } as CSSProperties}>
    <div className="garden-image-scroll" tabIndex={0} aria-label="收藏场景，可左右滚动">
      <div className="garden-image-world" role="group" aria-label={`${pack.presentation!.title}场景`}>
        <img className="garden-image-background garden-day-only" src={scene.day} alt="" onError={() => setFailed(true)} />
        <img className="garden-image-background garden-night-only" src={scene.night} alt="" onError={() => setFailed(true)} />
        {slots.map(({ spec, image }, index) => {
          const position = layout.slots[index]
          const name = pack.species[spec.speciesId]?.name ?? spec.speciesId
          const label = `${name} ${spec.level} 级 ${stageLabel(pack, stageOf(spec.level))}${spec.shiny ? ' 闪光' : ''}`
          const feedback = pending.find(item => item.instanceId === spec.instanceId)
          return <div key={spec.instanceId} className="garden-image-object" style={{ left: `${position.x}%`, top: `${position.y}%`, width: `calc((100cqh - 84px) * ${position.size})`, zIndex: Math.round(position.y) }}>
            <button className="garden-image-target" data-sway={layout.sway} aria-label={label} onClick={() => onOpen(spec.instanceId)}>
              <ImageSpecimen src={image!.src} anchor={image!.anchor} size="100%" label={label} />
              {spec.shiny && <span className="garden-image-quality" aria-hidden="true">✧</span>}
              <span className="garden-image-name">{name}</span>
            </button>
            {feedback && <button className="garden-image-feedback" aria-label={`新发现的${spec.shiny ? '闪光 ' : ''}${name}，点击标记已读`} onClick={() => onDismissPending(feedback.at)}>✧ 新发现</button>}
          </div>
        })}
      </div>
    </div>
    {failed && <p role="alert" className="garden-message" data-kind="error">场景素材加载失败，请检查呈现包。</p>}
    <div className="garden-image-footer">
      <span>左右游览 · 点击查看收藏</span>
      {pages > 1 && <span><button aria-label="上一组收藏" disabled={active === 0} onClick={() => setPage(active - 1)}>‹</button> {active + 1}/{pages} <button aria-label="下一组收藏" disabled={active === pages - 1} onClick={() => setPage(active + 1)}>›</button></span>}
      {missing.length > 0 && <details><summary>{missing.length} 项收藏待配图</summary><div className="garden-missing-list">{missing.map(({ spec }) => <button key={spec.instanceId} onClick={() => onOpen(spec.instanceId)}>{pack.species[spec.speciesId]?.name ?? spec.speciesId} · Lv.{spec.level}</button>)}</div></details>}
    </div>
  </div>
}
