import { describe, expect, it } from 'vitest';
import type { Dashboard, DashboardFolder } from '../../api';
import type { FolderTreeNode } from '../../lib/dashboardFolderTree';
import {
  buildDashboardFolderTree,
  dashboardKey,
  filterDashboardsBySelection,
  folderKey,
  isFolderDropAllowed,
  isSelfOrDescendantFolder,
  ROOT_KEY,
  resolveTreeMove,
  selectionFromKey,
  targetFolderIdForKey,
} from '../../lib/dashboardFolderTree';

const folder = (id: string, name: string, parentId: string | null = null): DashboardFolder =>
  ({ id, name, parent_id: parentId, created_at: '', updated_at: '' }) as DashboardFolder;

const dash = (id: string, name: string, folderId: string | null = null): Dashboard =>
  ({ id, name, folder_id: folderId, created_at: '', updated_at: '' }) as Dashboard;

const kids = (node: { children?: { key: string }[] }) => (node.children ?? []).map((c) => c.key);

describe('buildDashboardFolderTree', () => {
  it('把根级文件夹与根级仪表盘挂在 root 下', () => {
    const tree = buildDashboardFolderTree([folder('f1', '甲')], [dash('d1', '盘一')], '全部仪表盘');
    expect(tree.key).toBe(ROOT_KEY);
    expect(kids(tree)).toEqual([folderKey('f1'), dashboardKey('d1')]);
  });

  it('多层嵌套按 parent_id 组装', () => {
    const tree = buildDashboardFolderTree(
      [folder('f1', '一层'), folder('f2', '二层', 'f1'), folder('f3', '三层', 'f2')],
      [],
      '全部'
    );
    const f1 = tree.children?.[0];
    expect(f1?.key).toBe(folderKey('f1'));
    const f2 = f1?.children?.[0];
    expect(f2?.key).toBe(folderKey('f2'));
    expect(f2?.children?.[0]?.key).toBe(folderKey('f3'));
  });

  it('同层文件夹按名称排（ASCII 名，避免依赖运行环境的中文排序规则）', () => {
    const tree = buildDashboardFolderTree(
      [folder('f1', 'B'), folder('f2', 'A'), folder('f3', 'C')],
      [],
      'root'
    );
    expect(kids(tree)).toEqual([folderKey('f2'), folderKey('f1'), folderKey('f3')]);
  });

  it('仪表盘文件夹混排：文件夹在前、盘在后', () => {
    const tree = buildDashboardFolderTree(
      [folder('f1', '夹')],
      [dash('d1', '盘', 'f1'), dash('d2', '盘2', 'f1')],
      '全部'
    );
    const f1 = tree.children?.[0];
    expect(kids(f1 ?? { children: [] })).toEqual([dashboardKey('d1'), dashboardKey('d2')]);
    expect(f1?.dashboardCount).toBe(2);
  });

  it('悬空 parent_id 的文件夹退化成根级（后端不做级联清洗）', () => {
    const tree = buildDashboardFolderTree([folder('f1', '孤儿', 'ghost')], [], '全部');
    expect(kids(tree)).toEqual([folderKey('f1')]);
  });

  it('悬空 folder_id 的仪表盘退化成根级', () => {
    const tree = buildDashboardFolderTree([], [dash('d1', '孤儿盘', 'ghost')], '全部');
    expect(kids(tree)).toEqual([dashboardKey('d1')]);
  });

  it('两节点环：两个夹都退化成根级，一个都不丢', () => {
    // f1↔f2 互为父子。旧实现用单次遍历 + 全局 attached 集合断环，结果两个夹都被
    // 判成「已挂接」，谁都没进 rootFolders —— 整支从树上凭空消失（用户看到的是
    // 「文件夹被删了」）。正确语义是环成员**各自**退化成根级。
    const tree = buildDashboardFolderTree(
      [folder('b', 'B', 'a'), folder('a', 'A', 'b')],
      [],
      'root'
    );
    expect(kids(tree).sort()).toEqual([folderKey('a'), folderKey('b')].sort());
    // 退化后互不嵌套（否则 buildFolder 会无限递归）。
    for (const child of tree.children ?? []) {
      expect(child.children ?? []).toHaveLength(0);
    }
  });

  it('三节点环 + 环外子夹：环成员退化，环外的子夹仍挂在原父级下', () => {
    // a/b/c 成环 → 三者退到根；d 不在环上（从 d 上溯只会绕进环，不会回到 d），
    // 所以 d 仍挂在 a 下。环成员全部退化 ⇒ 剩余父子边必是森林，不会无限递归。
    const tree = buildDashboardFolderTree(
      [folder('a', 'A', 'c'), folder('b', 'B', 'a'), folder('c', 'C', 'b'), folder('d', 'D', 'a')],
      [dash('d1', '盘', 'a')],
      'root'
    );
    expect(kids(tree).sort()).toEqual([folderKey('a'), folderKey('b'), folderKey('c')].sort());
    const a = tree.children?.find((c) => c.key === folderKey('a'));
    expect(kids(a ?? { children: [] })).toEqual([folderKey('d'), dashboardKey('d1')]);
  });

  it('自环（parent 指向自己）退化成根级且不递归崩栈', () => {
    const tree = buildDashboardFolderTree([folder('f1', '自环', 'f1')], [], '全部');
    expect(kids(tree)).toEqual([folderKey('f1')]);
    expect(tree.children?.[0]?.children ?? []).toHaveLength(0);
  });

  it('长链嵌套全部可达', () => {
    const chain = Array.from({ length: 6 }, (_, i) =>
      folder(`f${i}`, `层${i}`, i === 0 ? null : `f${i - 1}`)
    );
    const tree = buildDashboardFolderTree(chain, [], '全部');
    // 一路往下钻：任何一层缺失都会让下面的 key 断言失败（而不是悄悄用 ! 蒙过去）。
    let node: FolderTreeNode | undefined = tree;
    for (let i = 0; i < 6; i++) {
      node = node?.children?.[0];
      expect(node?.key).toBe(folderKey(`f${i}`));
    }
  });
});

