import React from 'react';
import FileContextMenu, { ContextMenuItem } from '../filetransfer/FileContextMenu';
import type { SessionNode } from '../ports';
import { collectConnections, findLocation, isFolder } from './treeModel';

/** 会话树右键菜单的动作。集中成一个联合类型，便于容器统一处理。 */
export type ContextMenuAction =
    | { kind: 'newFolder'; parentId: string }
    | { kind: 'newConnection'; parentId: string }
    | { kind: 'connect'; node: SessionNode }
    | { kind: 'connectAll'; node: SessionNode }
    | { kind: 'properties'; node: SessionNode }
    | { kind: 'duplicate'; node: SessionNode }
    | { kind: 'sortByName'; parentId: string }
    | { kind: 'moveToRoot'; node: SessionNode }
    | { kind: 'rename'; node: SessionNode }
    | { kind: 'delete'; node: SessionNode }
    | { kind: 'import' };

type Props = {
    x: number;
    y: number;
    /** 右键点中的节点；点在空白处为 null。 */
    node: SessionNode | null;
    nodes: SessionNode[];
    canDuplicate: boolean;
    canImport: boolean;
    onAction: (action: ContextMenuAction) => void;
    onClose: () => void;
};

/**
 * 会话树右键菜单。菜单项按点击对象动态生成：文件夹、连接、空白处各有一套。
 *
 * 复用 FileContextMenu（fixed 定位、边缘翻转、ESC/外部关闭），不再自造一份内联菜单。
 */
const SessionContextMenu: React.FC<Props> = ({
    x,
    y,
    node,
    nodes,
    canDuplicate,
    canImport,
    onAction,
    onClose,
}) => {
    const items: ContextMenuItem[] = [];
    const run = (action: ContextMenuAction) => () => onAction(action);

    if (!node) {
        // 空白处：新建与整体操作用于根层级。
        items.push({ label: '新建文件夹', onClick: run({ kind: 'newFolder', parentId: '' }) });
        items.push({ label: '新建连接', onClick: run({ kind: 'newConnection', parentId: '' }) });
        items.push({ label: '按名称排序', onClick: run({ kind: 'sortByName', parentId: '' }) });
        if (canImport) {
            items.push({ label: '导入 Xshell 会话…', onClick: run({ kind: 'import' }) });
        }
        return <FileContextMenu x={x} y={y} items={items} onClose={onClose} />;
    }

    const location = findLocation(nodes, node.id);
    const atRoot = location?.parent == null;

    if (isFolder(node)) {
        items.push({ label: '新建子文件夹', onClick: run({ kind: 'newFolder', parentId: node.id }) });
        items.push({ label: '新建连接', onClick: run({ kind: 'newConnection', parentId: node.id }) });

        const connections = collectConnections(node);
        if (connections.length > 0) {
            items.push({ label: `全部连接（${connections.length}）`, onClick: run({ kind: 'connectAll', node }) });
        }
        items.push({ label: '按名称排序', onClick: run({ kind: 'sortByName', parentId: node.id }) });
        items.push({ label: '重命名', onClick: run({ kind: 'rename', node }) });
        if (!atRoot) {
            items.push({ label: '移到根目录', onClick: run({ kind: 'moveToRoot', node }) });
        }
        const count = connections.length;
        items.push({
            label: count > 0 ? `删除（含 ${count} 个连接）` : '删除',
            danger: true,
            onClick: run({ kind: 'delete', node }),
        });
    } else {
        items.push({ label: '打开连接', onClick: run({ kind: 'connect', node }) });
        items.push({ label: '编辑连接', onClick: run({ kind: 'properties', node }) });
        if (canDuplicate) {
            items.push({ label: '复制连接', onClick: run({ kind: 'duplicate', node }) });
        }
        items.push({ label: '重命名', onClick: run({ kind: 'rename', node }) });
        if (!atRoot) {
            items.push({ label: '移到根目录', onClick: run({ kind: 'moveToRoot', node }) });
        }
        items.push({ label: '删除', danger: true, onClick: run({ kind: 'delete', node }) });
    }

    return <FileContextMenu x={x} y={y} items={items} onClose={onClose} />;
};

export default SessionContextMenu;
