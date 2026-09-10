import type { SessionNode } from '../ports';

/**
 * 会话树的纯函数模型。
 *
 * 拖拽排序、环检测、按名称排序这类逻辑全部放在这里，与 React 渲染解耦：
 * 它们是"正确性"所在，必须能被单测直接覆盖，而不是藏在事件处理器里。
 */

/** 拖拽落点语义：同级插入到目标之前 / 之后，或移入目标文件夹内部。 */
export type DropPosition = 'before' | 'after' | 'inside';

/** 行高三分区的边缘比例：上下各 25% 用于同级排序，中间 50% 用于移入文件夹。 */
export const EDGE_RATIO = 0.25;

/** 缩进最大层级，超过后不再继续缩进，改用 tooltip 展示完整路径。 */
export const MAX_INDENT_LEVEL = 6;

export function indentForLevel(level: number): number {
  return Math.min(level, MAX_INDENT_LEVEL) * 18 + 10;
}

export function isFolder(node: SessionNode): boolean {
  return node.type === 'folder';
}

export function findNode(nodes: SessionNode[], id: string): SessionNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    if (node.children?.length) {
      const found = findNode(node.children, id);
      if (found) return found;
    }
  }
  return undefined;
}

/** 节点在树中的位置：父节点（根为 null）、所在兄弟数组、下标。 */
export interface NodeLocation {
  parent: SessionNode | null;
  siblings: SessionNode[];
  index: number;
}

export function findLocation(nodes: SessionNode[], id: string): NodeLocation | null {
  const walk = (list: SessionNode[], parent: SessionNode | null): NodeLocation | null => {
    for (let i = 0; i < list.length; i++) {
      const node = list[i];
      if (node.id === id) return { parent, siblings: list, index: i };
      if (node.children?.length) {
        const found = walk(node.children, node);
        if (found) return found;
      }
    }
    return null;
  };
  return walk(nodes, null);
}

/** 取某一层子节点；parentId 为空表示根。 */
export function childrenOf(nodes: SessionNode[], parentId: string): SessionNode[] {
  if (!parentId) return nodes;
  return findNode(nodes, parentId)?.children ?? [];
}

/** 从根到该节点的路径，用于 tooltip 与面包屑。 */
export function pathOf(nodes: SessionNode[], id: string): SessionNode[] {
  const walk = (list: SessionNode[], trail: SessionNode[]): SessionNode[] | null => {
    for (const node of list) {
      const next = [...trail, node];
      if (node.id === id) return next;
      if (node.children?.length) {
        const found = walk(node.children, next);
        if (found) return found;
      }
    }
    return null;
  };
  return walk(nodes, []) ?? [];
}

export function breadcrumb(nodes: SessionNode[], id: string): string {
  return pathOf(nodes, id)
    .map((node) => node.name)
    .join(' / ');
}

/** 递归收集节点下的全部连接（文件夹自身不算）。 */
export function collectConnections(node: SessionNode): SessionNode[] {
  if (node.type === 'session') return [node];
  return (node.children ?? []).flatMap(collectConnections);
}

/** 统计节点下的连接数量，用于删除文件夹时的确认提示。 */
export function countConnections(node: SessionNode): number {
  return collectConnections(node).length;
}

export function isDescendant(ancestor: SessionNode, id: string): boolean {
  if (!id) return false;
  for (const child of ancestor.children ?? []) {
    if (child.id === id || isDescendant(child, id)) return true;
  }
  return false;
}

/**
 * 判断能否把 dragId 移到 targetParentId 下。
 *
 * 文件夹不能移入自身或其后代，否则会形成脱离根节点的环。前端用它在拖拽时
 * 显示禁止光标；后端 MoveNode 也会独立校验，两边都不能省。
 */
export function canMove(nodes: SessionNode[], dragId: string, targetParentId: string): boolean {
  if (!dragId || dragId === targetParentId) return false;
  const drag = findNode(nodes, dragId);
  if (!drag) return false;
  if (drag.type === 'session') return true;
  return !isDescendant(drag, targetParentId);
}

/** 由光标在行内的相对高度比例推落点语义。 */
export function dropPositionFrom(ratio: number, targetIsFolder: boolean): DropPosition {
  if (targetIsFolder && ratio >= EDGE_RATIO && ratio <= 1 - EDGE_RATIO) return 'inside';
  return ratio < 0.5 ? 'before' : 'after';
}

