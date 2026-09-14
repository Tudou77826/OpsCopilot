import React from 'react';
import { TbClock, TbScreenShare, TbStethoscope, TbMessageChatbot, TbCode, TbBolt, TbBook } from 'react-icons/tb';
import logo from '../assets/logo-universal.png';

export type ProductTab = 'sessions' | 'troubleshoot' | 'chat' | 'knowledge' | 'script';
const ATTENTION_DOT_COLOR = 'var(--danger)';

/** Extracted from the desktop App. Both desktop and plugin mount this same toolbar. */
export function ProductToolbar({ status = '就绪', theme, onNewConnection, onThemeToggle: handleThemeToggle, onSettings, updateAvailable = false, highlightNeedsAttention = false, parsedTimestamp }: {
  status?: string; theme: 'dark' | 'light'; onNewConnection(): void; onThemeToggle(): void; onSettings(): void;
  updateAvailable?: boolean; highlightNeedsAttention?: boolean; parsedTimestamp?: { local: string } | null;
}) {
  const setIsSmartModalOpen = (_value: boolean) => onNewConnection();
  const setIsSettingsOpen = (_value: boolean) => onSettings();
  return (<div style={{
                padding: '6px 12px',
                background: 'var(--bg-elevated)',
                borderBottom: '1px solid var(--bg-primary)',
                color: 'var(--text-primary)',
                display: 'flex',
                gap: '12px',
                alignItems: 'center',
                justifyContent: 'space-between'
            }}>
                <div style={{ display: 'flex', gap: '16px', alignItems: 'center' }}>
                    {status === '就绪' || status === '已连接' || status === '已重连' ? (
                        <img src={logo} alt="OpsCopilot" className="shell-brand-logo" style={{ width: 28, height: 28 }} />
                    ) : null}
                    {status !== '就绪' && status !== '已连接' && status !== '已重连' && (
                        <div style={{
                            ...styles.loadingIndicator,
                            color: (status.includes('失败') || status.includes('请先')) ? 'var(--danger)' : 'var(--text-muted)',
                        }}>
                            {!status.includes('失败') && !status.includes('请先') && (
                                <svg className="spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                                    <path d="M12 2a10 10 0 0 1 10 10" />
                                </svg>
                            )}
                            <span>{status}</span>
                        </div>
                    )}
                    <button onClick={() => setIsSmartModalOpen(true)} style={styles.primaryBtn}>
                        + 新建连接
                    </button>
                    <button onClick={handleThemeToggle} style={styles.iconBtnUnified} title={theme === 'dark' ? '切换到亮色' : '切换到暗色'}>
                        {theme === 'dark' ? (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <circle cx="12" cy="12" r="4" />
                                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                            </svg>
                        ) : (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
                            </svg>
                        )}
                    </button>
                    <button onClick={() => setIsSettingsOpen(true)} style={{ ...styles.iconBtnUnified, position: 'relative' }} title="设置">
                        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                            <circle cx="12" cy="12" r="3"></circle>
                            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                        </svg>
                        {(updateAvailable || highlightNeedsAttention) && (
                            <span style={{
                                position: 'absolute',
                                top: '2px',
                                right: '2px',
                                width: '8px',
                                height: '8px',
                                borderRadius: '50%',
                                backgroundColor: ATTENTION_DOT_COLOR,
                                border: '1px solid var(--bg-primary)',
                            }} />
                        )}
                    </button>
                    {parsedTimestamp && (
                        <div style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '4px',
                            padding: '2px 8px',
                            background: 'var(--bg-input)',
                            borderRadius: '4px',
                            fontSize: '0.75rem',
                        }}>
                            <span style={{ color: 'var(--text-muted)' }}>{TbClock({ size: 12 })}</span>
                            <span style={{ color: 'var(--text-secondary)' }}>{parsedTimestamp.local}</span>
                        </div>
                    )}
                </div>
            </div>);
}

