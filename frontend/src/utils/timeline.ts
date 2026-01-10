export interface TimelineEvent {
    timestamp: string;
    type: string;
    content: string;
    metadata?: any;
}

export const filterTimelineEvents = (events: TimelineEvent[]): TimelineEvent[] => {
    return events.filter(e => {
        // 1. Filter out terminal_output
        if (e.type === 'terminal_output') return false;

        // 2. Filter out empty content
        if (!e.content || !e.content.trim()) return false;

        // 3. Filter out control characters/garbage
        const trimmed = e.content.trim();
        // Filter common control char artifacts that might have slipped through
        if (trimmed === '' || trimmed === '^V') return false; 
        
        return true;
    });
};

export const translateType = (type: string) => {
    switch (type) {
        case 'user_query': return '用户提问';
        case 'ai_suggestion': return 'AI 建议';
        case 'terminal_input': return '终端执行';
        default: return type;
    }
};

export const generateMarkdown = (events: TimelineEvent[], problem: string, rootCause: string): string => {
    let md = `# 排查会话记录\n\n`;
    md += `**排查目标:** ${problem}\n\n`;
    md += `**根本原因:** ${rootCause}\n\n`;
    md += `## 详细过程\n\n`;

    events.forEach((e) => {
        md += `### ${translateType(e.type)}\n`;
        md += `${e.content.trim()}\n\n`;
    });
    
    return md;
};
