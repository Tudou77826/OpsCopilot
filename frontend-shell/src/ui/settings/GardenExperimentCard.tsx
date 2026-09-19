import React from 'react';
import { productSettingsStyles as styles } from './productSettingsStyles';

export default function GardenExperimentCard({ enabled, onChange }: { enabled: boolean; onChange(value: boolean): void }) {
    return <details style={styles.card}>
        <summary>实验功能</summary>
        <label style={styles.row}>
            <span style={styles.rowLeft}>
                <span style={styles.rowLabel}>启用养成功能（实验）</span>
                <span style={styles.rowDesc}>默认关闭，暂缓开发。关闭后隐藏入口并停止积累，已有收藏保留。保存后生效。</span>
            </span>
            <input type="checkbox" checked={enabled} onChange={event => onChange(event.target.checked)} />
        </label>
    </details>;
}
