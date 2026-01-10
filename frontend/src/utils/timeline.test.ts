import { describe, test, expect } from 'vitest';
import { filterTimelineEvents, generateMarkdown, TimelineEvent } from './timeline';

describe('timeline utils', () => {
    const mockEvents: TimelineEvent[] = [
        { timestamp: '1', type: 'user_query', content: 'CPU High' },
        { timestamp: '2', type: 'ai_suggestion', content: 'Check top' },
        { timestamp: '3', type: 'terminal_input', content: 'top -b -n 1' },
        { timestamp: '4', type: 'terminal_output', content: '...output...' },
        { timestamp: '5', type: 'terminal_input', content: '   ' }, // Empty
        { timestamp: '6', type: 'terminal_input', content: '^V' }, // Garbage
        { timestamp: '7', type: 'terminal_input', content: '' }, // Garbage char
    ];

    test('filterTimelineEvents removes unwanted events', () => {
        const filtered = filterTimelineEvents(mockEvents);
        // user_query, ai_suggestion, terminal_input(top) -> 3 events
        expect(filtered.length).toBe(3);
        
        expect(filtered.find(e => e.type === 'terminal_output')).toBeUndefined();
        expect(filtered.find(e => e.content === '   ')).toBeUndefined();
        expect(filtered.find(e => e.content === '^V')).toBeUndefined();
        expect(filtered.find(e => e.content === '')).toBeUndefined();
        
        expect(filtered[0].content).toBe('CPU High');
        expect(filtered[2].content).toBe('top -b -n 1');
    });

    test('generateMarkdown formats correctly', () => {
        const filtered = filterTimelineEvents(mockEvents);
        const md = generateMarkdown(filtered, 'Problem', 'Cause');
        
        expect(md).toContain('# 排查会话记录');
        expect(md).toContain('**排查目标:** Problem');
        expect(md).toContain('**根本原因:** Cause');
        
        expect(md).toContain('### 用户提问');
        expect(md).toContain('CPU High');
        
        expect(md).toContain('### 终端执行');
        expect(md).toContain('top -b -n 1');
    });
});
