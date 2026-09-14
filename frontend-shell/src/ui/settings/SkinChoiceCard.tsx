import React, { useMemo } from 'react';
import type { Skin } from '../appearanceTypes';
import type { Theme } from '../appearanceTypes';
import { productSettingsStyles as styles } from './productSettingsStyles';
import { skinSwatch } from '../appearance';

export interface SkinOption { id: Skin; label: string; hint: string }
export interface SkinChoice { value: Skin; options: SkinOption[]; onChange(skin: Skin): void; previewRoot?: HTMLElement }

/**
 * 皮肤选择卡。与主题卡并列在"外观"页，但两者是正交的：主题管明暗方向，皮肤管视觉语言从哪来。
 * 只在调用方注入 `skin` 时渲染——没有宿主令牌的环境里，宿主映射型皮肤换不出差别
 * （见 appearance.ts 的 hostTokensAvailable / availableSkins）。
 *
 * 预览色块不写死色值：`skinSwatch` 把候选皮肤临时写到目标元素上读真实计算值，
 * 所以预览与最终效果必然一致。mode 参与依赖是因为亮色/暗色下取到的色不同。
 */
export default function SkinChoiceCard({ skin, mode }: { skin: SkinChoice; mode: Theme }) {
    const swatches = useMemo(
        () => skin.options.map(option => ({ option, swatch: skinSwatch(option.id, skin.previewRoot) })),
        [skin.options, skin.previewRoot, mode],
    );
    return (
        <div style={styles.card}>
            <div style={styles.cardTitle}>皮肤</div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <div style={styles.rowLabel}>视觉语言来源</div>
                    <div style={styles.rowDesc}>
                        皮肤决定颜色取自哪里；界面明暗仍由上面的主题决定，两者互不影响。字体与字号属于终端自己的设置，不随皮肤变。
                    </div>
                </div>
                <div style={styles.rowRight}>
                    <div style={styles.themeChoiceRow} role="radiogroup" aria-label="皮肤">
                        {swatches.map(({ option, swatch }) => (
                            <button
                                key={option.id}
                                type="button"
                                role="radio"
                                aria-checked={skin.value === option.id}
                                title={option.hint}
                                style={{
                                    ...styles.themeChoiceCard,
                                    ...(skin.value === option.id ? styles.themeChoiceCardActive : {}),
                                }}
                                onClick={() => skin.onChange(option.id)}
                            >
                                {swatch ? (
                                    <span aria-hidden="true" data-testid={`skin-swatch-${option.id}`} style={{ display: 'flex', gap: '2px', flex: 'none' }}>
                                        {[swatch.bg, swatch.surface, swatch.accent, swatch.text].map((color, index) => (
                                            <span key={index} style={{ width: '10px', height: '16px', borderRadius: '2px', backgroundColor: color, border: '1px solid var(--border-subtle)' }} />
                                        ))}
                                    </span>
                                ) : null}
                                <span>{option.label}</span>
                            </button>
                        ))}
                    </div>
                </div>
            </div>
        </div>
    );
}