export function ProductNavigation({ isSidebarOpen, sidebarTab, toggleSidebar, isQuickCommandOpen, onToggleQuickCommands, tabs = ['sessions','troubleshoot','chat','knowledge','script'] }: {
  isSidebarOpen: boolean; sidebarTab: ProductTab; toggleSidebar(tab: ProductTab): void;
  isQuickCommandOpen: boolean; onToggleQuickCommands(): void; tabs?: ProductTab[];
}) {
  const setIsQuickCommandOpen = (_value: boolean) => onToggleQuickCommands();
  return (<div style={styles.rightNav}>
                    {tabs.includes('sessions') && (
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'sessions') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'sessions') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('sessions')}
                        title="会话管理"
                    >
                        {TbScreenShare({ size: 20 })}
                    </div>
                    )}
                    {tabs.includes('troubleshoot') && (
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'troubleshoot') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'troubleshoot') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('troubleshoot')}
                        title="定位助手"
                    >
                        {TbStethoscope({ size: 20 })}
                    </div>
                    )}
                    {tabs.includes('chat') && (
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'chat') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'chat') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('chat')}
                        title="AI 问答"
                    >
                        {TbMessageChatbot({ size: 20 })}
                    </div>
                    )}
                    {tabs.includes('knowledge') && (
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'knowledge') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'knowledge') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('knowledge')}
                        title="知识库"
                    >
                        {TbBook({ size: 20 })}
                    </div>
                    )}
                    {tabs.includes('script') && (
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'script') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'script') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('script')}
                        title="脚本录制"
                    >
                        {TbCode({ size: 20 })}
                    </div>
                    )}
                    <div style={{ flex: 1 }} />
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: isQuickCommandOpen ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: isQuickCommandOpen ? '2px solid var(--accent)' : '2px solid transparent',
                        }}
                        onClick={() => setIsQuickCommandOpen(!isQuickCommandOpen)}
                        title="快捷命令"
                        data-testid="nav-icon-quickcommands"
                    >
                        {TbBolt({ size: 20 })}
                    </div>
                </div>);
}

/** The desktop's slot geometry: quick commands belong below the terminal, beside the sidebar. */
export function ProductFrame({ toolbar, terminal, quickCommands, sidebar, navigation, footer, children, id }: {
  toolbar: React.ReactNode; terminal: React.ReactNode; quickCommands: React.ReactNode;
  sidebar: React.ReactNode; navigation: React.ReactNode; footer: React.ReactNode; children?: React.ReactNode; id?: string;
}) {
  return <div id={id} data-product-shell style={{ height: '100%', minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', flex: 1 }}>
    {toolbar}
    <div style={{ flex: 1, position: 'relative', overflow: 'hidden', display: 'flex', flexDirection: 'row' }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', position: 'relative', overflow: 'hidden', minWidth: 0 }}>
        <div style={{ flex: 1, position: 'relative', overflow: 'hidden' }}>{terminal}</div>
        {quickCommands}
      </div>
      {sidebar}
      {navigation}
    </div>
    {footer}
    {children}
  </div>;
}

const styles = {
    primaryBtn: {
        height: '28px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '4px',
        padding: '0 12px',
        backgroundColor: 'var(--accent)',
        border: 'none',
        borderRadius: '4px',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontSize: '0.82rem',
        fontWeight: 500 as const,
        transition: 'background-color 0.15s',
    },
    iconBtnUnified: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'transparent',
        border: 'none',
        borderRadius: '4px',
        color: 'var(--text-tertiary)',
        cursor: 'pointer',
        padding: '4px',
        transition: 'color 0.15s',
    },
    loadingIndicator: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: '0.8rem',
    },
    rightNav: {
        width: '40px',
        backgroundColor: 'var(--bg-secondary)',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        borderLeft: '1px solid var(--border)',
        paddingTop: '10px',
        paddingBottom: '10px',
    },
    navIcon: {
        width: '100%',
        height: '42px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        fontSize: '24px',
        marginBottom: '4px',
        transition: 'background-color 0.2s',
    }
};
