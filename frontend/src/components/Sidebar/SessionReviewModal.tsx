import React, { useState, useEffect, useCallback, useRef } from 'react';
import ReactDOM from 'react-dom';
import { TimelineEvent, filterTimelineEvents, generateMarkdown } from '../../utils/timeline';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface ModuleInfo {
    name: string;
    fileName: string;
}

interface ServiceInfo {
    name: string;
    modules: ModuleInfo[];
}

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
    problem: string;
    onArchive: (params: ArchiveParams) => Promise<{ success: boolean; message: string }>;
}

// 归档选择状态（合并相关状态）
interface ArchiveSelection {
    serviceOptions: ServiceInfo[];
    selectedService: string;
    selectedModule: string;
    selectedFileName: string;
    isNewService: boolean;
    isNewModule: boolean;
    newServiceName: string;
    newModuleName: string;
}

const defaultArchiveSelection: ArchiveSelection = {
    serviceOptions: [],
    selectedService: '',
    selectedModule: '',
    selectedFileName: '',
    isNewService: false,
    isNewModule: false,
    newServiceName: '',
    newModuleName: '',
};

// Extract section preview from Markdown
function extractSectionPreview(md: string, ...headings: string[]): string {
    for (const h of headings) {
        const prefix = '## ' + h;
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
    }
    return '';
}

// Index Preview Component
const IndexPreview: React.FC<{ conclusion: string }> = ({ conclusion }) => {
    const phenomena = extractSectionPreview(conclusion, '问题现象', '问题描述', '现象', '故障现象');
    const keywords = extractSectionPreview(conclusion, '关键词', '关键字');
    const components = extractSectionPreview(conclusion, '涉及组件', '相关组件', '组件');

    const hasAny = !!(phenomena || keywords || components);

    if (!hasAny) return null;

    return (
        <div style={styles.indexPreview}>
            <div style={styles.indexPreviewTitle}>
                索引标签 <span style={styles.indexPreviewHint}>（自动提取，请保留 ## 关键词 / ## 问题现象 / ## 涉及组件 标题）</span>
            </div>
            <div style={styles.indexPreviewList}>
                {phenomena && <IndexRow label="现象" value={phenomena} />}
                {keywords && <IndexRow label="关键词" value={keywords} />}
                {components && <IndexRow label="组件" value={components} />}
            </div>
        </div>
    );
};

const IndexRow: React.FC<{ label: string; value: string }> = ({ label, value }) => (
    <div style={styles.indexRow}>
        <span style={styles.indexLabel}>{label}</span>
        <span style={styles.indexValue} title={value}>{value}</span>
    </div>
);

// Celebration Overlay Component
const CelebrationOverlay: React.FC = () => (
    <div style={styles.celebrationOverlay}>
        <style>{CELEBRATION_ANIMATIONS}</style>
        <div style={styles.celebrationContent}>
            <div style={styles.celebrationIcon}>🎉</div>
            <div style={styles.celebrationTitle}>知识已入库！</div>
            <div style={styles.celebrationSubtitle}>您的排查经验已成为团队财富</div>
            <FlyingEmojis type="stars" emojis={['⭐', '✨', '🌟', '💫', '⭐']} />
            <FlyingEmojis type="confetti" emojis={['🎊', '🎉', '🎊', '🎉', '🎊']} />
        </div>
    </div>
);

const FlyingEmojis: React.FC<{ type: 'stars' | 'confetti'; emojis: string[] }> = ({ type, emojis }) => {
    const containerStyle = type === 'stars' ? styles.celebrationStars : styles.celebrationConfetti;
    const emojiStyle = type === 'stars' ? styles.star : styles.confetti;
    const positions = ['10%', '30%', '50%', '70%', '90%'];

    return (
        <div style={containerStyle}>
            {emojis.map((emoji, i) => (
                <span
                    key={i}
                    style={{
                        ...emojiStyle,
                        left: positions[i],
                        animationDelay: `${i * 0.1}s`,
                    }}
                >
                    {emoji}
                </span>
            ))}
        </div>
    );
};

