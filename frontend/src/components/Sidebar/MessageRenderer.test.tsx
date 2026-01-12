import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import MessageRenderer from './MessageRenderer';

describe('MessageRenderer', () => {
    it('应该渲染纯文本消息', () => {
        const { container } = render(
            <MessageRenderer content="这是一条纯文本消息" role="user" />
        );
        expect(container.textContent).toContain('这是一条纯文本消息');
    });

    it('应该渲染粗体文本', () => {
        const content = '这是**粗体**文本';
        const { container } = render(
            <MessageRenderer content={content} role="ai" />
        );
        const strong = container.querySelector('strong');
        expect(strong).toBeTruthy();
        expect(strong?.textContent).toBe('粗体');
    });

    it('应该渲染代码块', () => {
        const content = '```javascript\nconsole.log("hello");\n```';
        const { container } = render(
            <MessageRenderer content={content} role="ai" />
        );
        expect(container.querySelector('pre')).toBeTruthy();
        expect(container.querySelector('code')).toBeTruthy();
    });

    it('应该渲染链接', () => {
        const content = '[点击这里](https://example.com)';
        const { container } = render(
            <MessageRenderer content={content} role="ai" />
        );
        const link = container.querySelector('a');
        expect(link).toBeTruthy();
        expect(link?.getAttribute('href')).toBe('https://example.com');
        expect(link?.getAttribute('target')).toBe('_blank');
    });

    it('应该渲染列表', () => {
        const content = '- 项目1\n- 项目2\n- 项目3';
        const { container } = render(
            <MessageRenderer content={content} role="ai" />
        );
        expect(container.querySelector('ul')).toBeTruthy();
        expect(container.querySelectorAll('li').length).toBe(3);
    });
});
