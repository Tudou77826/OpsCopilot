import { describe, expect, it } from 'vitest';
import {
  ancestorIdsOf,
  breadcrumb,
  canMove,
  childrenOf,
  collectConnections,
  countConnections,
  dropPositionFrom,
  EDGE_RATIO,
  filterTree,
  findLocation,
  findNode,
  indentForLevel,
  insertionIndex,
  isDescendant,
  MAX_INDENT_LEVEL,
  pathOf,
  sortedChildIds,
  uniqueNameWithin,
} from './treeModel';
import type { SessionNode } from '../ports';

/**
 * 测试树：
 *
 *	生产(folder)
 *	  华东(folder)
 *	    web-1(session)
 *	  db-1(session)
 *	测试(folder)
 *	root-1(session)
 */
function tree(): SessionNode[] {
  return [
    {
      id: 'f-prod',
      name: '生产',
      type: 'folder',
      children: [
        {
          id: 'f-east',
          name: '华东',
          type: 'folder',
          children: [
            { id: 's-web1', name: 'web-1', type: 'session', config: { host: '10.0.0.1', port: 22, user: 'root' } },
          ],
        },
        { id: 's-db', name: 'db-1', type: 'session', config: { host: '10.0.0.2', port: 22, user: 'root' } },
      ],
    },
    { id: 'f-test', name: '测试', type: 'folder' },
    { id: 's-root', name: 'root-1', type: 'session', config: { host: '10.0.0.3', port: 22, user: 'root' } },
  ];
}

describe('查找与路径', () => {
  it('findNode 递归查找，跨层也能命中', () => {
    expect(findNode(tree(), 's-web1')?.name).toBe('web-1');
    expect(findNode(tree(), 'nope')).toBeUndefined();
  });

  it('findLocation 返回父节点、兄弟数组与下标', () => {
    const loc = findLocation(tree(), 's-db');
    expect(loc?.parent?.id).toBe('f-prod');
    expect(loc?.index).toBe(1);
    expect(loc?.siblings.map((n) => n.id)).toEqual(['f-east', 's-db']);

    const rootLoc = findLocation(tree(), 'f-test');
    expect(rootLoc?.parent).toBeNull();
    expect(rootLoc?.index).toBe(1);
  });

  it('childrenOf 对根与文件夹都返回同一层子节点', () => {
    expect(childrenOf(tree(), '').map((n) => n.id)).toEqual(['f-prod', 'f-test', 's-root']);
    expect(childrenOf(tree(), 'f-east').map((n) => n.id)).toEqual(['s-web1']);
    expect(childrenOf(tree(), 's-root')).toEqual([]);
    expect(childrenOf(tree(), 'ghost')).toEqual([]);
  });

  it('pathOf / breadcrumb 给出完整层级', () => {
    expect(pathOf(tree(), 's-web1').map((n) => n.id)).toEqual(['f-prod', 'f-east', 's-web1']);
    expect(breadcrumb(tree(), 's-web1')).toBe('生产 / 华东 / web-1');
    expect(breadcrumb(tree(), 'ghost')).toBe('');
  });

  it('ancestorIdsOf 用于展开搜索命中的祖先链', () => {
    expect(ancestorIdsOf(tree(), 's-web1')).toEqual(['f-prod', 'f-east']);
    expect(ancestorIdsOf(tree(), 's-root')).toEqual([]);
  });
});

describe('连接统计', () => {
  it('collectConnections 递归收集，不把文件夹算进去', () => {
    const folder = findNode(tree(), 'f-prod')!;
    expect(collectConnections(folder).map((n) => n.id)).toEqual(['s-web1', 's-db']);
  });

  it('countConnections 用于删除文件夹前的提示', () => {
    expect(countConnections(findNode(tree(), 'f-prod')!)).toBe(2);
    expect(countConnections(findNode(tree(), 'f-test')!)).toBe(0);
    expect(countConnections(findNode(tree(), 's-root')!)).toBe(1);
  });
});

describe('环检测', () => {
  it('isDescendant 识别任意层级的后代', () => {
    const outer = findNode(tree(), 'f-prod')!;
    expect(isDescendant(outer, 'f-east')).toBe(true);
    expect(isDescendant(outer, 's-web1')).toBe(true);
    expect(isDescendant(outer, 'f-test')).toBe(false);
    expect(isDescendant(outer, '')).toBe(false);
  });

  it('文件夹不能移入自身或其后代，会话不受限', () => {
    const nodes = tree();
    expect(canMove(nodes, 'f-prod', 'f-prod')).toBe(false);
    expect(canMove(nodes, 'f-prod', 'f-east')).toBe(false);
    expect(canMove(nodes, 'f-prod', '')).toBe(true);
    expect(canMove(nodes, 'f-east', 'f-test')).toBe(true);
    expect(canMove(nodes, 's-web1', 'f-test')).toBe(true);
    expect(canMove(nodes, '', 'f-test')).toBe(false);
  });
});

describe('拖拽落点', () => {
  it('文件夹行三分区：中段移入，上下 25% 同级排序', () => {
    expect(dropPositionFrom(0.1, true)).toBe('before');
    expect(dropPositionFrom(EDGE_RATIO, true)).toBe('inside');
    expect(dropPositionFrom(0.5, true)).toBe('inside');
    expect(dropPositionFrom(0.9, true)).toBe('after');
  });

  it('会话行没有"移入"语义，只有上下两段', () => {
    expect(dropPositionFrom(0.5, false)).toBe('after');
    expect(dropPositionFrom(0.2, false)).toBe('before');
  });
});

