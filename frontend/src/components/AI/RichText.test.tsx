import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import RichText from './RichText';

describe('Markdown 命令阅读与复制', () => {
    afterEach(() => vi.unstubAllGlobals());
    it('行内命令保持在段落内，不生成代码框和复制按钮', () => {
        const {container}=render(<RichText content={'先用 `pwd` 确认目录。'}/>);
        expect(container.querySelector('p > code')).toHaveTextContent('pwd');
        expect(container.querySelector('pre')).toBeNull();
        expect(screen.queryByRole('button',{name:'复制'})).not.toBeInTheDocument();
    });
    it('高亮代码保持合法结构，复制得到完整原文', async () => {
        const copy=vi.fn(async()=>undefined);
        vi.stubGlobal('navigator',{clipboard:{writeText:copy}});
        const code='echo "hello"\npwd';
        const {container}=render(<RichText content={'```bash\n'+code+'\n```'}/>);
        expect(container.querySelectorAll('pre')).toHaveLength(1);
        expect(container.querySelector('pre > code')).toHaveTextContent('echo "hello"');
        fireEvent.click(screen.getByRole('button',{name:'复制'}));
        await waitFor(()=>expect(copy).toHaveBeenCalledWith(code));
        expect(screen.getByRole('button',{name:'已复制'})).toBeVisible();
    });
    it('剪贴板不可用时明确反馈', async () => {
        vi.stubGlobal('navigator',{clipboard:{writeText:vi.fn(async()=>{throw Error('unavailable');})}});
        render(<RichText content={'```\npwd\n```'}/>);
        fireEvent.click(screen.getByRole('button',{name:'复制'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('复制失败');
    });
});
