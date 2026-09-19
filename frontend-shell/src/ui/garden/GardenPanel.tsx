import React, { useMemo, useState } from 'react'
import GardenScene from './GardenScene'
import { DEFAULT_PACK_ID, getPack, stageOf, stageLabel, resolveSpecimenVisual, packSupportsSnapshot, MAX_LEVEL, type GardenPack, type GardenSnapshot, type GardenSpecimen } from './pack'
// 引入即登记内置呈现包（见 packs/index.ts）。
import './packs'
import { font } from '../settings/settingsStyles'
import type { GardenHost } from '../ports'

/**
 * 花园停靠面板：与快捷命令共用槽位形态（同一处停靠、可关闭归还终端空间），
 * 内部是一整块有纵深的种植园场景——白天/夜晚跟随工作台明暗（gardenScene.css），
 * 闪光与新发现全部用视觉表达；点击植株打开详情抽屉。
 *
 * 状态只来自后端快照（宿主端口 GardenHost），前台不上报任何事件。
 */
export interface GardenPanelProps {
  host?: GardenHost
  isOpen?: boolean
  /** 滚动场景的停靠高度；完整构图的内容包按可用宽度等比确定高度。 */
  height?: number
  packId?: string
}

function levelProgress(specimen: GardenSpecimen): number {
  return Math.min(1, Math.max(0, specimen.xp / (80 + specimen.level * 20)))
}

/**
 * 收藏详情卡：采用花园自身的昼夜配色，完整展示档案信息。
 * 左侧大图（image 画法直接放图，SVG 画法用无盆地面形态），右侧收藏档案：
 * 编号 / 学名 / 名称 / 含义 / 描述 / 等级与本级成长 / 品质徽章 / 获得日期。
 * 只展示业务类别，不出现命令正文、凭据、服务器地址或文件内容。
 */
function SpecimenDetail({ specimen, collectionNo, pack, onClose }: {
  specimen: GardenSpecimen
  collectionNo: number
  pack: GardenPack
  onClose(): void
}) {
  const meta = pack.elements[specimen.itemId]
  const stage = stageOf(specimen.level)
  const visual = resolveSpecimenVisual(pack, specimen.itemId, specimen.level, specimen.shiny, specimen.instanceId)
  React.useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div role="dialog" aria-label={`${meta?.name ?? specimen.itemId} 收藏详情`} className="garden-collection-card" data-art-style={pack.art?.style}>
      <div className="garden-collection-figure">
        {visual
          ? <img className="garden-collection-image" src={visual.src} alt={meta?.name} />
          : <span className="garden-art-unavailable">当前内容包未提供该收藏的形象</span>}
      </div>
      <div className="garden-collection-body">
        <p className="garden-collection-eyebrow">收藏档案 · No.{String(collectionNo).padStart(2, '0')}</p>
        {meta?.latin ? <p className="garden-collection-latin">{meta.latin}</p> : null}
        <h3 className="garden-collection-name">{meta?.name ?? specimen.itemId}</h3>
        <p className="garden-collection-meaning">{meta?.meaning ?? '—'}</p>
        {meta?.description ? <p className="garden-collection-description">{meta.description}</p> : null}
        <div className="garden-collection-stats">
          <div className="garden-progress-row">
            等级 {specimen.level} / {MAX_LEVEL}{specimen.level >= MAX_LEVEL ? `（${pack.presentation.maxLevel}）` : ''} · 本级成长 {Math.round(levelProgress(specimen) * 100)}%
            <span className="garden-pity-bar" aria-hidden="true">
              <span style={{ width: `${Math.round(levelProgress(specimen) * 100)}%` }} />
            </span>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '2.5px 0', fontSize: font.sm }}>
            <span className="garden-collection-stat">形态 {stageLabel(pack, stage, specimen.itemId)}（第 {stage + 1} / 6 档）</span>
            {specimen.shiny
              ? <span className="garden-quality" data-tier="shiny">✦ 闪光</span>
              : <span className="garden-quality" data-tier="normal">普通</span>}
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: font.sm }}>
            <span className="garden-collection-stat">获得</span>
            <span>{specimen.acquiredAt.slice(0, 10)}</span>
          </div>
        </div>
      </div>
      <button type="button" className="garden-collection-close" onClick={onClose} aria-label="关闭收藏详情">×</button>
    </div>
  )
}

