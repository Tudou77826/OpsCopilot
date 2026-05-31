import React, { useState, useEffect, useCallback, useMemo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
    TbSearch,
    TbX,
    TbChevronDown,
    TbChevronRight,
    TbStarFilled,
    TbArrowLeft,
    TbRefresh,
} from 'react-icons/tb';
import {
    GetKnowledgeTree,
    GetKnowledgeScenarioContent,
    GetPatchFeedback,
    RatePatch,
    ReportPatchIssue,
    UpdatePatchIssueStatus,
} from '../../../wailsjs/go/main/App';

// --- Types ---

interface ScenarioEntry {
    title: string;
    file: string;
    lineStart: number;
    lineEnd: number;
    phenomena: string;
    keywords: string[];
    components: string[];
    type: string; // "sop" | "archive"
}

interface ModuleEntry {
    name: string;
    scenarios: ScenarioEntry[];
}

interface ServiceEntry {
    name: string;
    modules: ModuleEntry[];
}

interface Catalog {
    version: number;
    services: ServiceEntry[];
}

interface Rating {
    score: number;
    comment: string;
    timestamp: string;
}

interface Issue {
    id: string;
    type: string;       // "bug" | "outdated" | "suggestion"
    priority: string;   // "high" | "medium" | "low"
    title: string;
    description: string;
    reporter: string;
    status: string;     // "open" | "resolved" | "wontfix"
    timestamp: string;
}

interface UserFeedback {
    patchId: string;
    user: string;
    rating?: Rating;
    issues?: Issue[];
}

type ViewMode = 'tree' | 'detail' | 'newIssue';
type DetailTab = 'content' | 'rating' | 'issues';

// --- Helpers ---

const patchIdFromEntry = (entry: ScenarioEntry): string => {
    // Extract a stable ID from file + lineStart (matching patch ID format)
    return `${entry.file}#L${entry.lineStart}`;
};

const avgRating = (feedbacks: UserFeedback[]): { avg: number; count: number } => {
    let sum = 0, count = 0;
    for (const fb of feedbacks) {
        if (fb.rating && fb.rating.score > 0) {
            sum += fb.rating.score;
            count++;
        }
    }
    return { avg: count > 0 ? sum / count : 0, count };
};

const formatDate = (ts: string): string => {
    if (!ts) return '';
    try {
        const d = new Date(ts);
        return d.toLocaleDateString('zh-CN');
    } catch {
        return ts;
    }
};

// --- Badge Components ---

const TypeBadge: React.FC<{ type: string }> = ({ type }) => {
    const colors: Record<string, string> = {
        bug: '#e74c3c',
        outdated: '#f39c12',
        suggestion: '#3498db',
    };
    const labels: Record<string, string> = {
        bug: '错误',
        outdated: '过时',
        suggestion: '建议',
    };
    return (
        <span style={{
            ...styles.badge,
            backgroundColor: colors[type] || '#666',
        }}>
            {labels[type] || type}
        </span>
    );
};

const PriorityBadge: React.FC<{ priority: string }> = ({ priority }) => {
    const colors: Record<string, string> = {
        high: '#e74c3c',
        medium: '#f39c12',
        low: '#888',
    };
    const labels: Record<string, string> = {
        high: '高',
        medium: '中',
        low: '低',
    };
    return (
        <span style={{
            ...styles.badge,
            backgroundColor: colors[priority] || '#666',
        }}>
            {labels[priority] || priority}
        </span>
    );
};

const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
    const colors: Record<string, string> = {
        open: '#f39c12',
        resolved: '#27ae60',
        wontfix: '#888',
    };
    const labels: Record<string, string> = {
        open: '待处理',
        resolved: '已解决',
        wontfix: '不处理',
    };
    return (
        <span style={{
            ...styles.badge,
            backgroundColor: colors[status] || '#666',
        }}>
            {labels[status] || status}
        </span>
    );
};

const DocTypeBadge: React.FC<{ type: string }> = ({ type }) => {
    return (
        <span style={{
            ...styles.badge,
            backgroundColor: type === 'archive' ? '#6c5ce7' : '#00b894',
            fontSize: '10px',
            padding: '1px 6px',
        }}>
            {type === 'archive' ? '归档' : 'SOP'}
        </span>
    );
};

// --- Star Rating Component ---

