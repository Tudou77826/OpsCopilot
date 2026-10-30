import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
        primary: vi.fn(async (id, path) => {const p = state.projects.find(p => p.id === id)!; p.directories = [path, ...p.directories.filter(v => v !== path)]; return copy();}),
        status: vi.fn(async () => ({state: 'pending'})),
    };
    return {host};
}
async function open() {fireEvent.click(screen.getByRole('button', {name: '本地项目'})); await screen.findByRole('dialog');}
async function create(name: string) {
    fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: name}});
    fireEvent.click(screen.getByRole('button', {name: '保存项目'}));
    return screen.findByRole('region', {name: `项目 ${name}`});
}
async function add(project: string, path: string) {
    const tree = screen.getByRole('region', {name: `项目 ${project}`});
    fireEvent.click(within(tree).getByRole('button', {name: '输入路径'}));
    fireEvent.change(within(tree).getByLabelText('添加工作目录'), {target: {value: path}});
    fireEvent.click(within(tree).getByRole('button', {name: '添加本地目录'}));
    await within(tree).findByText(path);
}
function projectMenu(project: string) {fireEvent.click(screen.getByRole('button', {name: `管理项目 ${project}`}));}

describe('本地项目与目录', () => {
    it('展开其他项目只是查看；选择图标才切换项目', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); await host.add(a.activeId, ['D:\\alpha']);
        const b = await host.create('B项目'); await host.add(b.activeId, ['D:\\beta']); await host.activate(a.activeId);
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open();
        const aTree = await screen.findByRole('region', {name: '项目 A项目'});
        expect(within(aTree).getByText('D:\\alpha')).toBeVisible();
        const bTree = screen.getByRole('region', {name: '项目 B项目'});
        fireEvent.click(within(bTree).getByRole('button', {name: 'B项目'}));
        expect(within(bTree).getByText('D:\\beta')).toBeVisible();
        expect(within(aTree).getByText('D:\\alpha')).toBeVisible();
        expect(screen.getByRole('button', {name: '选择项目 A项目'})).toHaveAttribute('aria-pressed', 'true');
        expect((await host.list()).activeId).toBe(a.activeId);
        fireEvent.click(screen.getByRole('button', {name: '选择项目 B项目'}));
        await waitFor(() => expect(screen.getByRole('button', {name: '选择项目 B项目'})).toHaveAttribute('aria-pressed', 'true'));
        expect((await host.list()).activeId).toBe(b.activeId);
    });
    it('添加目录优先打开选择器，也支持输入路径', async () => {
        const {host} = fixture(); await host.create('项目');
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open();
        const tree = await screen.findByRole('region', {name: '项目 项目'});
        fireEvent.click(within(tree).getByRole('button', {name: '添加目录'}));
        expect(await within(tree).findByText('D:\\downloads')).toBeVisible();
        await add('项目', 'D:\\code');
        expect(host.add).toHaveBeenCalledWith('id-0', ['D:\\code']);
    });
    it('切换主目录并显示实际应用状态', async () => {
        const {host} = fixture(); const project = await host.create('支付服务'); await host.add(project.activeId, ['D:\\api', 'D:\\web']);
        host.status = vi.fn(async () => ({state: 'applied'}));
        render(<AssistantLocalDirectories compact busy={false} revision={0} host={host}/>); await open(); await screen.findByText('D:\\web');
        expect(screen.getByText(/已应用到助手/)).toBeVisible();
        fireEvent.click(screen.getByRole('button', {name: '设为主目录'}));
        await waitFor(() => expect(host.primary).toHaveBeenCalledWith(project.activeId, 'D:\\web'));
        await waitFor(() => expect(screen.getByText('主目录').parentElement).toHaveTextContent('web'));
    });
    it('未选择项目和读取失败时不显示已选择状态', async () => {
        const {host} = fixture(); const view = render(<AssistantLocalDirectories compact busy={false} revision={0} host={host}/>);
        await waitFor(() => expect(screen.getByRole('button', {name: '本地项目'})).toHaveAttribute('title', '未选择本地项目'));
        host.list = vi.fn(async () => {throw Error('读取失败');}); view.rerender(<AssistantLocalDirectories compact busy={false} revision={1} host={host}/>);
        await waitFor(() => expect(screen.getByRole('button', {name: '本地项目'})).toHaveAttribute('title', '项目配置读取失败'));
    });
    it('编辑期间其他窗口切换项目，不把重命名改投到新项目', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); const b = await host.create('B项目'); await host.activate(a.activeId);
        const view = render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByRole('region', {name: '项目 A项目'});
        projectMenu('A项目'); fireEvent.click(screen.getByRole('button', {name: '重命名'})); fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: 'A已改名'}});
        await host.activate(b.activeId); view.rerender(<AssistantLocalDirectories busy={false} revision={1} host={host}/>);
        await waitFor(() => expect(screen.getByRole('button', {name: '选择项目 B项目'})).toHaveAttribute('aria-pressed', 'true'));
        fireEvent.click(screen.getByRole('button', {name: '保存项目'})); await waitFor(() => expect(host.rename).toHaveBeenCalledWith(a.activeId, 'A已改名'));
    });
    it('运行中允许展开查看，不能切换或修改', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); await host.add(a.activeId, ['D:\\project']); const b = await host.create('B项目'); await host.add(b.activeId, ['D:\\other']); await host.activate(a.activeId);
        const view = render(<AssistantLocalDirectories busy={true} revision={0} host={host}/>); await open(); await screen.findByText('D:\\project');
        expect(screen.getByRole('button', {name: '选择项目 A项目'})).toBeDisabled();
        fireEvent.click(screen.getByRole('button', {name: 'B项目'})); expect(screen.getByText('D:\\other')).toBeVisible();
        expect(within(screen.getByRole('region', {name: '项目 B项目'})).getByRole('button', {name: '添加目录'})).toBeDisabled();
        view.rerender(<AssistantLocalDirectories busy={false} revision={1} host={host}/>); await waitFor(() => expect(screen.getByRole('button', {name: '选择项目 A项目'})).toBeEnabled());
    });
    it('重命名及移除项目通过项目菜单，删除前说明不会删除文件', async () => {
        const {host} = fixture(); const a = await host.create('A项目'); await host.add(a.activeId, ['D:\\project']);
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByText('D:\\project');
        projectMenu('A项目'); fireEvent.click(screen.getByRole('button', {name: '重命名'})); fireEvent.change(screen.getByLabelText('项目名称'), {target: {value: '新名称'}}); fireEvent.click(screen.getByRole('button', {name: '保存项目'}));
        await screen.findByRole('button', {name: '管理项目 新名称'}); fireEvent.click(screen.getByRole('button', {name: '移除目录 D:\\project'})); await waitFor(() => expect(screen.queryByText('D:\\project')).not.toBeInTheDocument());
        projectMenu('新名称'); fireEvent.click(screen.getByRole('button', {name: '移除项目'})); expect(host.delete).not.toHaveBeenCalled(); expect(screen.getByText('磁盘上的目录和文件不会删除。')).toBeVisible();
        fireEvent.click(screen.getByRole('button', {name: '移除项目配置'})); await screen.findByLabelText('项目名称'); expect(host.delete).toHaveBeenCalledWith(a.activeId);
    });
    it('无效路径保留输入及错误提示', async () => {
        const {host} = fixture(); await host.create('A项目'); host.add = vi.fn(async () => {throw Error('本地目录不存在');});
        render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await screen.findByRole('region', {name: '项目 A项目'});
        fireEvent.click(screen.getByRole('button', {name: '输入路径'})); fireEvent.change(screen.getByLabelText('添加工作目录'), {target: {value: 'D:\\missing'}}); fireEvent.click(screen.getByRole('button', {name: '添加本地目录'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('本地目录不存在'); expect(screen.getByLabelText('添加工作目录')).toHaveValue('D:\\missing');
    });
    it('新建项目后自动展开，切换时不丢失各项目目录', async () => {
        const {host} = fixture(); render(<AssistantLocalDirectories busy={false} revision={0} host={host}/>); await open(); await create('A项目'); await add('A项目', 'D:\\project');
        fireEvent.click(screen.getByRole('button', {name: '新建项目'})); await create('B项目'); await add('B项目', 'D:\\business');
        expect(screen.getByText('D:\\project')).toBeVisible(); expect(screen.getByText('D:\\business')).toBeVisible();
        fireEvent.click(screen.getByRole('button', {name: '选择项目 A项目'})); await waitFor(() => expect(screen.getByRole('button', {name: '选择项目 A项目'})).toHaveAttribute('aria-pressed', 'true'));
        expect(host.add).toHaveBeenCalledWith('id-1', ['D:\\business']);
    });
});