/**
 * 计算后端 moveNode 需要的 index。
 *
 * index 的语义是"摘除待拖拽节点之后、目标兄弟列表中的插入位置"，因此同一层内
 * 向后拖动时要先扣除自身造成的下标偏移。把这段算术放在纯函数里并单测，
 * 是因为它错一位的表现是"拖拽结果偶尔差一格"，很难靠人工点击发现。
 */
export function insertionIndex(
  nodes: SessionNode[],
  dragId: string,
  targetParentId: string,
  targetId: string | null,
  position: DropPosition,
): number {
  const siblings = childrenOf(nodes, targetParentId);

  let rawIndex: number;
  if (position === 'inside' || !targetId) {
    rawIndex = siblings.length;
  } else {
    const at = siblings.findIndex((node) => node.id === targetId);
    rawIndex = at < 0 ? siblings.length : position === 'before' ? at : at + 1;
  }

  const dragLoc = findLocation(nodes, dragId);
  const sameList = !!dragLoc && dragLoc.siblings === siblings;
  let index = rawIndex;
  if (sameList && dragLoc!.index < rawIndex) index -= 1;

  const maxIndex = siblings.length - (sameList ? 1 : 0);
  return Math.max(0, Math.min(index, maxIndex));
}

/**
 * 判断这次移动是不是原地不动。
 *
 * 三分区拖拽里"拖到自己所在文件夹的中间"是合法手势，此时插入位置就等于原下标；
 * 直接跳过可以避免一次无意义的写盘与刷新（用户会看到列表闪一下）。
 */
export function isNoopMove(
  nodes: SessionNode[],
  dragId: string,
  targetParentId: string,
  index: number,
): boolean {
  const location = findLocation(nodes, dragId);
  if (!location) return true;
  const currentParentId = location.parent?.id ?? '';
  if (currentParentId !== targetParentId) return false;
  return location.index === index;
}

/**
 * 按搜索词过滤。文件夹名命中时展示其全部子节点；文件夹名未命中但子节点命中时，
 * 只保留命中的子节点。
 */
export function filterTree(nodes: SessionNode[], term: string): SessionNode[] {
  const trimmed = term.trim().toLowerCase();
  if (!trimmed) return nodes;

  return nodes.reduce<SessionNode[]>((acc, node) => {
    const matches =
      node.name.toLowerCase().includes(trimmed) ||
      (node.config?.host ?? '').toLowerCase().includes(trimmed);

    if (node.type === 'folder') {
      if (matches) {
        acc.push(node);
      } else {
        const children = filterTree(node.children ?? [], term);
        if (children.length > 0) acc.push({ ...node, children });
      }
    } else if (matches) {
      acc.push(node);
    }
    return acc;
  }, []);
}

/**
 * 计算某一层"按名称排序"后的 ID 顺序：文件夹在前，各自按中文拼音序。
 *
 * 中文排序刻意放在前端：浏览器的 localeCompare('zh-CN') 能正确处理拼音序，
 * 而后端没有等价的本地化比较。算好后交给 reorderNodes 落库。
 */
export function sortedChildIds(nodes: SessionNode[], parentId: string): string[] {
  const siblings = childrenOf(nodes, parentId);
  const byName = (a: SessionNode, b: SessionNode) =>
    a.name.localeCompare(b.name, 'zh-CN', { sensitivity: 'base' });
  const folders = siblings.filter((node) => node.type === 'folder').sort(byName);
  const sessions = siblings.filter((node) => node.type === 'session').sort(byName);
  return [...folders, ...sessions].map((node) => node.id);
}

/** 递归收集所有文件夹 ID，用于搜索时展开命中项的祖先链。 */
export function ancestorIdsOf(nodes: SessionNode[], id: string): string[] {
  return pathOf(nodes, id)
    .slice(0, -1)
    .map((node) => node.id);
}

/** 为新节点生成同层不重名的名称。 */
export function uniqueNameWithin(siblings: SessionNode[], base: string): string {
  const used = new Set(siblings.map((node) => node.name));
  if (!used.has(base)) return base;
  for (let i = 2; ; i += 1) {
    const candidate = `${base}${i}`;
    if (!used.has(candidate)) return candidate;
  }
}
