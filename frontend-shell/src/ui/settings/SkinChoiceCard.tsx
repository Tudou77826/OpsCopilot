import React from 'react';
import type { Skin } from '../appearanceTypes';
import { productSettingsStyles as styles } from './productSettingsStyles';

export interface SkinOption { id: Skin; label: string; hint: string }
export interface SkinChoice { value: Skin; options: SkinOption[]; onChange(skin: Skin): void }

/**
 * 皮肤选择卡。与主题卡并列在"外观"页，但两者是正交的：主题管明暗方向，皮肤管视觉语言从哪来。
 * 只在调用方注入 `skin` 时渲染——没有宿主令牌的环境里换皮肤是空操作（见 hostTokensAvailable 的说明），
 * 所以桌面壳不注入、也就看不到这张卡。
 */
export default function SkinChoiceCard({ skin }: { skin: SkinChoice }) {
    return (
        <div style={styles.card}>
            <div style={styles.cardTitle}>皮肤</div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <div style={styles.rowLabel}>视觉语言来源</div>
                    <div style={styles.rowDesc}>
                        皮肤决定颜色、圆角与字号取自哪里；界面明暗仍由上面的主题决定，两者互不影响。
                    </div>
                </div>
                <div style={styles.rowRight}>
                    <div style={styles.themeChoiceRow} role="radiogroup" aria-label="皮肤">
                        {skin.options.map(option => (
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
                                <span>{option.label}</span>
                            </button>
                        ))}
                    </div>
                </div>
            </div>
        </div>
    );
}
