import React, { useRef, useState } from 'react';
import type { ProductTab } from './ProductChrome';

/** Desktop sidebar frame, with bounds measured within the current host. */
export function ProductSidebar({ isOpen, activeTab, onToggle, children }: {
 isOpen: boolean; activeTab: ProductTab; onToggle(): void; children: React.ReactNode;
}) {
 const container = useRef<HTMLDivElement>(null);
 const [widths, setWidths] = useState<Partial<Record<ProductTab, number>>>({});
 const width = widths[activeTab] ?? (activeTab === 'knowledge' ? 500 : activeTab === 'chat' || activeTab === 'troubleshoot' ? 420 : 300);
 const titles: Record<ProductTab,string> = { sessions: '会话管理', troubleshoot: '定位助手', chat: 'AI 问答', knowledge: '知识库', script: '脚本录制' };
 const drag = useRef<{x:number;width:number}>();
 return <div ref={container} className={`sidebar-shell sidebar-${activeTab}`} style={{
   ...styles.container, width: isOpen ? width : 0, flexShrink: 0, minWidth: 0, position: 'relative',
   borderLeft: isOpen ? '1px solid var(--border)' : 'none', boxShadow: isOpen ? undefined : 'none', overflow: isOpen ? 'visible' : 'hidden',
 }}>
   {isOpen && <div role="separator" aria-label="调整侧栏宽度" style={styles.resizeHandle}
     onPointerDown={e => { drag.current={x:e.clientX,width}; e.currentTarget.setPointerCapture(e.pointerId); }}
     onPointerMove={e => { if (!drag.current) return; const max = Math.min(800, (container.current?.parentElement?.clientWidth ?? 920) - 120); setWidths(v => ({...v,[activeTab]:Math.max(250, Math.min(max, drag.current!.width + drag.current!.x - e.clientX))})); }}
     onPointerUp={() => {drag.current=undefined;}} onPointerCancel={() => {drag.current=undefined;}} />}
   <div style={{ display: isOpen ? 'flex' : 'none', flexDirection: 'column', height: '100%', flex: 1, minHeight: 0 }}>
     <div className="sidebar-header" style={styles.header}>
       <div className="sidebar-title-group"><h3 style={styles.title}>{titles[activeTab]}</h3>
         {(activeTab === 'troubleshoot' || activeTab === 'chat') && <span className="sidebar-ai-badge">AI</span>}
       </div>
       <button onClick={onToggle} style={styles.closeButton} aria-label="Toggle Sidebar">×</button>
     </div>
     <div style={styles.mainArea}><div style={styles.content}>{children}</div></div>
   </div>
 </div>;
}
const styles = {
    container: {
        backgroundColor: 'var(--bg-secondary)',
        borderLeft: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
    },
    resizeHandle: {
        position: 'absolute' as const,
        left: -3,
        top: 0,
        bottom: 0,
        width: '6px',
        cursor: 'col-resize',
        zIndex: 100,
        backgroundColor: 'transparent',
    },
    header: {
        padding: '10px 16px',
        backgroundColor: 'var(--bg-secondary)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        borderBottom: '1px solid var(--border)',
    },
    mainArea: {
        display: 'flex',
        flex: 1,
        overflow: 'hidden',
        minHeight: 0,
    },
    title: {
        margin: 0,
        fontSize: '14px',
        color: 'var(--text-primary)',
    },
    closeButton: {
        background: 'none',
        border: 'none',
        color: 'var(--text-secondary)',
        cursor: 'pointer',
        fontSize: '18px',
    },
    content: {
        flex: 1,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: 0,
    },
};
