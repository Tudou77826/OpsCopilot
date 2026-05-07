import React, { useState, useEffect } from 'react';
import ReactDOM from 'react-dom';
import { TimelineEvent, filterTimelineEvents, generateMarkdown } from '../../utils/timeline';
// @ts-ignore
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface ModuleInfo {
    name: string;
    fileName: string;
}

interface ServiceInfo {
    name: string;
    modules: ModuleInfo[];
}

// Extract a section's first line from Markdown for preview
function previewSection(md: string, heading: string): string {
    return previewSectionAny(md, heading);
}

// Try multiple heading candidates, return first match
function previewSectionAny(md: string, ...headings: string[]): string {
    for (const h of headings) {
        const result = previewSingleSection(md, h);
        if (result) return result;
    }
    return '';
}

function previewSingleSection(md: string, heading: string): string {
    const prefix = '## ' + heading;
    const lines = md.split('\n');
    let found = false;
    for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.startsWith(prefix)) { found = true; continue; }
        if (!found) continue;
        if (trimmed.startsWith('## ')) break;
        if (trimmed === '') continue;
        return trimmed;
    }
    return '';
}

const IndexPreview: React.FC<{ conclusion: string }> = ({ conclusion }) => {
    const phenomena = previewSectionAny(conclusion, '问题现象', '问题描述', '现象', '故障现象');
    const keywords = previewSectionAny(conclusion, '关键词', '关键字');
    const components = previewSectionAny(conclusion, '涉及组件', '相关组件', '组件');

    const hasAny = !!(phenomena || keywords || components);

    return (
        <div style={styles.indexPreview}>
            <div style={styles.indexPreviewTitle}>
                索引标签 <span style={styles.indexPreviewHint}>（自动提取，请保留 ## 关键词 / ## 问题现象 / ## 涉及组件 标题）</span>
            </div>
            {hasAny ? (
                <div style={styles.indexPreviewList}>
                    {phenomena && <div style={styles.indexRow}><span style={styles.indexLabel}>现象</span><span style={styles.indexValue} title={phenomena}>{phenomena}</span></div>}
                    {keywords && <div style={styles.indexRow}><span style={styles.indexLabel}>关键词</span><span style={styles.indexValue} title={keywords}>{keywords}</span></div>}
                    {components && <div style={styles.indexRow}><span style={styles.indexLabel}>组件</span><span style={styles.indexValue} title={components}>{components}</span></div>}
                </div>
            ) : (
                <div style={styles.indexPreviewEmpty}>未检测到索引标签，请确认内容包含上述 section 标题</div>
            )}
        </div>
    );
};

export interface ArchiveParams {
    conclusion: string;
    service: string;
    module: string;
    targetFile: string;
}

interface SessionReviewModalProps {
    isOpen: boolean;
    onClose: () => void;
    rootCause: string;
    onArchive: (params: ArchiveParams) => void;
}