describe('插入位置计算', () => {
  it('跨层移入文件夹末尾', () => {
    expect(insertionIndex(tree(), 's-root', 'f-east', 'f-east', 'inside')).toBe(1);
  });

  it('跨层插入到某个兄弟之前', () => {
    // 把 s-web1 插到 f-prod 下、s-db 之前 -> [f-east, s-web1, s-db]
    expect(insertionIndex(tree(), 's-web1', 'f-prod', 's-db', 'before')).toBe(1);
  });

  it('同层向后拖动要先扣除自身造成的偏移', () => {
    // 根层 [f-prod, f-test, s-root]，把 f-prod 拖到 f-test 之后 -> [f-test, f-prod, s-root]
    expect(insertionIndex(tree(), 'f-prod', '', 'f-test', 'after')).toBe(1);
  });

  it('同层向前拖动不需要扣偏移', () => {
    // 把 s-root 拖到 f-prod 之前 -> [s-root, f-prod, f-test]
    expect(insertionIndex(tree(), 's-root', '', 'f-prod', 'before')).toBe(0);
  });

  it('拖到自己身上是原地不动', () => {
    // s-web1 在 [s-web1] 里，插到自己之后 -> index 0，仍在原位
    expect(insertionIndex(tree(), 's-web1', 'f-east', 's-web1', 'after')).toBe(0);
    expect(insertionIndex(tree(), 's-web1', 'f-east', 's-web1', 'before')).toBe(0);
  });

  it('目标为空时追加到末尾', () => {
    expect(insertionIndex(tree(), 's-root', 'f-test', null, 'inside')).toBe(0);
    expect(insertionIndex(tree(), 'f-test', 'f-east', null, 'inside')).toBe(1);
  });

  it('结果始终落在合法区间内', () => {
    const nodes = tree();
    for (const dragId of ['f-prod', 'f-test', 's-root', 's-web1', 's-db']) {
      for (const parentId of ['', 'f-prod', 'f-east', 'f-test']) {
        for (const position of ['before', 'after', 'inside'] as const) {
          for (const targetId of [null, 'f-prod', 'f-test', 's-db']) {
            const index = insertionIndex(nodes, dragId, parentId, targetId, position);
            const length = childrenOf(nodes, parentId).length;
            expect(index).toBeGreaterThanOrEqual(0);
            expect(index).toBeLessThanOrEqual(Math.max(0, length - 1) + 1);
          }
        }
      }
    }
  });
});

describe('过滤', () => {
  it('空搜索词返回原树', () => {
    expect(filterTree(tree(), '  ')).toHaveLength(3);
  });

  it('文件夹名命中时保留其全部子节点', () => {
    const result = filterTree(tree(), '华东');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('f-prod');
    expect(result[0].children?.[0].children).toHaveLength(1);
  });

  it('仅子节点命中时只保留命中的子节点', () => {
    const result = filterTree(tree(), 'web-1');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('f-prod');
    expect(result[0].children?.map((n) => n.id)).toEqual(['f-east']);
  });

  it('按主机地址也能命中', () => {
    const result = filterTree(tree(), '10.0.0.2');
    expect(result.map((n) => n.id)).toEqual(['f-prod']);
  });

  it('无命中时返回空', () => {
    expect(filterTree(tree(), 'zzz')).toEqual([]);
  });
});

describe('按名称排序', () => {
  it('文件夹在前、会话在后，各自按中文拼音序', () => {
    const nodes: SessionNode[] = [
      { id: 's2', name: 'zulu', type: 'session' },
      { id: 'f2', name: '测试', type: 'folder' },
      { id: 's1', name: 'alpha', type: 'session' },
      { id: 'f1', name: '生产', type: 'folder' },
    ];
    // 中文按拼音：测(cè) 在 生(shēng) 之前；英文按字母：alpha 在 zulu 之前。
    // 这正是把排序放在前端的原因——后端没有等价的本地化比较。
    expect(sortedChildIds(nodes, '')).toEqual(['f2', 'f1', 's1', 's2']);
  });

  it('只重排指定层，不影响其它层', () => {
    const ids = sortedChildIds(tree(), 'f-prod');
    expect(ids).toEqual(['f-east', 's-db']);
  });
});

describe('缩进与命名', () => {
  it('缩进在最大层级后不再增长，避免深层节点被推出视野', () => {
    expect(indentForLevel(0)).toBe(10);
    expect(indentForLevel(1)).toBe(28);
    expect(indentForLevel(MAX_INDENT_LEVEL)).toBe(indentForLevel(MAX_INDENT_LEVEL + 5));
  });

  it('uniqueNameWithin 在同层不重名', () => {
    const siblings: SessionNode[] = [
      { id: 'a', name: 'folder', type: 'folder' },
      { id: 'b', name: 'folder2', type: 'folder' },
    ];
    expect(uniqueNameWithin(siblings, 'folder')).toBe('folder3');
    expect(uniqueNameWithin(siblings, 'other')).toBe('other');
  });
});
