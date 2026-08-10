import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
    TbBook,
    TbBrain,
    TbCheck,
    TbChevronDown,
    TbRefresh,
    TbSearch,
    TbTarget,
    TbWriting,
    TbAlertTriangle,
} from 'react-icons/tb';
import { AgentTraceEvent } from './types';
import './ai-components.css';

interface RetrievalTraceProps {
    events: AgentTraceEvent[];
    active: boolean;
}

const STAGES: Record<string, { label: string; icon: React.ReactNode }> = {
    thinking: { label: '分析问题', icon: TbBrain({ size: 13 }) },
    catalog_match: { label: '匹配目录', icon: TbTarget({ size: 13 }) },
    searching: { label: '检索文档', icon: TbSearch({ size: 13 }) },
    grepping: { label: '搜索关键词', icon: TbSearch({ size: 13 }) },
    reading: { label: '查阅文档', icon: TbBook({ size: 13 }) },
    answering: { label: '组织回答', icon: TbWriting({ size: 13 }) },
    retrying: { label: '重试请求', icon: TbRefresh({ size: 13 }) },
    error: { label: '检索失败', icon: TbAlertTriangle({ size: 13 }) },
};

function cleanDetail(stage: string, message: string): string {
    return message
        .replace(/^正在(?:分析问题，)?/, '')
        .replace(/^正在(?:搜索关键词|阅读文档|检索文档列表):?\s*/, '')
        .replace(/^模型未调用工具，/, '')
        .replace(/\.\.\.$/, '')
        .trim() || STAGES[stage]?.label || message;
}

const RetrievalTrace: React.FC<RetrievalTraceProps> = ({ events, active }) => {
    const [open, setOpen] = useState(active);
    const wasActive = useRef(active);

    useEffect(() => {
        if (active) setOpen(true);
        if (wasActive.current && !active) setOpen(false);
        wasActive.current = active;
    }, [active]);

    const duration = useMemo(() => {
        if (events.length < 2) return null;
        const delta = Math.max(0, events[events.length - 1].ts - events[0].ts);
        return `${(delta / 1000).toFixed(1)}s`;
    }, [events]);

    if (events.length === 0) return null;

    return (
        <section className={`ai-trace ${active ? 'is-active' : ''} ${open ? 'is-open' : ''}`}>
            <button className="ai-trace-header" type="button" onClick={() => setOpen(value => !value)} aria-expanded={open}>
                <span className="ai-trace-orb">{!active && TbCheck({ size: 11 })}</span>
                <span className="ai-trace-title">{active ? '正在检索知识' : '已完成知识检索'}</span>
                <span className="ai-trace-meta">{events.length} 项{duration ? ` · ${duration}` : ''}</span>
                <span className="ai-trace-chevron">{TbChevronDown({ size: 14 })}</span>
            </button>
            <div className="ai-trace-collapsible">
                <div className="ai-trace-collapsible-inner">
                    <ol className="ai-trace-list">
                        {events.map((event, index) => {
                            const config = STAGES[event.stage] || { label: event.stage, icon: TbSearch({ size: 13 }) };
                            const running = active && index === events.length - 1;
                            return (
                                <li className={`ai-trace-item ${running ? 'is-running' : ''} ${event.stage === 'error' ? 'is-error' : ''}`} key={`${event.ts}-${event.stage}-${index}`}>
                                    <span className="ai-trace-item-icon">{config.icon}</span>
                                    <span className="ai-trace-kind">{config.label}</span>
                                    <span className="ai-trace-detail">{cleanDetail(event.stage, event.message)}</span>
                                </li>
                            );
                        })}
                    </ol>
                </div>
            </div>
        </section>
    );
};

export default RetrievalTrace;
