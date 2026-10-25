import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { LocalProjectState } from '../../../../frontend-shell/src/assistant/localProjects';
import { AssistantLocalDirectories, type LocalDirectoryHost } from './AssistantLocalDirectories';

beforeEach(() => {HTMLDialogElement.prototype.showModal = function () {this.setAttribute('open', '');};});
function fixture() {
    let state: LocalProjectState = {version: 1, activeId: '', projects: []};
    const copy = () => structuredClone(state);
    const host: LocalDirectoryHost = {
        list: vi.fn(async () => copy()),
        create: vi.fn(async name => {const id = `id-${state.projects.length}`; state.projects.push({id, name, directories: []}); state.activeId = id; return copy();}),
        activate: vi.fn(async id => {state.activeId = id; return copy();}),
        rename: vi.fn(async (id, name) => {state.projects.find(p => p.id === id)!.name = name; return copy();}),
        delete: vi.fn(async id => {state.projects = state.projects.filter(p => p.id !== id); if (state.activeId === id) state.activeId = ''; return copy();}),
        add: vi.fn(async (id, next) => {const p = state.projects.find(p => p.id === id)!; p.directories = [...new Set([...p.directories, ...next])]; return copy();}),
        remove: vi.fn(async (id, path) => {const p = state.projects.find(p => p.id === id)!; p.directories = p.directories.filter(v => v !== path); return copy();}),
        select: vi.fn(async () => 'D:\\downloads'),
    };
    return {host};
}
async function open() {fireEvent.click(screen.getByRole('button', {name: '本地项目'})); await screen.findByRole('dialog');}
async function create(name: string) {fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: name}}); fireEvent.click(screen.getByRole('button', {name: '保存项目'})); await screen.findByLabelText('添加目录');}
async function add(path: string) {fireEvent.change(screen.getByLabelText('添加目录'), {target: {value: path}}); fireEvent.click(screen.getByRole('button', {name: '添加本地目录'})); await waitFor(() => expect(screen.getByLabelText('添加目录')).toHaveValue(''));}
describe('本地项目与目录', () => {
    it('编辑期间其他窗口切换当前项目，不把重命名改投到新项目', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); const b = await host.create('B项目'); await host.activate(a.activeId);
        const view = render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByText('A项目的目录');
        fireEvent.click(screen.getByRole('button', {name: '重命名项目'})); fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: 'A已改名'}});
        await host.activate(b.activeId); view.rerender(<AssistantLocalDirectories busy={false} revision={1} host={host}/>);
        await waitFor(() => expect(screen.getByLabelText('当前项目')).toHaveValue(b.activeId));
        fireEvent.click(screen.getByRole('button', {name: '保存项目'}));
        await waitFor(() => expect(host.rename).toHaveBeenCalledWith(a.activeId, 'A已改名'));
    });
    it('A项目管理多个目录，切到B项目后只显示B的目录，切回不丢失A', async () => {
        const {host} = fixture();
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>);
        await open(); await create('A项目'); await add('D:\\project'); await add('D:\\logs');
        fireEvent.click(screen.getByRole('button', {name: '选择本地文件夹'})); await screen.findByText('D:\\downloads');
        fireEvent.click(screen.getByRole('button', {name: '新建项目'})); await create('B项目');
        await add('D:\\business'); expect(screen.queryByText('D:\\project')).not.toBeInTheDocument();
        fireEvent.change(screen.getByLabelText('当前项目'), {target: {value: 'id-0'}});
        await screen.findByText('D:\\project'); expect(screen.getByText('D:\\logs')).toBeVisible(); expect(screen.queryByText('D:\\business')).not.toBeInTheDocument();
        expect(host.add).toHaveBeenCalledWith('id-1', ['D:\\business']);
    });
    it('运行时可查看项目但不能切换或修改，新建对话的刷新标记不清空配置', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); await host.add(a.activeId, ['D:\\project']);
        const view = render(<AssistantLocalDirectories busy={true} revision={0} host={host}/>);
        await open(); await screen.findByText('D:\\project');
        expect(screen.getByRole('button', {name: '移除目录 D:\\project'})).toBeDisabled(); expect(screen.getByLabelText('当前项目')).toBeDisabled();
        view.rerender(<AssistantLocalDirectories busy={false} revision={1} host={host}/>);
        await waitFor(() => expect(screen.getByLabelText('当前项目')).toBeEnabled()); expect(screen.getByText('D:\\project')).toBeVisible();
    });
    it('重命名、单项移除及删除项目需明确确认，界面说明不会删除文件', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); await host.add(a.activeId, ['D:\\project']);
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByText('D:\\project');
        fireEvent.click(screen.getByRole('button', {name: '重命名项目'})); fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: '新名称'}}); fireEvent.click(screen.getByRole('button', {name: '保存项目'}));
        await screen.findByText('新名称的目录');
        fireEvent.click(screen.getByRole('button', {name: '移除目录 D:\\project'})); await waitFor(() => expect(screen.queryByText('D:\\project')).not.toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', {name: '移除项目'}));
        expect(host.delete).not.toHaveBeenCalled(); expect(screen.getByText('磁盘上的目录和文件不会删除。')).toBeVisible();
        fireEvent.click(screen.getByRole('button', {name: '移除项目配置'})); await screen.findByLabelText('项目名称'); expect(host.delete).toHaveBeenCalledWith(a.activeId);
    });
    it('无效路径保留输入及错误提示', async () => {
        const {host} = fixture(); await host.create('A项目'); host.add = vi.fn(async () => {throw Error('本地目录不存在');});
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByLabelText('添加目录');
        fireEvent.change(screen.getByLabelText('添加目录'), {target: {value: 'D:\\missing'}}); fireEvent.click(screen.getByRole('button', {name: '添加本地目录'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('本地目录不存在'); expect(screen.getByLabelText('添加目录')).toHaveValue('D:\\missing');
    });
});