describe('resolveTreeMove', () => {
  it('拖仪表盘进文件夹', () => {
    expect(
      resolveTreeMove({
        dragKey: dashboardKey('d1'),
        dropKey: folderKey('f1'),
        dropNodeFolderId: 'f1',
        currentFolderId: null,
      })
    ).toEqual({ kind: 'dashboard', dashboardId: 'd1', intoFolderId: 'f1' });
  });

  it('拖文件夹进文件夹', () => {
    expect(
      resolveTreeMove({
        dragKey: folderKey('f2'),
        dropKey: folderKey('f1'),
        dropNodeFolderId: 'f1',
        currentFolderId: null,
      })
    ).toEqual({ kind: 'folder', folderId: 'f2', intoFolderId: 'f1' });
  });

  it('拖到 root：盘 = 移出到未归档，夹 = 移到根级', () => {
    expect(
      resolveTreeMove({
        dragKey: dashboardKey('d1'),
        dropKey: ROOT_KEY,
        dropNodeFolderId: null,
        currentFolderId: 'f1',
      })
    ).toEqual({ kind: 'dashboard', dashboardId: 'd1', intoFolderId: null });
    expect(
      resolveTreeMove({
        dragKey: folderKey('f2'),
        dropKey: ROOT_KEY,
        dropNodeFolderId: null,
        currentFolderId: 'f1',
      })
    ).toEqual({ kind: 'folder', folderId: 'f2', intoFolderId: null });
  });

  it('落回当前位置不产生请求', () => {
    expect(
      resolveTreeMove({
        dragKey: dashboardKey('d1'),
        dropKey: folderKey('f1'),
        dropNodeFolderId: 'f1',
        currentFolderId: 'f1',
      })
    ).toBeNull();
    expect(
      resolveTreeMove({
        dragKey: folderKey('f2'),
        dropKey: folderKey('f1'),
        dropNodeFolderId: 'f1',
        currentFolderId: 'f1',
      })
    ).toBeNull();
  });

  it('根节点不可拖（它只是视图入口，没有可移动的实体）', () => {
    expect(
      resolveTreeMove({
        dragKey: ROOT_KEY,
        dropKey: folderKey('f1'),
        dropNodeFolderId: 'f1',
        currentFolderId: null,
      })
    ).toBeNull();
  });
});

