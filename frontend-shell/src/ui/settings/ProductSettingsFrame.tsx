import React from 'react';
import { TbSearch } from 'react-icons/tb';
import { navGroupTitle, navItem, navItemActive } from './settingsStyles';
import { productSettingsStyles as styles } from './productSettingsStyles';

export interface SettingsNavItem<T extends string> { id: T; label: string; icon: React.ReactNode; badge?: boolean }
export interface SettingsNavGroup<T extends string> { category: string; items: SettingsNavItem<T>[] }
/** Actual desktop settings frame, shared by both hosts. */
export function ProductSettingsFrame<T extends string>({ embedded = false, showSaveAction = true, handleClose, searchInputRef, searchQuery, setSearchQuery, groupedNavItems, activeTab, setActiveTab, msg, handleSave, loading, showUnsavedConfirm, setShowUnsavedConfirm, onClose, persistConfig, children }: {
 showSaveAction?: boolean;
 embedded?: boolean; handleClose(): void; searchInputRef?: React.RefObject<HTMLInputElement>; searchQuery: string; setSearchQuery(value: string): void;
 groupedNavItems: SettingsNavGroup<T>[]; activeTab: T; setActiveTab(value: T): void; msg: string; handleSave(): void; loading: boolean;
 showUnsavedConfirm: boolean; setShowUnsavedConfirm(value: boolean): void; onClose(): void; persistConfig(close: boolean): void; children: React.ReactNode;
}) {
    return (
        <div style={{ ...styles.overlay, ...(embedded ? { position: 'absolute' as const } : {}) }}>
            <div style={{ ...styles.modal, ...(embedded ? { width: '100%', height: '100%' } : {}) }}>
                {/* Header */}
                <div style={styles.header}>
                    {/* 左上角返回入口：把标题做成可点击的「← 系统设置」，
                        贴近视口左上角、贴近用户高频的左侧导航区。
                        右上 × 与右下「取消」保留不动，这里只是新增更顺手的入口。 */}
                    <button
                        onClick={handleClose}
                        style={styles.backTitle}
                        title="返回"
                        className="settings-back-title"
                    >
                        <span style={styles.backArrow} className="settings-back-arrow">←</span>
                        <span>系统设置</span>
                    </button>
                    <button onClick={handleClose} style={styles.closeBtn}>×</button>
                </div>

                {/* Main Content Area */}
                <div style={styles.mainContent}>
                    {/* Left Sidebar */}
                    <div style={styles.sidebar}>
                        <div style={styles.searchBox}>
                            <div style={styles.searchInner}>
                                <span style={styles.searchIcon}>{TbSearch({})}</span>
                                <input
                                    ref={searchInputRef}
                                    style={styles.searchInput}
                                    placeholder="搜索设置..."
                                    value={searchQuery}
                                    onChange={(e) => setSearchQuery(e.target.value)}
                                />
                            </div>
                        </div>
                        <nav style={styles.nav}>
                            {groupedNavItems.length > 0 ? (
                                groupedNavItems.map((group) => (
                                    <div key={group.category} style={styles.navGroup}>
                                        <div style={navGroupTitle}>{group.category}</div>
                                        {group.items.map((item) => (
                                            <div
                                                key={item.id}
                                                style={activeTab === item.id ? navItemActive : navItem}
                                                onClick={() => setActiveTab(item.id)}
                                            >
                                                <span style={styles.navIcon}>{item.icon}</span>
                                                <span style={styles.navText}>{item.label}</span>
                                                {item.badge && (
                                                    <span style={styles.navBadge} />
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                ))
                            ) : (
                                <div style={styles.noResults}>没有找到匹配的设置项</div>
                            )}
                        </nav>
                    </div>

                    {/* Right Content Area */}
                    <div style={styles.contentArea}>
                        <div style={styles.pageContent}>
                            {/* Settings Content（页头大标题已移除：侧边栏已高亮当前页，避免标题冗余） */}
                            <div style={styles.settingsContent}>
                                {children}
                            </div>
                        </div>
                    </div>
                </div>

                {/* Footer */}
                <div style={styles.footer}>
                    <div style={styles.statusMsg}>{msg}</div>
                    <div style={styles.footerActions}>
                        <button onClick={handleClose} style={styles.cancelBtn}>取消</button>
                        {showSaveAction && <button onClick={handleSave} style={styles.saveBtn} disabled={loading}>
                            {loading ? '正在保存...' : '保存更改'}
                        </button>}
                    </div>
                </div>
            </div>

            {/* 关闭确认：存在未保存改动时提醒用户保存/放弃/继续编辑 */}
            {showUnsavedConfirm && (
                <div style={styles.unsavedOverlay}>
                    <div style={styles.unsavedModal}>
                        <h3 style={styles.unsavedTitle}>有未保存的更改</h3>
                        <p style={styles.unsavedMessage}>
                            当前设置页还有改动尚未保存，直接关闭将丢失这些改动。
                        </p>
                        <div style={styles.unsavedActions}>
                            <button style={styles.cancelBtn} onClick={() => setShowUnsavedConfirm(false)}>继续编辑</button>
                            <button style={styles.discardBtn} onClick={() => { setShowUnsavedConfirm(false); onClose(); }}>放弃更改</button>
                            <button style={styles.saveBtn} onClick={() => { setShowUnsavedConfirm(false); void persistConfig(true); }} disabled={loading}>
                                保存并关闭
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
