import React from 'react'

/**
 * image 画法的通用渲染器：一张图 + 落地接触阴影，题材无关。
 * 植株/动物/建筑在渲染层没有区别——图片内容即语义，运行时只负责
 * 按 (锚点, 尺寸) 把它"种"在地面上。
 *
 * 锚点语义：anchor.y 是根/地基在图片高度内的纵向百分比。
 * 图片以 contain 放进方形容器、底部对齐基线后，向下平移 (100-y)%
 * 让锚点正好落在地面接触线上（y=100 表示图片底边就是根）。
 */
export default function ImageSpecimen({ src, anchor, size, label }: {
  src: string
  anchor: { x: number; y: number }
  size: number
  label?: string
}) {
  return (
    <span className="garden-image-specimen" style={{ width: size, height: size }} data-anchor-x={anchor.x} data-anchor-y={anchor.y}>
      <img
        src={src}
        alt={label ?? ''}
        draggable={false}
        style={{ transform: `translateY(${100 - anchor.y}%)` }}
      />
      <span className="garden-contact-shadow" aria-hidden="true" />
    </span>
  )
}