describe('targetFolderIdForKey', () => {
  const rows = [dash('d1', '盘一', 'f1'), dash('d2', '盘二', null)];

  it('根 = null（未归档 / 根级）', () => {
    expect(targetFolderIdForKey(ROOT_KEY, rows)).toBeNull();
  });

  it('文件夹 key = 该夹', () => {
    expect(targetFolderIdForKey(folderKey('f1'), rows)).toBe('f1');
  });

  it('仪表盘叶子 = 该盘所在的夹（落在盘上等于落进它的夹）', () => {
    expect(targetFolderIdForKey(dashboardKey('d1'), rows)).toBe('f1');
    expect(targetFolderIdForKey(dashboardKey('d2'), rows)).toBeNull();
  });

  it('只依赖 key：不读节点上的自定义字段（rc-tree 转换后那些字段不保证存在）', () => {
    // 未知盘 id 也只能退化成 null，不会抛。
    expect(targetFolderIdForKey(dashboardKey('ghost'), rows)).toBeNull();
  });
});

describe('isFolderDropAllowed', () => {
  const folders = [folder('a', 'A'), folder('b', 'B', 'a'), folder('c', 'C', 'b')];

  it('仪表盘可以落在任何位置', () => {
    expect(isFolderDropAllowed(dashboardKey('d1'), folderKey('a'), folders)).toBe(true);
    expect(isFolderDropAllowed(dashboardKey('d1'), ROOT_KEY, folders)).toBe(true);
  });

  it('夹不能落在自己的子夹里', () => {
    expect(isFolderDropAllowed(folderKey('a'), folderKey('b'), folders)).toBe(false);
    expect(isFolderDropAllowed(folderKey('a'), folderKey('c'), folders)).toBe(false);
  });

  it('夹不能落在自己身上', () => {
    expect(isFolderDropAllowed(folderKey('a'), folderKey('a'), folders)).toBe(false);
  });

  it('夹可以往上或平级移动', () => {
    expect(isFolderDropAllowed(folderKey('c'), folderKey('a'), folders)).toBe(true);
    expect(isFolderDropAllowed(folderKey('b'), ROOT_KEY, folders)).toBe(true);
  });
});

describe('isSelfOrDescendantFolder', () => {
  const chain = [folder('a', 'A'), folder('b', 'B', 'a'), folder('c', 'C', 'b')];

  it('自身算后代（不能拖到自己里）', () => {
    expect(isSelfOrDescendantFolder(chain, 'a', 'a')).toBe(true);
  });

  it('孙子算后代', () => {
    expect(isSelfOrDescendantFolder(chain, 'c', 'a')).toBe(true);
  });

  it('父级不算后代（向上拖是合法的）', () => {
    expect(isSelfOrDescendantFolder(chain, 'a', 'c')).toBe(false);
  });

  it('不存在的 id 不算后代', () => {
    expect(isSelfOrDescendantFolder(chain, 'ghost', 'a')).toBe(false);
  });

  it('已经是环的数据不会把浏览器拖进死循环', () => {
    const loop = [folder('a', 'A', 'b'), folder('b', 'B', 'a')];
    expect(isSelfOrDescendantFolder(loop, 'a', 'b')).toBe(true);
    expect(isSelfOrDescendantFolder(loop, 'x', 'b')).toBe(false);
  });
});

describe('selectionFromKey / 过滤', () => {
  it('root key = 全部（不过滤）', () => {
    expect(selectionFromKey(ROOT_KEY)).toEqual({ kind: 'root' });
  });

  it('文件夹 key = 选中该文件夹', () => {
    expect(selectionFromKey(folderKey('f1'))).toEqual({ kind: 'folder', id: 'f1' });
  });

  it('根视图列出全部，含未归档；选中文件夹只剩其直属仪表盘', () => {
    const rows = [dash('d1', '甲', 'f1'), dash('d2', '乙', null), dash('d3', '丙', 'f2')];
    expect(filterDashboardsBySelection(rows, { kind: 'root' })).toHaveLength(3);
    expect(
      filterDashboardsBySelection(rows, { kind: 'folder', id: 'f1' }).map((d) => d.id)
    ).toEqual(['d1']);
  });
});