const StarRating: React.FC<{
    score: number;
    onChange?: (score: number) => void;
    size?: number;
}> = ({ score, onChange, size = 18 }) => {
    const [hover, setHover] = useState(0);
    return (
        <span style={{ display: 'inline-flex', gap: '2px' }}>
            {[1, 2, 3, 4, 5].map(i => (
                <span
                    key={i}
                    style={{
                        cursor: onChange ? 'pointer' : 'default',
                        lineHeight: 1,
                    }}
                    onClick={() => onChange?.(i)}
                    onMouseEnter={() => onChange && setHover(i)}
                    onMouseLeave={() => setHover(0)}
                >
                    {TbStarFilled({
                        size: size,
                        style: {
                            color: i <= (hover || score) ? '#f1c40f' : '#555',
                        },
                    })}
                </span>
            ))}
        </span>
    );
};

// --- Main Component ---

const KnowledgeBrowser: React.FC = () => {
    const [view, setView] = useState<ViewMode>('tree');
    const [detailTab, setDetailTab] = useState<DetailTab>('content');
    const [searchQuery, setSearchQuery] = useState('');
    const [catalog, setCatalog] = useState<Catalog | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    // Tree state
    const [expandedServices, setExpandedServices] = useState<Set<string>>(new Set());
    const [expandedModules, setExpandedModules] = useState<Set<string>>(new Set());

    // Detail state
    const [selectedEntry, setSelectedEntry] = useState<ScenarioEntry | null>(null);
    const [selectedService, setSelectedService] = useState('');
    const [selectedModule, setSelectedModule] = useState('');
    const [fileContent, setFileContent] = useState('');
    const [feedbackList, setFeedbackList] = useState<UserFeedback[]>([]);
    const [detailLoading, setDetailLoading] = useState(false);
    const [submitting, setSubmitting] = useState(false);

    // Rating state
    const [myScore, setMyScore] = useState(0);
    const [myComment, setMyComment] = useState('');
    const [ratingSubmitted, setRatingSubmitted] = useState(false);

    // New Issue state
    const [issueType, setIssueType] = useState('bug');
    const [issuePriority, setIssuePriority] = useState('medium');
    const [issueTitle, setIssueTitle] = useState('');
    const [issueDesc, setIssueDesc] = useState('');

    // Load catalog
    const loadCatalog = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            const data = await GetKnowledgeTree();
            const parsed = JSON.parse(data) as Catalog;
            setCatalog(parsed);
        } catch (e: any) {
            setError('加载知识库失败: ' + (e.message || e));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => { loadCatalog(); }, [loadCatalog]);

    // Toggle tree nodes
    const toggleService = (name: string) => {
        setExpandedServices(prev => {
            const next = new Set(prev);
            next.has(name) ? next.delete(name) : next.add(name);
            return next;
        });
    };

    const toggleModule = (key: string) => {
        setExpandedModules(prev => {
            const next = new Set(prev);
            next.has(key) ? next.delete(key) : next.add(key);
            return next;
        });
    };

    // Open detail view
    const openDetail = async (entry: ScenarioEntry, service: string, module: string) => {
        setSelectedEntry(entry);
        setSelectedService(service);
        setSelectedModule(module);
        setView('detail');
        setDetailTab('content');
        setMyScore(0);
        setMyComment('');
        setRatingSubmitted(false);
        setDetailLoading(true);

        // Load scenario content (only the selected section, not the whole file)
        try {
            const resp = JSON.parse(await GetKnowledgeScenarioContent(entry.file, entry.lineStart, entry.lineEnd));
            if (resp.success) {
                setFileContent(resp.content || '');
            } else {
                setFileContent('加载失败: ' + (resp.error || '未知错误'));
            }
        } catch (e: any) {
            setFileContent('加载失败');
        }

        // Load feedback
        const pid = patchIdFromEntry(entry);
        try {
            const resp = JSON.parse(await GetPatchFeedback(pid));
            setFeedbackList(resp.feedback || []);
        } catch {
            setFeedbackList([]);
        }

        setDetailLoading(false);
    };

    // Submit rating
    const submitRating = async () => {
        if (myScore < 1 || !selectedEntry || submitting) return;
        setSubmitting(true);
        try {
            const pid = patchIdFromEntry(selectedEntry);
            await RatePatch(pid, myScore, myComment);
            setRatingSubmitted(true);
            // Refresh feedback
            const resp = JSON.parse(await GetPatchFeedback(pid));
            setFeedbackList(resp.feedback || []);
        } catch (e: any) {
            setError('评分失败: ' + (e.message || e));
        } finally {
            setSubmitting(false);
        }
    };

    // Submit issue
    const submitIssue = async () => {
        if (!issueTitle.trim() || !selectedEntry || submitting) return;
        setSubmitting(true);
        try {
            const pid = patchIdFromEntry(selectedEntry);
            await ReportPatchIssue(pid, issueType, issuePriority, issueTitle, issueDesc);
            // Refresh feedback
            const resp = JSON.parse(await GetPatchFeedback(pid));
            setFeedbackList(resp.feedback || []);
            setView('detail');
            setIssueTitle('');
            setIssueDesc('');
        } catch (e: any) {
            setError('Issue 提交失败: ' + (e.message || e));
        } finally {
            setSubmitting(false);
        }
    };

    // Update issue status
    const updateIssueStatus = async (issueId: string, status: string) => {
        if (!selectedEntry) return;
        try {
            const pid = patchIdFromEntry(selectedEntry);
            await UpdatePatchIssueStatus(pid, issueId, status);
            const resp = JSON.parse(await GetPatchFeedback(pid));
            setFeedbackList(resp.feedback || []);
        } catch (e: any) {
            setError('状态更新失败: ' + (e.message || e));
        }
    };

    // --- Search filter ---
    const matchesSearch = (entry: ScenarioEntry, serviceName: string, moduleName: string): boolean => {
        if (!searchQuery.trim()) return true;
        const q = searchQuery.toLowerCase();
        return (
            entry.title.toLowerCase().includes(q) ||
            entry.phenomena?.toLowerCase().includes(q) ||
            entry.keywords?.some(kw => kw.toLowerCase().includes(q)) ||
            entry.components?.some(c => c.toLowerCase().includes(q)) ||
            serviceName.toLowerCase().includes(q) ||
            moduleName.toLowerCase().includes(q) ||
            (entry.type === 'archive' ? '归档' : 'sop').includes(q)
        );
    };

    // Memoized search results to avoid O(n*m*k) recomputation on every render
    const searchResults = useMemo(() => {
        if (!catalog || !searchQuery.trim()) return null;
        const results: { svc: string; mod: string; entry: ScenarioEntry }[] = [];
        for (const svc of catalog.services) {
            for (const mod of svc.modules) {
                for (const entry of mod.scenarios) {
                    if (matchesSearch(entry, svc.name, mod.name)) {
                        results.push({ svc: svc.name, mod: mod.name, entry });
                    }
                }
            }
        }
        return results;
    }, [searchQuery, catalog]);

    // --- Render: Tree View ---
    const renderTree = () => {
        if (loading) return <div style={styles.emptyState}>加载中...</div>;
        if (error && !catalog) return <div style={{ ...styles.emptyState, color: '#e74c3c' }}>{error}</div>;
        if (!catalog || !catalog.services?.length) {
            return <div style={styles.emptyState}>知识库为空，请先归档排查记录</div>;
        }

        const hasQuery = searchQuery.trim().length > 0;

        return (
            <div style={styles.treeContainer}>
                {/* Search box */}
                <div style={styles.searchBox}>
                    {TbSearch({ size: 14, style: styles.searchIcon })}
                    <input
                        style={styles.searchInput}
                        placeholder="搜索知识库..."
                        value={searchQuery}
                        onChange={e => setSearchQuery(e.target.value)}
                    />
                    <button style={styles.searchClear} onClick={loadCatalog} title="刷新">
                        {TbRefresh({ size: 14 })}
                    </button>
                    {searchQuery && (
                        <button style={styles.searchClear} onClick={() => setSearchQuery('')}>{TbX({ size: 12 })}</button>
                    )}
                </div>

                {/* Stats summary */}
                {!hasQuery && (
                    <div style={styles.statsRow}>
                        <span>{catalog.services.length} 个服务</span>
                        <span style={styles.statsDot}>·</span>
                        <span>{catalog.services.reduce((s, svc) => s + svc.modules.length, 0)} 个模块</span>
                        <span style={styles.statsDot}>·</span>
                        <span>{catalog.services.reduce((s, svc) => s + svc.modules.reduce((s2, m) => s2 + m.scenarios.length, 0), 0)} 个场景</span>
                    </div>
                )}

                {hasQuery && searchResults && (
                    searchResults.length === 0
                        ? <div style={styles.emptyState}>没有匹配「{searchQuery}」的记录</div>
                        : <div style={styles.searchResults}>
                            <div style={styles.searchResultCount}>{searchResults.length} 条匹配</div>
                            {searchResults.map(({ svc, mod, entry }, idx) => (
                                <div key={idx} style={styles.scenarioCard} onClick={() => openDetail(entry, svc, mod)}>
                                    <div style={styles.scenarioHeader}>
                                        <DocTypeBadge type={entry.type} />
                                        <span style={styles.scenarioTitle} title={entry.title}>
                                            {entry.title.length > 50 ? entry.title.slice(0, 50) + '...' : entry.title}
                                        </span>
                                    </div>
                                    <div style={styles.scenarioMetaLine}>
                                        <span style={styles.scenarioServiceTag}>{svc}</span>
                                        <span style={styles.scenarioModTag}>{mod}</span>
                                    </div>
                                    {entry.phenomena && (
                                        <div style={styles.scenarioPhenomena}>
                                            {entry.phenomena.length > 100 ? entry.phenomena.slice(0, 100) + '...' : entry.phenomena}
                                        </div>
                                    )}
                                    {entry.keywords?.length > 0 && (
                                        <div style={styles.keywordList}>
                                            {entry.keywords.slice(0, 8).map((kw, ki) => (
                                                <span key={ki} style={styles.keyword}>{kw}</span>
                                            ))}
                                            {entry.keywords.length > 8 && (
                                                <span style={styles.keywordMore}>+{entry.keywords.length - 8}</span>
                                            )}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                )}

                {!hasQuery && catalog.services.map(svc => (
                    <div key={svc.name}>
                        <div className="kb-tree-node" style={styles.treeNode} onClick={() => toggleService(svc.name)}>
                            {expandedServices.has(svc.name)
                                ? TbChevronDown({ size: 12, style: styles.expandIcon })
                                : TbChevronRight({ size: 12, style: styles.expandIcon })
                            }
                            <span style={styles.serviceName}>{svc.name}</span>
                            <span style={styles.countBadge}>
                                {svc.modules.reduce((sum, m) => sum + m.scenarios.length, 0)}
                            </span>
                        </div>
                        {expandedServices.has(svc.name) && svc.modules.map(mod => {
                            const modKey = `${svc.name}/${mod.name}`;
                            return (
                                <div key={modKey}>
                                    <div className="kb-tree-node" style={{ ...styles.treeNode, paddingLeft: '20px' }} onClick={() => toggleModule(modKey)}>
                                        {expandedModules.has(modKey)
                                            ? TbChevronDown({ size: 12, style: styles.expandIcon })
                                            : TbChevronRight({ size: 12, style: styles.expandIcon })
                                        }
                                        <span style={styles.moduleName}>{mod.name}</span>
                                        <span style={styles.countBadge}>{mod.scenarios.length}</span>
                                    </div>
                                    {expandedModules.has(modKey) && mod.scenarios.map((entry, idx) => (
                                        <div
                                            key={idx}
                                            className="kb-scenario-card"
                                            style={styles.scenarioCard}
                                            onClick={() => openDetail(entry, svc.name, mod.name)}
                                        >
                                            <div style={styles.scenarioHeader}>
                                                <DocTypeBadge type={entry.type} />
                                                <span style={styles.scenarioTitle} title={entry.title}>
                                                    {entry.title.length > 60 ? entry.title.slice(0, 60) + '...' : entry.title}
                                                </span>
                                            </div>
                                            {entry.keywords?.length > 0 && (
                                                <div style={styles.keywordList}>
                                                    {entry.keywords.slice(0, 8).map((kw, ki) => (
                                                        <span key={ki} style={styles.keyword}>{kw}</span>
                                                    ))}
                                                    {entry.keywords.length > 8 && (
                                                        <span style={styles.keywordMore}>+{entry.keywords.length - 8}</span>
                                                    )}
                                                </div>
                                            )}
                                        </div>
                                    ))}
                                </div>
                            );
                        })}
                    </div>
                ))}
            </div>
        );
    };

    // --- Render: Detail View ---
    const renderDetail = () => {
        if (!selectedEntry) return null;

        if (detailLoading) {
            return <div style={styles.detailContainer}><div style={styles.emptyState}>加载中...</div></div>;
        }

        const allFeedback = feedbackList;
        const { avg, count } = avgRating(allFeedback);
        const allIssues = allFeedback.flatMap(fb => fb.issues || []);
        const openCount = allIssues.filter(i => i.status === 'open').length;

        return (
            <div style={styles.detailContainer}>
                {/* Header */}
                <div style={styles.detailHeader}>
                    <button className="kb-back-btn" style={styles.backButton} onClick={() => { setView('tree'); setError(''); }}>
                        {TbArrowLeft({ size: 16 })}返回
                    </button>
                    <span style={styles.detailTitle} title={selectedEntry.title}>
                        {selectedEntry.title.length > 80
                            ? selectedEntry.title.slice(0, 80) + '...'
                            : selectedEntry.title}
                    </span>
                </div>

                {/* Metadata */}
                <div style={styles.metaRow}>
                    <span style={styles.metaItem}>
                        <span style={styles.metaLabel}>服务:</span> {selectedService}
                    </span>
                    <span style={styles.metaItem}>
                        <span style={styles.metaLabel}>模块:</span> {selectedModule}
                    </span>
                    <DocTypeBadge type={selectedEntry.type} />
                    {count > 0 && (
                        <span style={styles.metaRating}>
                            <StarRating score={Math.round(avg)} size={12} /> {avg.toFixed(1)}
                        </span>
                    )}
                    {openCount > 0 && (
                        <span style={styles.metaIssueCount}>{openCount} issue</span>
                    )}
                </div>

                {/* Sub tabs */}
                <div style={styles.subTabBar}>
                    <button
                        className="kb-sub-tab"
                        style={{ ...styles.subTab, ...(detailTab === 'content' ? styles.subTabActive : {}) }}
                        onClick={() => setDetailTab('content')}
                    >
                        内容
                    </button>
                    <button
                        className="kb-sub-tab"
                        style={{ ...styles.subTab, ...(detailTab === 'rating' ? styles.subTabActive : {}) }}
                        onClick={() => setDetailTab('rating')}
                    >
                        评分 {count > 0 ? `(${count})` : ''}
                    </button>
                    <button
                        className="kb-sub-tab"
                        style={{ ...styles.subTab, ...(detailTab === 'issues' ? styles.subTabActive : {}) }}
                        onClick={() => setDetailTab('issues')}
                    >
                        Issue {openCount > 0 ? `(${openCount})` : ''}
                    </button>
                </div>

                {/* Tab content */}
                <div style={styles.tabContentArea}>
                    {/* Content tab */}
                    {detailTab === 'content' && (
                        <div className="message-markdown-content">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>
                                {fileContent}
                            </ReactMarkdown>
                        </div>
                    )}

                    {/* Rating tab */}
                    {detailTab === 'rating' && (
                        <div style={styles.tabInner}>
                            {count > 0 && (
                                <div style={styles.ratingOverview}>
                                    <span style={styles.ratingBig}>{avg.toFixed(1)}</span>
                                    <span style={styles.ratingOutOf}>/5</span>
                                    <StarRating score={Math.round(avg)} size={16} />
                                    <span style={styles.ratingCount}>{count} 人评分</span>
                                </div>
                            )}

                            {!ratingSubmitted ? (
                                <div style={styles.rateRow}>
                                    <div style={styles.rateStarsRow}>
                                        <StarRating score={myScore} onChange={setMyScore} />
                                        {myScore > 0 && <span style={styles.rateScoreLabel}>{myScore} 分</span>}
                                    </div>
                                    <input
                                        style={styles.rateComment}
                                        placeholder="评价（可选）"
                                        value={myComment}
                                        onChange={e => setMyComment(e.target.value)}
                                    />
                                    <button
                                        className="kb-rate-submit"
                                        style={{ ...styles.submitBtn, ...(myScore > 0 && !submitting ? styles.submitBtnActive : styles.submitBtnDisabled) }}
                                        disabled={myScore < 1 || submitting}
                                        onClick={submitRating}
                                    >
                                        {submitting ? '提交中...' : '提交评分'}
                                    </button>
                                </div>
                            ) : (
                                <div style={{ color: '#27ae60', fontSize: '12px', padding: '4px 0' }}>评分已提交</div>
                            )}

                            <div style={styles.ratingList}>
                                {allFeedback.filter(fb => fb.rating && fb.rating.score > 0).map((fb, i) => (
                                    <div key={i} style={styles.ratingItem}>
                                        <StarRating score={fb.rating!.score} size={12} />
                                        <span style={styles.ratingUser}>{fb.user}</span>
                                        {fb.rating!.comment && <span style={styles.ratingComment}>{fb.rating!.comment}</span>}
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Issues tab */}
                    {detailTab === 'issues' && (
                        <div style={styles.tabInner}>
                            <button className="kb-new-issue-btn" style={styles.newIssueBtnFull} onClick={() => setView('newIssue')}>
                                + 新建 Issue
                            </button>

                            {allIssues.length === 0 ? (
                                <div style={styles.emptyHint}>暂无 Issue</div>
                            ) : (
                                allIssues.map((issue, i) => (
                                    <div key={i} style={styles.issueCard}>
                                        <div style={styles.issueHeader}>
                                            <TypeBadge type={issue.type} />
                                            <PriorityBadge priority={issue.priority} />
                                            <span style={styles.issueTitle}>{issue.title}</span>
                                        </div>
                                        {issue.description && (
                                            <div style={styles.issueDesc}>{issue.description}</div>
                                        )}
                                        <div style={styles.issueFooter}>
                                            <span style={styles.issueReporter}>{issue.reporter}</span>
                                            <span style={styles.issueDate}>{formatDate(issue.timestamp)}</span>
                                            <StatusBadge status={issue.status} />
                                            {issue.status === 'open' && (
                                                <button
                                                    className="kb-issue-action"
                                                    style={styles.issueAction}
                                                    onClick={() => updateIssueStatus(issue.id, 'resolved')}
                                                >
                                                    标记解决
                                                </button>
                                            )}
                                        </div>
                                    </div>
                                ))
                            )}
                        </div>
                    )}
                </div>
            </div>
        );
    };

    // --- Render: New Issue Form ---
    const renderNewIssue = () => {
        return (
            <div style={styles.detailContainer}>
                <div style={styles.detailHeader}>
                    <button className="kb-back-btn" style={styles.backButton} onClick={() => setView('detail')}>
                        {TbArrowLeft({ size: 16 })}返回
                    </button>
                    <span style={styles.detailTitle}>新建 Issue</span>
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.formLabel}>类型</label>
                    <select style={styles.formSelect} value={issueType} onChange={e => setIssueType(e.target.value)}>
                        <option value="bug">错误 - 内容有误</option>
                        <option value="outdated">过时 - 信息已过时</option>
                        <option value="suggestion">建议 - 改进建议</option>
                    </select>
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.formLabel}>优先级</label>
                    <select style={styles.formSelect} value={issuePriority} onChange={e => setIssuePriority(e.target.value)}>
                        <option value="high">高</option>
                        <option value="medium">中</option>
                        <option value="low">低</option>
                    </select>
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.formLabel}>标题 *</label>
                    <input
                        style={styles.formInput}
                        value={issueTitle}
                        onChange={e => setIssueTitle(e.target.value)}
                        placeholder="简要描述问题"
                    />
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.formLabel}>描述</label>
                    <textarea
                        style={styles.formTextarea}
                        value={issueDesc}
                        onChange={e => setIssueDesc(e.target.value)}
                        placeholder="详细描述（可选）"
                        rows={4}
                    />
                </div>

                {error && <div style={styles.formError}>{error}</div>}

                <div style={styles.formActions}>
                    <button style={styles.cancelBtn} onClick={() => setView('detail')}>取消</button>
                    <button
                        style={{
                            ...styles.submitBtn,
                            opacity: issueTitle.trim() ? 1 : 0.4,
                        }}
                        disabled={!issueTitle.trim() || submitting}
                        onClick={submitIssue}
                    >
                        {submitting ? '提交中...' : '提交'}
                    </button>
                </div>
            </div>
        );
    };

    return (
        <div style={styles.container}>
            <style>{`
                .kb-scenario-card:hover {
                    border-color: #007acc !important;
                    background-color: #2d2d3a !important;
                }
                .kb-tree-node:hover {
                    background-color: #2a2a2a !important;
                }
                .kb-sub-tab:hover {
                    color: #bbb !important;
                }
                .kb-back-btn:hover {
                    background-color: #383838 !important;
                    border-color: #555 !important;
                    color: #fff !important;
                }
                .kb-issue-action:hover {
                    background-color: #27ae60 !important;
                    color: #fff !important;
                }
                .kb-new-issue-btn:hover {
                    background-color: rgba(0, 122, 204, 0.1) !important;
                }
                .kb-rate-submit:hover:not(:disabled) {
                    background-color: #0098ff !important;
                }
            `}</style>
            {error && view === 'tree' && (
                <div style={styles.errorBar}>{error}</div>
            )}
            {view === 'tree' && renderTree()}
            {view === 'detail' && renderDetail()}
            {view === 'newIssue' && renderNewIssue()}
        </div>
    );
};

// --- Styles ---

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        overflow: 'hidden',
    },
    emptyState: {
        padding: '20px',
        color: '#888',
        textAlign: 'center' as const,
        fontSize: '13px',
    },
    errorBar: {
        padding: '8px 12px',
        backgroundColor: '#3a1a1a',
        color: '#e74c3c',
        fontSize: '12px',
        borderRadius: '4px',
        margin: '8px',
    },

    // Tree
    treeContainer: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '0 0 8px 0',
    },

    // Search
    searchBox: {
        display: 'flex',
        alignItems: 'center',
        margin: '8px 10px 6px',
        backgroundColor: '#2a2a2a',
        border: '1px solid #3c3c3c',
        borderRadius: '6px',
        padding: '0 8px',
        gap: '6px',
    },
    searchIcon: {
        fontSize: '13px',
        opacity: 0.5,
    },
    searchInput: {
        flex: 1,
        backgroundColor: 'transparent',
        border: 'none',
        color: '#ccc',
        fontSize: '13px',
        padding: '7px 0',
        outline: 'none',
    },
    searchClear: {
        background: 'none',
        border: 'none',
        color: '#666',
        cursor: 'pointer',
        fontSize: '12px',
        padding: '2px',
    },
    statsRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '6px 16px 10px',
        fontSize: '11px',
        color: '#777',
        borderBottom: '1px solid #333',
    },
    statsDot: {
        color: '#555',
    },
    searchResults: {
        padding: '0 10px',
    },
    searchResultCount: {
        fontSize: '11px',
        color: '#888',
        padding: '4px 0',
    },
    scenarioServiceTag: {
        fontSize: '10px',
        backgroundColor: '#1a3a5a',
        color: '#4a9eff',
        padding: '1px 6px',
        borderRadius: '3px',
    },
    scenarioModTag: {
        fontSize: '10px',
        backgroundColor: '#2a2a3a',
        color: '#888',
        padding: '1px 6px',
        borderRadius: '3px',
    },
    scenarioMetaLine: {
        display: 'flex',
        gap: '4px',
        marginBottom: '3px',
    },
    treeNode: {
        display: 'flex',
        alignItems: 'center',
        padding: '6px 12px',
        cursor: 'pointer',
        gap: '6px',
        fontSize: '13px',
        color: '#ccc',
        userSelect: 'none' as const,
    },
    expandIcon: {
        fontSize: '10px',
        color: '#888',
        width: '12px',
        flexShrink: 0,
    },
    serviceName: {
        fontWeight: 600,
        flex: 1,
    },
    moduleName: {
        flex: 1,
        color: '#aaa',
    },
    countBadge: {
        fontSize: '10px',
        backgroundColor: '#333',
        color: '#888',
        padding: '1px 6px',
        borderRadius: '10px',
        minWidth: '18px',
        textAlign: 'center' as const,
    },
    scenarioCard: {
        margin: '2px 12px 2px 40px',
        padding: '8px 10px',
        backgroundColor: '#2a2a2a',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px solid #333',
        transition: 'border-color 0.2s',
    },
    scenarioHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        marginBottom: '4px',
    },
    scenarioTitle: {
        fontSize: '13px',
        color: '#ddd',
        fontWeight: 500,
    },
    scenarioPhenomena: {
        fontSize: '11px',
        color: '#999',
        marginBottom: '4px',
        lineHeight: 1.4,
    },
    keywordList: {
        display: 'flex',
        flexWrap: 'wrap' as const,
        gap: '3px',
    },
    keyword: {
        fontSize: '10px',
        backgroundColor: '#1a3a5a',
        color: '#4a9eff',
        padding: '1px 5px',
        borderRadius: '3px',
    },
    keywordMore: {
        fontSize: '10px',
        color: '#666',
        padding: '1px 3px',
    },

    // Detail
    detailContainer: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        overflow: 'hidden',
    },
    detailHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '10px 12px',
        borderBottom: '1px solid #333',
        backgroundColor: '#252526',
    },
    backButton: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        background: '#2d2d2d',
        border: '1px solid #444',
        color: '#ccc',
        cursor: 'pointer',
        fontSize: '12px',
        padding: '4px 10px',
        borderRadius: '4px',
        transition: 'background-color 0.15s, border-color 0.15s',
    },
    detailTitle: {
        fontSize: '14px',
        fontWeight: 600,
        color: '#ddd',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
        flex: 1,
    },
    metaRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        padding: '8px 12px 10px',
        fontSize: '12px',
        color: '#999',
        borderBottom: '1px solid #3c3c3c',
        flexWrap: 'wrap' as const,
    },
    metaItem: {},
    metaLabel: {
        color: '#777',
    },
    metaRating: {
        display: 'flex',
        alignItems: 'center',
        gap: '3px',
        fontSize: '12px',
        color: '#f1c40f',
    },
    metaIssueCount: {
        fontSize: '11px',
        backgroundColor: '#f39c12',
        color: '#000',
        padding: '1px 6px',
        borderRadius: '10px',
        fontWeight: 500,
    },

    // Sub tabs
    subTabBar: {
        display: 'flex',
        borderBottom: '1px solid #333',
        flexShrink: 0,
    },
    subTab: {
        flex: 1,
        padding: '8px 0',
        textAlign: 'center' as const,
        fontSize: '12px',
        color: '#888',
        background: 'none',
        border: 'none',
        borderBottom: '2px solid transparent',
        cursor: 'pointer',
        transition: 'color 0.2s, border-color 0.2s',
    },
    subTabActive: {
        color: '#ddd',
        borderBottomColor: '#007acc',
        fontWeight: 600,
    },
    tabContentArea: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '12px',
        fontSize: '13px',
        color: '#ccc',
        lineHeight: 1.6,
    },
    tabInner: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px',
    },

    // Rating overview in tab
    ratingOverview: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '8px 0',
        borderBottom: '1px solid #2a2a2a',
        marginBottom: '4px',
    },
    ratingBig: {
        fontSize: '24px',
        fontWeight: 700,
        color: '#f1c40f',
    },
    ratingOutOf: {
        fontSize: '14px',
        color: '#888',
    },
    ratingCount: {
        fontSize: '12px',
        color: '#888',
        marginLeft: '4px',
    },
    ratingList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '2px',
    },
    ratingSummary: {
        fontSize: '12px',
        color: '#999',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
    },
    newIssueBtnFull: {
        background: 'none',
        border: '1px solid #007acc',
        color: '#007acc',
        borderRadius: '4px',
        padding: '6px 12px',
        fontSize: '12px',
        cursor: 'pointer',
        width: '100%',
        textAlign: 'center' as const,
    },

    // Rating
    rateStarsRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
    },
    rateScoreLabel: {
        color: '#007acc',
        fontSize: '13px',
        fontWeight: 500,
    },
    rateRow: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        marginBottom: '6px',
    },
    rateComment: {
        backgroundColor: '#2a2a2a',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        padding: '6px 8px',
        color: '#ccc',
        fontSize: '12px',
        outline: 'none',
        minHeight: '28px',
    },
    submitBtn: {
        backgroundColor: '#007acc',
        color: '#fff',
        border: 'none',
        borderRadius: '4px',
        padding: '6px 12px',
        fontSize: '12px',
        cursor: 'pointer',
    },
    submitBtnDisabled: {
        backgroundColor: '#444',
        cursor: 'not-allowed',
    },
    submitBtnActive: {
        backgroundColor: '#007acc',
        cursor: 'pointer',
    },
    ratingItem: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '3px 0',
        fontSize: '12px',
    },
    ratingUser: {
        color: '#007acc',
        fontWeight: 500,
    },
    ratingComment: {
        color: '#999',
        flex: 1,
    },

    // Issues
    newIssueBtn: {
        background: 'none',
        border: '1px solid #007acc',
        color: '#007acc',
        borderRadius: '4px',
        padding: '2px 10px',
        fontSize: '12px',
        cursor: 'pointer',
    },
    emptyHint: {
        color: '#666',
        fontSize: '12px',
        textAlign: 'center' as const,
        padding: '8px',
    },
    issueCard: {
        backgroundColor: '#2a2a2a',
        borderRadius: '4px',
        padding: '8px 10px',
        marginBottom: '6px',
        border: '1px solid #333',
    },
    issueHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        marginBottom: '4px',
    },
    issueTitle: {
        fontSize: '13px',
        color: '#ddd',
        fontWeight: 500,
        flex: 1,
    },
    issueDesc: {
        fontSize: '12px',
        color: '#999',
        marginBottom: '6px',
        lineHeight: 1.4,
    },
    issueFooter: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: '11px',
    },
    issueReporter: {
        color: '#007acc',
    },
    issueDate: {
        color: '#666',
    },
    issueAction: {
        background: 'none',
        border: '1px solid #27ae60',
        color: '#27ae60',
        borderRadius: '3px',
        padding: '1px 8px',
        fontSize: '11px',
        cursor: 'pointer',
        marginLeft: 'auto',
    },

    // Badge
    badge: {
        fontSize: '11px',
        color: '#fff',
        padding: '1px 6px',
        borderRadius: '3px',
        fontWeight: 500,
        whiteSpace: 'nowrap' as const,
    },

    // Form
    formGroup: {
        padding: '0 12px',
        marginBottom: '10px',
    },
    formLabel: {
        display: 'block',
        fontSize: '12px',
        color: '#999',
        marginBottom: '4px',
    },
    formSelect: {
        width: '100%',
        backgroundColor: '#2a2a2a',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        padding: '6px 8px',
        color: '#ccc',
        fontSize: '13px',
        outline: 'none',
    },
    formInput: {
        width: '100%',
        backgroundColor: '#2a2a2a',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        padding: '6px 8px',
        color: '#ccc',
        fontSize: '13px',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    formTextarea: {
        width: '100%',
        backgroundColor: '#2a2a2a',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        padding: '6px 8px',
        color: '#ccc',
        fontSize: '13px',
        outline: 'none',
        resize: 'vertical' as const,
        boxSizing: 'border-box' as const,
    },
    formError: {
        color: '#e74c3c',
        fontSize: '12px',
        padding: '4px 12px',
    },
    formActions: {
        display: 'flex',
        justifyContent: 'flex-end' as const,
        gap: '8px',
        padding: '10px 12px',
    },
    cancelBtn: {
        background: 'none',
        border: '1px solid #555',
        color: '#999',
        borderRadius: '4px',
        padding: '6px 16px',
        fontSize: '13px',
        cursor: 'pointer',
    },
};

export default KnowledgeBrowser;