const SessionReviewModal: React.FC<SessionReviewModalProps> = ({ isOpen, onClose, rootCause, onArchive }) => {
    const [events, setEvents] = useState<TimelineEvent[]>([]);
    const [view, setView] = useState<'timeline' | 'conclusion'>('timeline');
    const [conclusion, setConclusion] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [markdownContent, setMarkdownContent] = useState('');

    // Archive selection state
    const [serviceOptions, setServiceOptions] = useState<ServiceInfo[]>([]);
    const [selectedService, setSelectedService] = useState('');
    const [selectedModule, setSelectedModule] = useState('');
    const [selectedFileName, setSelectedFileName] = useState('');
    const [isNewService, setIsNewService] = useState(false);
    const [isNewModule, setIsNewModule] = useState(false);
    const [newServiceName, setNewServiceName] = useState('');
    const [newModuleName, setNewModuleName] = useState('');

    useEffect(() => {
        if (isOpen) {
            loadTimeline();
            setView('timeline');
            setConclusion('');
            resetArchiveSelection();
            loadCatalogServices();
        }
    }, [isOpen]);

    const resetArchiveSelection = () => {
        setSelectedService('');
        setSelectedModule('');
        setSelectedFileName('');
        setIsNewService(false);
        setIsNewModule(false);
        setNewServiceName('');
        setNewModuleName('');
    };

    const loadCatalogServices = async () => {
        try {
            // @ts-ignore
            if (window.go?.main?.App?.GetCatalogServices) {
                // @ts-ignore
                const services = await window.go.main.App.GetCatalogServices();
                if (Array.isArray(services)) {
                    setServiceOptions(services);
                }
            }
        } catch (e) {
            console.error("[SessionReviewModal] Error loading catalog services:", e);
        }
    };

    const loadTimeline = async () => {
        try {
            // @ts-ignore
            if (window.go?.main?.App?.GetSessionTimeline) {
                // @ts-ignore
                const sessionData = await window.go.main.App.GetSessionTimeline();

                if (sessionData && sessionData.timeline && Array.isArray(sessionData.timeline)) {
                    const data = sessionData.timeline;
                    const problem = sessionData.problem || "未指定";

                    const filtered = filterTimelineEvents(data);
                    setEvents(filtered);

                    const md = generateMarkdown(filtered, problem, rootCause);
                    setMarkdownContent(md);
                } else {
                    setEvents([]);
                    setMarkdownContent(`# 排查会话记录\n\n**根本原因:** ${rootCause}\n\n（无记录）`);
                }
            }
        } catch (e) {
            console.error("[SessionReviewModal] Error loading timeline:", e);
        } finally {
            setIsLoading(false);
        }
    };

    const handleAnalyze = async () => {
        setIsLoading(true);
        setConclusion('');
        setView('conclusion'); // Switch to conclusion view immediately so user sees streaming

        let streamConclusion = '';
        let cancelStream: (() => void) | undefined;

        try {
            // Listen for streaming tokens
            if (EventsOn) {
                cancelStream = EventsOn("conclusion:token", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    const token = String(data?.token ?? '');
                    if (token) {
                        streamConclusion += token;
                        setConclusion(streamConclusion);
                    }
                });

                // Also listen for error
                const cancelError = EventsOn("conclusion:error", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    const errMsg = String(data?.error ?? 'Unknown error');
                    if (!streamConclusion) {
                        setConclusion(`生成总结失败: ${errMsg}`);
                    }
                });

                // Listen for completion
                const cancelDone = EventsOn("conclusion:done", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    const final = String(data?.conclusion ?? '');
                    if (final) {
                        setConclusion(final);
                    }
                });

                // Combine cancel functions
                const origCancel = cancelStream;
                cancelStream = () => {
                    origCancel?.();
                    cancelError?.();
                    cancelDone?.();
                };
            }

            // @ts-ignore
            if (window.go?.main?.App?.StreamConclusion) {
                // @ts-ignore
                await window.go.main.App.StreamConclusion(markdownContent, rootCause);
            } else {
                // Fallback to non-streaming
                // @ts-ignore
                if (window.go?.main?.App?.GenerateConclusionWithContext) {
                    // @ts-ignore
                    const result = await window.go.main.App.GenerateConclusionWithContext(markdownContent, rootCause);
                    setConclusion(result);
                }
            }
        } catch (e) {
            console.error(e);
            if (!streamConclusion) {
                setConclusion(`生成总结失败: ${e}`);
            }
        } finally {
            cancelStream?.();
            setIsLoading(false);
        }
    };

    const handleServiceChange = (value: string) => {
        if (value === '__new__') {
            setIsNewService(true);
            setIsNewModule(false);
            setSelectedService('');
            setSelectedModule('');
            setSelectedFileName('');
            return;
        }
        setIsNewService(false);
        setSelectedService(value);
        setSelectedModule('');
        setSelectedFileName('');
        setIsNewModule(false);
    };

    const handleModuleChange = (value: string) => {
        if (value === '__new__') {
            setIsNewModule(true);
            setSelectedModule('');
            setSelectedFileName('');
            return;
        }
        setIsNewModule(false);
        setSelectedModule(value);
        // Find the file name for this module
        const svc = serviceOptions.find(s => s.name === selectedService);
        if (svc) {
            const mod = svc.modules.find(m => m.name === value);
            setSelectedFileName(mod?.fileName || '');
        }
    };

    const getEffectiveService = (): string => {
        return isNewService ? newServiceName.trim() : selectedService;
    };

    const getEffectiveModule = (): string => {
        return isNewModule ? newModuleName.trim() : selectedModule;
    };

    const canArchive = (): boolean => {
        const svc = getEffectiveService();
        const mod = getEffectiveModule();
        return !!svc && !!mod;
    };

    const handleArchive = () => {
        onArchive({
            conclusion,
            service: getEffectiveService(),
            module: getEffectiveModule(),
            targetFile: selectedFileName,
        });
    };

    // Get modules for selected service
    const currentModules = serviceOptions.find(s => s.name === selectedService)?.modules || [];

    if (!isOpen) return null;

    return ReactDOM.createPortal(
        <>
            <style>{`@keyframes sr-spin { to { transform: rotate(360deg); } }`}</style>
            <div style={styles.overlay}>
                <div style={styles.modal}>
                <div style={styles.header}>
                    <h3 style={styles.title}>
                        {view === 'timeline' ? '编辑排查记录' : '确认排查总结'}
                    </h3>
                    <button onClick={onClose} style={styles.closeButton}>&times;</button>
                </div>

                <div style={styles.body}>
                    {view === 'timeline' ? (
                        <div style={styles.conclusionContainer}>
                            <textarea
                                value={markdownContent}
                                onChange={(e) => setMarkdownContent(e.target.value)}
                                style={styles.textarea}
                                placeholder="正在生成排查记录..."
                            />
                        </div>
                    ) : (
                        <div style={styles.conclusionContainer}>
                            <textarea
                                value={conclusion}
                                onChange={(e) => setConclusion(e.target.value)}
                                style={styles.textarea}
                                placeholder="AI 正在生成总结..."
                            />

                            {/* Index tags preview — extracted from conclusion for Catalog indexing */}
                            {conclusion && <IndexPreview conclusion={conclusion} />}

                            {/* Archive location selection */}
                            <div style={styles.archiveSection}>
                                <div style={styles.archiveSectionTitle}>归档位置</div>
                                <div style={styles.archiveRow}>
                                    <label style={styles.label}>微服务</label>
                                    {isNewService ? (
                                        <div style={styles.inputRow}>
                                            <input
                                                style={styles.textInput}
                                                value={newServiceName}
                                                onChange={(e) => setNewServiceName(e.target.value)}
                                                placeholder="输入新微服务名称"
                                                autoFocus
                                            />
                                            <button
                                                style={styles.linkButton}
                                                onClick={() => { setIsNewService(false); setNewServiceName(''); }}
                                            >
                                                取消
                                            </button>
                                        </div>
                                    ) : (
                                        <select
                                            style={styles.select}
                                            value={selectedService}
                                            onChange={(e) => handleServiceChange(e.target.value)}
                                        >
                                            <option value="">选择微服务...</option>
                                            {serviceOptions.map(s => (
                                                <option key={s.name} value={s.name}>{s.name}</option>
                                            ))}
                                            <option value="__new__">+ 新建微服务</option>
                                        </select>
                                    )}
                                </div>

                                {(selectedService || isNewService) && (
                                    <div style={styles.archiveRow}>
                                        <label style={styles.label}>模块</label>
                                        {isNewModule || isNewService ? (
                                            <div style={styles.inputRow}>
                                                <input
                                                    style={styles.textInput}
                                                    value={newModuleName}
                                                    onChange={(e) => setNewModuleName(e.target.value)}
                                                    placeholder="输入新模块名称"
                                                    autoFocus={isNewModule}
                                                />
                                                {!isNewService && (
                                                    <button
                                                        style={styles.linkButton}
                                                        onClick={() => { setIsNewModule(false); setNewModuleName(''); }}
                                                    >
                                                        取消
                                                    </button>
                                                )}
                                            </div>
                                        ) : (
                                            <select
                                                style={styles.select}
                                                value={selectedModule}
                                                onChange={(e) => handleModuleChange(e.target.value)}
                                            >
                                                <option value="">选择模块...</option>
                                                {currentModules.map(m => (
                                                    <option key={m.name} value={m.name}>{m.name}</option>
                                                ))}
                                                <option value="__new__">+ 新建模块</option>
                                            </select>
                                        )}
                                    </div>
                                )}

                                {selectedFileName && (
                                    <div style={styles.fileHint}>
                                        将追加到: {selectedFileName}
                                    </div>
                                )}
                                {!selectedFileName && getEffectiveService() && getEffectiveModule() && (
                                    <div style={styles.fileHint}>
                                        将创建新文件
                                    </div>
                                )}
                            </div>
                        </div>
                    )}
                </div>

                <div style={styles.footer}>
                    {view === 'timeline' ? (
                        <button
                            onClick={handleAnalyze}
                            style={styles.primaryButton}
                            disabled={isLoading}
                        >
                            AI 解析
                        </button>
                    ) : isLoading ? (
                        <div style={styles.footerRow}>
                            <div style={styles.streamingHint}>
                                <div style={styles.loadingSpinner} />
                                <span>AI 正在生成总结...</span>
                            </div>
                        </div>
                    ) : (
                        <>
                            <button onClick={() => setView('timeline')} style={styles.secondaryButton}>上一步</button>
                            <button
                                onClick={handleArchive}
                                style={{
                                    ...styles.primaryButton,
                                    opacity: canArchive() ? 1 : 0.5,
                                    cursor: canArchive() ? 'pointer' : 'not-allowed',
                                }}
                                disabled={!canArchive()}
                            >
                                归档
                            </button>
                        </>
                    )}
                </div>
            </div>
        </div>
        </>,
        document.body
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.85)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 99999,
        backdropFilter: 'blur(4px)',
    },
    modal: {
        backgroundColor: '#252526',
        width: '600px',
        maxWidth: '90%',
        height: '80%',
        display: 'flex',
        flexDirection: 'column' as const,
        borderRadius: '8px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
        border: '1px solid #333',
    },
    header: {
        padding: '16px',
        borderBottom: '1px solid #333',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    title: {
        margin: 0,
        color: '#fff',
        fontSize: '16px',
    },
    closeButton: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        fontSize: '24px',
        cursor: 'pointer',
    },
    body: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '16px',
    },
    footer: {
        padding: '16px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
    },
    conclusionContainer: {
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: '100%',
    },
    textarea: {
        flex: 1,
        minHeight: '200px',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '12px',
        resize: 'none' as const,
        fontFamily: 'inherit',
        fontSize: '14px',
        lineHeight: '1.5',
    },
    primaryButton: {
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    footerRow: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        gap: '12px',
        width: '100%',
    },
    streamingHint: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        color: '#aaa',
        fontSize: '13px',
    },
    loadingSpinner: {
        width: '14px',
        height: '14px',
        border: '2px solid #444',
        borderTopColor: '#007acc',
        borderRadius: '50%',
        animation: 'sr-spin 0.8s linear infinite',
        flexShrink: 0,
    },
    secondaryButton: {
        padding: '8px 16px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    archiveSection: {
        borderTop: '1px solid #333',
        marginTop: '12px',
        paddingTop: '12px',
        flex: '0 0 auto',
    },
    archiveSectionTitle: {
        color: '#fff',
        fontSize: '13px',
        fontWeight: 'bold' as const,
        marginBottom: '8px',
    },
    archiveRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        marginBottom: '8px',
    },
    label: {
        color: '#aaa',
        fontSize: '13px',
        minWidth: '48px',
        flexShrink: 0,
    },
    select: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '6px 8px',
        fontSize: '13px',
    },
    inputRow: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
    },
    textInput: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #007acc',
        borderRadius: '4px',
        padding: '6px 8px',
        fontSize: '13px',
    },
    linkButton: {
        background: 'none',
        border: 'none',
        color: '#007acc',
        cursor: 'pointer',
        fontSize: '12px',
        flexShrink: 0,
    },
    fileHint: {
        color: '#888',
        fontSize: '12px',
        marginTop: '4px',
        fontStyle: 'italic',
    },
    indexPreview: {
        borderTop: '1px solid #333',
        padding: '8px 0',
        flex: '0 0 auto',
    },
    indexPreviewTitle: {
        color: '#aaa',
        fontSize: '12px',
        marginBottom: '6px',
    },
    indexPreviewHint: {
        color: '#666',
        fontSize: '11px',
    },
    indexPreviewEmpty: {
        color: '#665533',
        fontSize: '11px',
        fontStyle: 'italic' as const,
    },
    indexPreviewList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    indexRow: {
        display: 'flex',
        gap: '8px',
        fontSize: '12px',
        lineHeight: '1.4',
    },
    indexLabel: {
        color: '#4ec9b0',
        flexShrink: 0,
        minWidth: '42px',
    },
    indexValue: {
        color: '#ccc',
        wordBreak: 'break-all' as const,
    },
};

export default SessionReviewModal;
