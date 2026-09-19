import React, { useState } from 'react'

/**
 * image 画法的通用渲染器：一张图 + 落地接触阴影，题材无关。
 * 植株/动物/建筑在渲染层没有区别——图片内容即语义，运行时只负责
 * 按 (锚点, 尺寸) 把它"种"在地面上。
 *
 * 图片保留原始宽高比，将图片内归一化 (anchor.x, anchor.y) 位置
 * 对齐容器底边中心；非正方形图片也使用相同的落地语义。
 */
export default function ImageSpecimen({ src, anchor, size, label, onError }: {
  src: string
  anchor: { x: number; y: number }
  size: number | string
  label?: string
  onError?: () => void
}) {
  const [failedSrc, setFailedSrc] = useState<string>()
  return (
    <span className="garden-image-specimen" style={{ width: size, height: size }} data-anchor-x={anchor.x} data-anchor-y={anchor.y}>
      {failedSrc === src ? <span role="status" className="garden-art-unavailable">{label}：素材加载失败</span> : <img
        src={src}
        alt={label ?? ''}
        loading="lazy"
        decoding="async"
        onError={() => { setFailedSrc(src); onError?.() }}
        draggable={false}
        style={{ position: 'absolute', left: '50%', top: '100%', width: '100%', height: 'auto', transform: `translate(${-anchor.x * 100}%, ${-anchor.y * 100}%)` }}
      />}
      <span className="garden-contact-shadow" aria-hidden="true" />
    </span>
  )
}