// Archive Section Component
const ArchiveSection: React.FC<{
    selection: ArchiveSelection;
    onServiceChange: (value: string) => void;
    onModuleChange: (value: string) => void;
    onNewServiceNameChange: (value: string) => void;
    onNewModuleNameChange: (value: string) => void;
    onCancelNewService: () => void;
    onCancelNewModule: () => void;
    getEffectiveService: () => string;
    getEffectiveModule: () => string;
}> = ({
    selection,
    onServiceChange,
    onModuleChange,
    onNewServiceNameChange,
    onNewModuleNameChange,
    onCancelNewService,
    onCancelNewModule,
    getEffectiveService,
    getEffectiveModule,
}) => {
    const currentModules = selection.serviceOptions.find(s => s.name === selection.selectedService)?.modules || [];

    return (
        <div style={styles.archiveSection}>
            <div style={styles.archiveSectionTitle}>归档位置</div>

            {/* Service Row */}
            <div style={styles.archiveRow}>
                <label style={styles.label}>微服务</label>
                {selection.isNewService ? (
                    <div style={styles.inputRow}>
                        <input
                            style={styles.textInput}
                            value={selection.newServiceName}
                            onChange={(e) => onNewServiceNameChange(e.target.value)}
                            placeholder="输入新微服务名称"
                            autoFocus
                        />
                        <button style={styles.linkButton} onClick={onCancelNewService}>
                            取消
                        </button>
                    </div>
                ) : (
                    <select
                        style={styles.select}
                        value={selection.selectedService}
                        onChange={(e) => onServiceChange(e.target.value)}
                    >
                        <option value="">选择微服务...</option>
                        {selection.serviceOptions.map(s => (
                            <option key={s.name} value={s.name}>{s.name}</option>
                        ))}
                        <option value="__new__">+ 新建微服务</option>
                    </select>
                )}
            </div>

            {/* Module Row */}
            {(selection.selectedService || selection.isNewService) && (
                <div style={styles.archiveRow}>
                    <label style={styles.label}>模块</label>
                    {selection.isNewModule || selection.isNewService ? (
                        <div style={styles.inputRow}>
                            <input
                                style={styles.textInput}
                                value={selection.newModuleName}
                                onChange={(e) => onNewModuleNameChange(e.target.value)}
                                placeholder="输入新模块名称"
                                autoFocus={selection.isNewModule}
                            />
                            {!selection.isNewService && (
                                <button style={styles.linkButton} onClick={onCancelNewModule}>
                                    取消
                                </button>
                            )}
                        </div>
                    ) : (
                        <select
                            style={styles.select}
                            value={selection.selectedModule}
                            onChange={(e) => onModuleChange(e.target.value)}
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

            {/* File Hint */}
            {selection.selectedFileName && (
                <div style={styles.fileHint}>将追加到: {selection.selectedFileName}</div>
            )}
            {!selection.selectedFileName && getEffectiveService() && getEffectiveModule() && (
                <div style={styles.fileHint}>将创建新文件</div>
            )}
        </div>
    );
};

// Main Component
const SessionReviewModal: React.FC<SessionReviewModalProps> = ({ isOpen, onClose, rootCause, problem, onArchive }) => {
    // View state
    const [view, setView] = useState<'timeline' | 'conclusion'>('timeline');
    const [conclusion, setConclusion] = useState('');
    const [markdownContent, setMarkdownContent] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    // Archive state
    const [isArchiving, setIsArchiving] = useState(false);
    const [archiveError, setArchiveError] = useState<string | null>(null);
    const [archiveSuccess, setArchiveSuccess] = useState(false);

    // Archive selection (merged state)
    const [selection, setSelection] = useState<ArchiveSelection>(defaultArchiveSelection);

    // Derived values
    const getEffectiveService = useCallback(() => {
        return selection.isNewService ? selection.newServiceName.trim() : selection.selectedService;
    }, [selection.isNewService, selection.newServiceName, selection.selectedService]);

    const getEffectiveModule = useCallback(() => {
        return (selection.isNewModule || selection.isNewService)
            ? selection.newModuleName.trim()
            : selection.selectedModule;
    }, [selection.isNewModule, selection.isNewService, selection.newModuleName, selection.selectedModule]);

    const canArchive = useCallback(() => {
        return !!getEffectiveService() && !!getEffectiveModule();
    }, [getEffectiveService, getEffectiveModule]);

    // Reset all state when modal opens
    useEffect(() => {
        if (isOpen) {
            setView('timeline');
            setConclusion('');
            setMarkdownContent('');
            setIsLoading(false);
            setIsArchiving(false);
            setArchiveError(null);
            setArchiveSuccess(false);
            setSelection(defaultArchiveSelection);

            // Load data
            loadTimeline();
            loadCatalogServices();
        }
    }, [isOpen]);

    // Auto-switch out of new-service mode only when catalog first loads
    // (services transitions from 0 to >0). Do NOT re-trigger on user action.
    const prevServiceCountRef = useRef(0);
    useEffect(() => {
        const count = selection.serviceOptions.length;
        if (isOpen && prevServiceCountRef.current === 0 && count > 0 && selection.isNewService) {
            setSelection(prev => ({ ...prev, isNewService: false }));
        }
        prevServiceCountRef.current = count;
    }, [isOpen, selection.serviceOptions.length]);

    // Load catalog services
    const loadCatalogServices = async () => {
        try {
            // @ts-ignore
            const services = await window.go?.main?.App?.GetCatalogServices?.();
            if (Array.isArray(services)) {
                setSelection(prev => ({
                    ...prev,
                    serviceOptions: services,
                    // Only auto-enter new-service mode if catalog is empty
                    // AND user hasn't already started interacting
                    isNewService: services.length === 0 && !prev.selectedService,
                }));
            }
        } catch (e) {
            console.error("[SessionReviewModal] Error loading catalog services:", e);
        }
    };

    // Load timeline
    const loadTimeline = async () => {
        try {
            // @ts-ignore
            const sessionData = await window.go?.main?.App?.GetSessionTimeline?.();

            const effectiveProblem = sessionData?.problem || problem || "未指定";

            if (sessionData?.timeline && Array.isArray(sessionData.timeline)) {
                const filtered = filterTimelineEvents(sessionData.timeline);
                const md = generateMarkdown(filtered, effectiveProblem, rootCause);
                setMarkdownContent(md);
            } else {
                setMarkdownContent(`# 排查会话记录\n\n**原始问题:** ${effectiveProblem}\n\n**根本原因:** ${rootCause}\n\n（无记录）`);
            }
        } catch (e) {
            console.error("[SessionReviewModal] Error loading timeline:", e);
        }
    };

    // Handle service change
    const handleServiceChange = useCallback((value: string) => {
        if (value === '__new__') {
            setSelection(prev => ({
                ...prev,
                isNewService: true,
                isNewModule: false,
                selectedService: '',
                selectedModule: '',
                selectedFileName: '',
            }));
        } else {
            setSelection(prev => ({
                ...prev,
                isNewService: false,
                selectedService: value,
                selectedModule: '',
                selectedFileName: '',
                isNewModule: false,
            }));
        }
    }, []);

    // Handle module change
    const handleModuleChange = useCallback((value: string) => {
        if (value === '__new__') {
            setSelection(prev => ({
                ...prev,
                isNewModule: true,
                selectedModule: '',
                selectedFileName: '',
            }));
        } else {
            const svc = selection.serviceOptions.find(s => s.name === selection.selectedService);
            const mod = svc?.modules.find(m => m.name === value);
            setSelection(prev => ({
                ...prev,
                isNewModule: false,
                selectedModule: value,
                selectedFileName: mod?.fileName || '',
            }));
        }
    }, [selection.serviceOptions, selection.selectedService]);

    // Handle analyze
    const handleAnalyze = async () => {
        setIsLoading(true);
        setConclusion('');
        setView('conclusion');

        let streamConclusion = '';
        let cancelFunctions: (() => void)[] = [];

        try {
            if (EventsOn) {
                cancelFunctions.push(
                    EventsOn("conclusion:token", (data: { token?: string }) => {
                        const token = data?.token || '';
                        if (token) {
                            streamConclusion += token;
                            setConclusion(streamConclusion);
                        }
                    })
                );

                cancelFunctions.push(
                    EventsOn("conclusion:error", (data: { error?: string }) => {
                        if (!streamConclusion) {
                            setConclusion(`生成总结失败: ${data?.error || 'Unknown error'}`);
                        }
                    })
                );

                cancelFunctions.push(
                    EventsOn("conclusion:done", (data: { conclusion?: string }) => {
                        if (data?.conclusion) {
                            setConclusion(data.conclusion);
                        }
                    })
                );
            }

            // @ts-ignore
            if (window.go?.main?.App?.StreamConclusion) {
                // @ts-ignore
                await window.go.main.App.StreamConclusion(markdownContent, rootCause);
            // @ts-ignore
            } else if (window.go?.main?.App?.GenerateConclusionWithContext) {
                // @ts-ignore
                const result = await window.go.main.App.GenerateConclusionWithContext(markdownContent, rootCause);
                setConclusion(result);
            }
        } catch (e) {
            console.error(e);
            if (!streamConclusion) {
                setConclusion(`生成总结失败: ${e}`);
            }
        } finally {
            cancelFunctions.forEach(fn => fn?.());
            setIsLoading(false);
        }
    };

    // Handle archive
    const handleArchive = async () => {
        setIsArchiving(true);
        setArchiveError(null);
        try {
            const result = await onArchive({
                conclusion,
                service: getEffectiveService(),
                module: getEffectiveModule(),
                targetFile: selection.selectedFileName,
            });
            if (result.success) {
                setArchiveSuccess(true);
                setTimeout(onClose, 2500);
            } else {
                setArchiveError(result.message);
            }
        } catch (e) {
            setArchiveError(`归档异常: ${e}`);
        } finally {
            setIsArchiving(false);
        }
    };

    if (!isOpen) return null;

    return ReactDOM.createPortal(
        <>
            <style>{SPINNER_ANIMATION}</style>
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
                            <textarea
                                value={markdownContent}
                                onChange={(e) => setMarkdownContent(e.target.value)}
                                style={styles.textarea}
                                placeholder="正在生成排查记录..."
                            />
                        ) : (
                            <>
                                <div style={styles.textareaWrapper}>
                                    <textarea
                                        value={conclusion}
                                        onChange={(e) => setConclusion(e.target.value)}
                                        style={styles.textareaInWrapper}
                                        placeholder="AI 正在生成总结..."
                                    />
                                </div>
                                <div style={styles.bottomSection}>
                                    {conclusion && <IndexPreview conclusion={conclusion} />}
                                    <ArchiveSection
                                        selection={selection}
                                        onServiceChange={handleServiceChange}
                                        onModuleChange={handleModuleChange}
                                        onNewServiceNameChange={(v) => setSelection(prev => ({ ...prev, newServiceName: v }))}
                                        onNewModuleNameChange={(v) => setSelection(prev => ({ ...prev, newModuleName: v }))}
                                        onCancelNewService={() => setSelection(prev => ({ ...prev, isNewService: false, newServiceName: '' }))}
                                        onCancelNewModule={() => setSelection(prev => ({ ...prev, isNewModule: false, newModuleName: '' }))}
                                        getEffectiveService={getEffectiveService}
                                        getEffectiveModule={getEffectiveModule}
                                    />
                                </div>
                            </>
                        )}
                    </div>

                    <div style={styles.footer}>
                        {view === 'timeline' ? (
                            <button onClick={handleAnalyze} style={styles.primaryButton} disabled={isLoading}>
                                AI 解析
                            </button>
                        ) : isLoading ? (
                            <div style={styles.streamingHint}>
                                <div style={styles.loadingSpinner} />
                                <span>AI 正在生成总结...</span>
                            </div>
                        ) : archiveSuccess ? (
                            <div style={styles.successHint}>
                                <span style={styles.successIcon}>✓</span>
                                <span>归档成功</span>
                            </div>
                        ) : (
                            <>
                                {archiveError && <div style={styles.errorHint}>{archiveError}</div>}
                                <button
                                    onClick={() => setView('timeline')}
                                    style={styles.secondaryButton}
                                    disabled={isArchiving}
                                >
                                    上一步
                                </button>
                                <button
                                    onClick={handleArchive}
                                    style={{
                                        ...styles.primaryButton,
                                        opacity: canArchive() && !isArchiving ? 1 : 0.5,
                                        cursor: canArchive() && !isArchiving ? 'pointer' : 'not-allowed',
                                    }}
                                    disabled={!canArchive() || isArchiving}
                                >
                                    {isArchiving ? (
                                        <span style={styles.archiveLoadingText}>
                                            <div style={styles.loadingSpinner} />
                                            归档中...
                                        </span>
                                    ) : '归档'}
                                </button>
                            </>
                        )}
                    </div>

                    {archiveSuccess && <CelebrationOverlay />}
                </div>
            </div>
        </>,
        document.body
    );
};

// Animation CSS
const SPINNER_ANIMATION = `@keyframes sr-spin { to { transform: rotate(360deg); } }`;
const CELEBRATION_ANIMATIONS = `
    @keyframes celebration-pop {
        0% { transform: scale(0.5); opacity: 0; }
        50% { transform: scale(1.1); }
        100% { transform: scale(1); opacity: 1; }
    }
    @keyframes celebration-stars {
        0% { transform: translateY(0) rotate(0deg); opacity: 1; }
        100% { transform: translateY(-60px) rotate(180deg); opacity: 0; }
    }
    @keyframes celebration-confetti {
        0% { transform: translateY(0) rotate(0deg); opacity: 1; }
        100% { transform: translateY(80px) rotate(360deg); opacity: 0; }
    }
`;

// Styles
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
        position: 'relative' as const,
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
        overflow: 'hidden' as const,
        padding: '16px',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    footer: {
        padding: '16px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
    },
    textarea: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '12px',
        resize: 'none' as const,
        fontFamily: 'inherit',
        fontSize: '14px',
        lineHeight: '1.5',
        boxSizing: 'border-box' as const,
    },
    textareaWrapper: {
        flex: 1,
        minHeight: '200px',
        marginBottom: '12px',
        overflow: 'hidden' as const,
    },
    textareaInWrapper: {
        width: '100%',
        height: '100%',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '12px',
        resize: 'none' as const,
        fontFamily: 'inherit',
        fontSize: '14px',
        lineHeight: '1.5',
        boxSizing: 'border-box' as const,
    },
    bottomSection: {
        flexShrink: 0,
        maxHeight: '40%',
        overflowY: 'auto' as const,
        paddingRight: '8px',
    },
    primaryButton: {
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    secondaryButton: {
        padding: '8px 16px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
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
    successHint: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        color: '#4caf50',
        fontSize: '14px',
        fontWeight: 'bold' as const,
    },
    successIcon: {
        fontSize: '18px',
    },
    errorHint: {
        flex: 1,
        color: '#f44336',
        fontSize: '13px',
        textAlign: 'left' as const,
    },
    archiveLoadingText: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
    },
    // Archive section styles
    archiveSection: {
        borderTop: '1px solid #333',
        marginTop: '8px',
        paddingTop: '8px',
    },
    archiveSectionTitle: {
        color: '#fff',
        fontSize: '12px',
        fontWeight: 'bold' as const,
        marginBottom: '6px',
    },
    archiveRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        marginBottom: '6px',
    },
    label: {
        color: '#aaa',
        fontSize: '12px',
        minWidth: '40px',
        flexShrink: 0,
    },
    select: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '4px 6px',
        fontSize: '12px',
        height: '28px',
    },
    inputRow: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
    },
    textInput: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #007acc',
        borderRadius: '4px',
        padding: '4px 6px',
        fontSize: '12px',
        height: '28px',
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
        fontStyle: 'italic' as const,
    },
    // Index preview styles
    indexPreview: {
        borderTop: '1px solid #333',
        padding: '6px 0',
    },
    indexPreviewTitle: {
        color: '#aaa',
        fontSize: '11px',
        marginBottom: '4px',
    },
    indexPreviewHint: {
        color: '#666',
        fontSize: '10px',
    },
    indexPreviewList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '2px',
    },
    indexRow: {
        display: 'flex',
        gap: '6px',
        fontSize: '11px',
    },
    indexLabel: {
        color: '#4ec9b0',
        flexShrink: 0,
        minWidth: '36px',
    },
    indexValue: {
        color: '#ccc',
        wordBreak: 'break-all' as const,
    },
    // Celebration styles
    celebrationOverlay: {
        position: 'absolute' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.9)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        borderRadius: '8px',
        zIndex: 10,
    },
    celebrationContent: {
        textAlign: 'center' as const,
        animation: 'celebration-pop 0.4s ease-out forwards',
    },
    celebrationIcon: {
        fontSize: '64px',
        marginBottom: '16px',
        animation: 'celebration-pop 0.4s ease-out forwards',
    },
    celebrationTitle: {
        fontSize: '28px',
        fontWeight: 'bold' as const,
        color: '#4caf50',
        marginBottom: '8px',
        animation: 'celebration-pop 0.5s ease-out 0.1s forwards',
        opacity: 0,
    },
    celebrationSubtitle: {
        fontSize: '16px',
        color: '#aaa',
        marginBottom: '24px',
        animation: 'celebration-pop 0.5s ease-out 0.2s forwards',
        opacity: 0,
    },
    celebrationStars: {
        position: 'absolute' as const,
        top: '20%',
        left: 0, right: 0,
        height: '60px',
    },
    star: {
        position: 'absolute' as const,
        fontSize: '24px',
        animation: 'celebration-stars 1.5s ease-out forwards',
    },
    celebrationConfetti: {
        position: 'absolute' as const,
        bottom: '20%',
        left: 0, right: 0,
        height: '80px',
    },
    confetti: {
        position: 'absolute' as const,
        fontSize: '20px',
        animation: 'celebration-confetti 1.5s ease-out forwards',
    },
};

export default SessionReviewModal;