export default function GardenPanel({ host, isOpen = true, height = 260, packId = DEFAULT_PACK_ID }: GardenPanelProps) {
  const pack = getPack(packId)
  const [snapshot, setSnapshot] = useState<GardenSnapshot | null>(null)
  const [error, setError] = useState('')
  const [openId, setOpenId] = useState<string>()
  const [shopOpen, setShopOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [busyItem, setBusyItem] = useState('')
  const [actionError, setActionError] = useState('')

  React.useEffect(() => {
    if (!isOpen || !host) return
    let cancelled = false
    // 快照只在面板打开时拉一次（不做后台轮询：花园不在前台时零流量）。
    void host.snapshot().then(next => { if (!cancelled) { setSnapshot(next); setError('') } })
      .catch((reason: Error) => { if (!cancelled) setError(reason.message) })
    return () => { cancelled = true }
  }, [host, isOpen])

  const specimens = useMemo(() => snapshot?.specimens ?? [], [snapshot])
  const placed = useMemo(() => specimens.filter(spec => spec.placement.placed), [specimens])
  const inventory = useMemo(() => specimens.filter(spec => !spec.placement.placed), [specimens])
  const opened = specimens.find(spec => spec.instanceId === openId)
  const catalog = useMemo(() => Object.entries(pack?.elements ?? {}).filter(([, element]) => element.commerce), [pack])

  const buy = async (itemId: string) => {
    const commerce = pack?.elements[itemId]?.commerce
    if (!host || !commerce || busyItem) return
    setBusyItem(itemId); setActionError('')
    try {
      await host.purchase(itemId, commerce.price, commerce.initialLevel)
      setSnapshot(await host.snapshot())
      setEditing(true); setShopOpen(false)
    } catch (reason) { setActionError(reason instanceof Error ? reason.message : '购买失败') }
    finally { setBusyItem('') }
  }
  const savePlacement = async (instanceId: string, x: number, y: number) => {
    if (!host) return
    const spec = specimens.find(item => item.instanceId === instanceId)
    if (!spec) return
    setActionError('')
    try { setSnapshot(await host.place(instanceId, x, y, spec.placement.scale || 1, spec.placement.flipX)) }
    catch (reason) {
      const message = reason instanceof Error ? reason.message : '保存布局失败'
      setActionError(message)
      try { setSnapshot(await host.snapshot()) }
      catch (refreshReason) { setActionError(`${message}；重新同步失败：${refreshReason instanceof Error ? refreshReason.message : '未知错误'}`) }
    }
  }
  const placeFromInventory = (instanceId: string) => {
    const element = pack?.elements[specimens.find(item => item.instanceId === instanceId)?.itemId ?? '']
    const polygon = element ? pack?.scene.zones[element.placement.zone]?.polygon : undefined
    if (!polygon?.length) return
    const minX = Math.min(...polygon.map(point => point.x)), maxX = Math.max(...polygon.map(point => point.x))
    const minY = Math.min(...polygon.map(point => point.y)), maxY = Math.max(...polygon.map(point => point.y))
    const index = placed.filter(item => pack?.elements[item.itemId]?.placement.zone === element?.placement.zone).length
    const column = index % 5, row = Math.floor(index / 5) % 2
    void savePlacement(instanceId, minX + (maxX - minX) * (0.14 + column * 0.18), minY + (maxY - minY) * (0.5 + row * 0.32))
  }
  const stow = async (instanceId: string) => {
    if (!host) return
    setActionError('')
    try { setSnapshot(await host.stow(instanceId)) }
    catch (reason) { setActionError(reason instanceof Error ? reason.message : '收回失败') }
  }

  // 关闭时不挂载场景 DOM，因此不会加载图片、解码素材或运行动画。
  if (!isOpen) return null

  if (!host) {
    return (
      <div className="garden-root" style={{ height }}>
        <div className="garden-message">当前宿主未提供花园能力。</div>
      </div>
    )
  }
  if (error) {
    // 花园加载失败不影响终端与连接；错误必须显式说明原因。
    return (
      <div className="garden-root" style={{ height }}>
        <div className="garden-message" data-kind="error" role="status">花园加载失败：{error}</div>
      </div>
    )
  }
  if (!pack) {
    return <div className="garden-root" style={{ height }}><div className="garden-message" data-kind="error" role="alert">内容包 {packId} 未安装或登记失败。</div></div>
  }
  if (snapshot && !packSupportsSnapshot(pack, snapshot)) {
    return <div className="garden-root" style={{ height }}><div className="garden-message" data-kind="error" role="alert">内容包 {pack.name} 不支持状态 schema v{snapshot.schemaVersion}。</div></div>
  }

  return (
    <div className="garden-panel-frame">
    <div className="garden-root garden-picture-panel" style={{
      height: pack.scene.framing === 'fit'
        ? `calc((100cqw - 2px) / ${pack.scene.aspectRatio} + var(--garden-scene-inset) + 2px)`
        : height,
      boxSizing: 'border-box', border: '1px solid var(--g-chip-border)',
    }}>
      <GardenScene specimens={specimens} packId={packId} selectedId={openId} editing={editing}
        onOpen={setOpenId} onPlace={savePlacement} onStow={id => { void stow(id) }} />
      {placed.length === 0 ? (
        <>
          <div className="garden-empty" role="status">
            这里还没有摆放元素。打开商店挑选，再进入布置模式放进场景。
          </div>
        </>
      ) : null}
      <div className="garden-chip-row">
        <span className="garden-chip" style={{ fontWeight: 600 }}>
          {pack.presentation.title}
          <span className="garden-chip-muted">
            {snapshot ? `等级 ${snapshot.gardenLevel} · ${specimens.length} ${pack.presentation.unit}` : '载入中…'}
          </span>
        </span>
        <span className="garden-chip garden-wallet" title={`累计获得 ${snapshot?.earned ?? 0}，已花费 ${snapshot?.spent ?? 0}`}>◆ {snapshot?.balance ?? 0} 灵感币</span>
        <button className="garden-toolbar-button" aria-pressed={shopOpen} onClick={() => { setShopOpen(value => !value); setEditing(false) }}>商店</button>
        <button className="garden-toolbar-button" aria-pressed={editing} onClick={() => { setEditing(value => !value); setShopOpen(false); setOpenId(undefined) }}>{editing ? '完成' : '布置'}</button>
      </div>
      {shopOpen ? <section className="garden-commerce-panel" aria-label="元素商店">
        <header><strong>元素商店</strong><span>工作获得灵感币，自由选择喜欢的伙伴</span></header>
        <div className="garden-catalog-grid">
          {catalog.map(([itemId, element]) => {
            const commerce = element.commerce!
            const visual = resolveSpecimenVisual(pack, itemId, commerce.initialLevel, false, `catalog-${itemId}`)
            return <article className="garden-catalog-card" key={itemId}>
              {visual ? <img src={visual.src} alt="" /> : null}
              <div><strong>{element.name}</strong><small>{commerce.category}</small></div>
              <button disabled={busyItem !== '' || (snapshot?.balance ?? 0) < commerce.price} onClick={() => { void buy(itemId) }}>
                {busyItem === itemId ? '购买中…' : `◆ ${commerce.price}`}
              </button>
            </article>
          })}
        </div>
      </section> : null}
      {editing && inventory.length ? <section className="garden-commerce-panel garden-layout-panel" aria-label="场景布置">
        <header><strong>场景布置</strong><span>拖动已摆放元素；不想展示的可以收回背包</span></header>
        <div className="garden-inventory-row">
          {inventory.map(spec => {
            const element = pack.elements[spec.itemId]
            const visual = resolveSpecimenVisual(pack, spec.itemId, spec.level, spec.shiny, spec.instanceId)
            return <button className="garden-inventory-item" key={spec.instanceId} onClick={() => placeFromInventory(spec.instanceId)}>
              {visual ? <img src={visual.src} alt="" /> : null}<span>{element?.name ?? spec.itemId}</span><small>摆放</small>
            </button>
          })}
        </div>
      </section> : null}
      {actionError ? <div className="garden-action-error" role="status">{actionError}</div> : null}
      {opened ? (
        <SpecimenDetail
          specimen={opened}
          // 收藏编号按获得顺序（快照即按 acquiredAt 升序）。
          collectionNo={specimens.indexOf(opened) + 1}
          pack={pack}
          onClose={() => setOpenId(undefined)}
        />
      ) : null}
    </div>
    </div>
  )
}
