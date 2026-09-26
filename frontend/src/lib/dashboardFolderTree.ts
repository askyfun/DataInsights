import type { Dashboard, DashboardFolder } from '../api';

/**
 * 仪表盘归档树的**纯逻辑层**：把后端的扁平文件夹数组 + 仪表盘列表折叠成一棵
 * antd Tree 能直接吃的节点树。后端刻意不组树（移动只写一行），所以树形的全部
 * 规则都住在这一份文件里，可脱离 React 单测。
 *
 * 三条不可见的约定（改这里之前先读完）：
 *
 * 1. **悬空引用退化成根级**：folder 的 parent_id、dashboard 的 folder_id 指向
 *    不存在/已软删的夹时，不报错也不丢行，直接挂到根下。后端刻意不做级联清洗
 *    （无 FK），前端必须兜住这个退化形态，否则整棵树少一支、仪表盘凭空消失。
 * 2. **环退化成根级**：环在服务端就被拒绝，但历史数据可能已有环。只有**自己在环上**
 *    的夹退化成根级（环成员全退化 ⇒ 剩下的父子边必然是森林，buildFolder 不会无限
 *    递归）；挂在环成员下面的正常夹**不受连坐**，仍按原父级嵌套。
 * 3. **父级未加载时子级挂根**：list 不分页、一次给全表，所以这条只在未来加过滤
 *    参数时才会命中；显式写出来而不是靠 Map 查找失败静默兜住。
 */

/** 树节点 key 的前缀：夹与盘共用一棵树，靠前缀区分身份。 */
export const FOLDER_KEY_PREFIX = 'f:';
export const DASHBOARD_KEY_PREFIX = 'd:';
/** 根节点 key（字面量不可能与 UUID 撞车）。 */
export const ROOT_KEY = 'root';

/** 树节点（antd Tree 的 DataNode 形态，本项目只用到这几个键）。 */
export interface FolderTreeNode {
  key: string;
  title: string;
  /** 夹节点 / 根节点为 'folder'（可接收拖入），仪表盘叶子为 'dashboard'。 */
  kind: 'folder' | 'dashboard';
  /** kind 为 folder 时是夹 id；根节点与仪表盘叶子为 null。 */
  folderId: string | null;
  /** 夹内仪表盘数（不含子夹），叶子节点为 0。 */
  dashboardCount: number;
  children?: FolderTreeNode[];
}

/** 当前选中的归档位置：根 = 「全部仪表盘」（不过滤），否则是某个夹。 */
export type FolderSelection = { kind: 'root' } | { kind: 'folder'; id: string };

/**
 * 拖拽移动意图：盘进夹 / 夹进夹。`intoFolderId` 为 null 表示落到根
 * （盘 = 移出到未归档，夹 = 移到根级）。
 */
export type TreeMoveIntent =
  | { kind: 'dashboard'; dashboardId: string; intoFolderId: string | null }
  | { kind: 'folder'; folderId: string; intoFolderId: string | null };

export function folderKey(id: string): string {
  return FOLDER_KEY_PREFIX + id;
}

export function dashboardKey(id: string): string {
  return DASHBOARD_KEY_PREFIX + id;
}

/** 节点 key → 选中位置。仪表盘叶子的 key 也走这里：选中它等同于看它所在的夹。 */
export function selectionFromKey(
  key: string,
  fallback: FolderSelection = { kind: 'root' }
): FolderSelection {
  if (key === ROOT_KEY) return { kind: 'root' };
  if (key.startsWith(FOLDER_KEY_PREFIX)) {
    return { kind: 'folder', id: key.slice(FOLDER_KEY_PREFIX.length) };
  }
  return fallback;
}

/** key → 夹 id（只有夹节点返回非 null）。 */
export function folderIdFromKey(key: string): string | null {
  return key.startsWith(FOLDER_KEY_PREFIX) ? key.slice(FOLDER_KEY_PREFIX.length) : null;
}

/** key → 仪表盘 id。 */
export function dashboardIdFromKey(key: string): string | null {
  return key.startsWith(DASHBOARD_KEY_PREFIX) ? key.slice(DASHBOARD_KEY_PREFIX.length) : null;
}

/** 夹名排序：zh-CN localeCompare（英文/拼音夹名与中文名都按本地习惯排）。 */
const byName = (a: DashboardFolder, b: DashboardFolder) => a.name.localeCompare(b.name, 'zh-CN');

/** 盘排序：更新时间新的在前（与列表页同口径）。 */
const byUpdated = (a: Dashboard, b: Dashboard) => b.updated_at.localeCompare(a.updated_at);

/**
 * 折叠成树。`folders` / `dashboards` 的顺序都不影响结果（内部按名称/时间重排），
 * 因此调用方不需要预排序。`rootTitle` 由调用方注入（纯逻辑层不碰 i18n）。
 */
export function buildDashboardFolderTree(
  folders: DashboardFolder[],
  dashboards: Dashboard[],
  rootTitle: string
): FolderTreeNode {
  const byId = new Map<string, DashboardFolder>();
  for (const f of folders) byId.set(f.id, f);

  // 每个夹下的直接子夹 / 直接仪表盘。
  const childFolders = new Map<string, DashboardFolder[]>();
  const rootFolders: DashboardFolder[] = [];

  // 归属判定分两步，顺序不能反：
  //   1. parent_id 为 NULL / 指向不存在的夹 → 根级（悬空退化，约定 1）；
  //   2. 该夹**自己处在环上**（从它的父级上溯能绕回它自己）→ 根级（约定 2）；
  //   3. 其余情况挂到现有父级下 —— 哪怕那个父级自己刚被判定成环成员退到了根。
  //
  // 为什么第 3 步重要：环成员必须全部退化，剩下的父子边才构成森林（buildFolder
  // 才不会无限递归）；但**环外的子夹不该被连坐**。早期版本把「祖先链走不到头」
  // 也当成退化条件，于是 a↔b 成环时连挂在 a 下的正常子夹 d 也被冲到根上，
  // 整个嵌套结构全平 —— 树没崩，但「多层嵌套」这条验收直接没实现。
  const onCycle = (f: DashboardFolder): boolean => {
    let cursor = f.parent_id;
    // 上溯步数上界 = 夹总数：超过就说明进了不包含 f 的环（第 3 步的形态），
    // 而不是 f 自己在环上。
    for (let depth = 0; depth <= folders.length; depth++) {
      if (!cursor) return false; // 走到根：无环
      const next = byId.get(cursor);
      if (!next) return false; // 悬空：无环
      if (next.id === f.id) return true; // 绕回自己：f 在环上
      cursor = next.parent_id;
    }
    return false;
  };

  for (const f of folders) {
    const parentExists = !!f.parent_id && byId.has(f.parent_id);
    if (!parentExists || onCycle(f)) {
      rootFolders.push(f);
      continue;
    }
    const siblings = childFolders.get(f.parent_id as string) ?? [];
    siblings.push(f);
    childFolders.set(f.parent_id as string, siblings);
  }

  const childDashboards = new Map<string, Dashboard[]>();
  const rootDashboards: Dashboard[] = [];
  for (const d of dashboards) {
    const target = d.folder_id ? byId.get(d.folder_id) : undefined;
    if (!target) {
      rootDashboards.push(d);
      continue;
    }
    const bucket = childDashboards.get(target.id) ?? [];
    bucket.push(d);
    childDashboards.set(target.id, bucket);
  }

  const buildFolder = (f: DashboardFolder): FolderTreeNode => {
    const kids = childDashboards.get(f.id) ?? [];
    return {
      key: folderKey(f.id),
      title: f.name,
      kind: 'folder',
      folderId: f.id,
      dashboardCount: kids.length,
      children: [
        ...(childFolders.get(f.id) ?? []).sort(byName).map(buildFolder),
        ...[...kids].sort(byUpdated).map(dashboardNode),
      ],
    };
  };

  const dashboardNode = (d: Dashboard): FolderTreeNode => ({
    key: dashboardKey(d.id),
    title: d.name,
    kind: 'dashboard',
    folderId: d.folder_id ?? null,
    dashboardCount: 0,
  });

  return {
    key: ROOT_KEY,
    title: rootTitle,
    kind: 'folder',
    folderId: null,
    dashboardCount: rootDashboards.length,
    children: [
      ...rootFolders.sort(byName).map(buildFolder),
      ...[...rootDashboards].sort(byUpdated).map(dashboardNode),
    ],
  };
}

/**
 * 落点 key → 目标夹 id。
 *
 * ⚠️ 刻意**只吃 key**，不吃节点上的自定义字段：rc-tree 在 onDrop 里给的是
 * `convertNodePropsToEventData` 转换后的对象，自定义字段能否原样带出来属于库的
 * 实现细节（真浏览器里实测根节点的自定义字段就成了 undefined）。key 是 Tree 的
 * 主键、必然存在，所以「落在哪个夹」这件事只从 key 推导：
 *   - root → null（未归档 / 根级）
 *   - f:<id> → 该夹
 *   - d:<id> → 那个仪表盘当前所在的夹（落在一个盘上 = 落进它所在的夹）
 */
export function targetFolderIdForKey(key: string, dashboards: Dashboard[]): string | null {
  if (key === ROOT_KEY) return null;
  const folderId = folderIdFromKey(key);
  if (folderId) return folderId;
  const dashboardId = dashboardIdFromKey(key);
  if (dashboardId) return dashboards.find((d) => d.id === dashboardId)?.folder_id ?? null;
  return null;
}

/**
 * 拖拽移动文件夹时的守卫：被拖的夹不能落进自己的子树。
 * 同样只依赖 key（`f:<id>`）。
 */
export function isFolderDropAllowed(
  dragKey: string,
  dropKey: string,
  folders: DashboardFolder[]
): boolean {
  const dragFolderId = folderIdFromKey(dragKey);
  if (!dragFolderId) return true; // 拖的是仪表盘：任何落点都行
  const dropFolderId = dropKey === ROOT_KEY ? null : folderIdFromKey(dropKey);
  if (!dropFolderId) return true; // 落在盘上/根上：不涉及「拖进自己子树」
  // 方向：落点必须不是被拖夹的后代（也不能是它自己）。
  return !isSelfOrDescendantFolder(folders, dropFolderId, dragFolderId);
}

/**
 * 把 antd Tree 的 onDrop 参数归一成移动意图（盘与夹都能拖，验收要求两者都可移动）。
 *
 * `dropNodeFolderId` 由调用方从被落节点上取：夹节点取自身 id、根节点取 null、
 * 仪表盘叶子取该盘的 folder_id —— 落在一个盘上等同于落进它所在的夹，符合直觉。
 * 落在夹的 gap（前后排位的空隙）上也进该夹：第一期不提供夹内排序，gap 不承载语义。
 *
 * 返回 null = 不该发请求（落点就是当前位置，或根节点被当成拖动目标）。
 */
export function resolveTreeMove(args: {
  dragKey: string;
  dropKey: string;
  /** 落点节点所属的夹 id；根级为 null。 */
  dropNodeFolderId: string | null;
  /** 被拖对象（盘或夹）当前所在的夹 id；未归档 / 根级为 null。 */
  currentFolderId: string | null;
}): TreeMoveIntent | null {
  // 根节点本身不可拖（它只是「全部」的视图入口）。
  if (args.dragKey === ROOT_KEY) return null;

  const intoFolderId = args.dropKey === ROOT_KEY ? null : args.dropNodeFolderId;

  const dashboardId = dashboardIdFromKey(args.dragKey);
  if (dashboardId) {
    if (intoFolderId === args.currentFolderId) return null;
    return { kind: 'dashboard', dashboardId, intoFolderId };
  }

  const folderId = folderIdFromKey(args.dragKey);
  if (folderId) {
    // 夹的 currentFolderId 就是它的 parent_id；根级夹为 null。
    if (intoFolderId === args.currentFolderId) return null;
    return { kind: 'folder', folderId, intoFolderId };
  }
  return null;
}

/**
 * `candidateId` 是不是 `ancestorId` 的**自身或后代**（环守卫的前端形态）。
 *
 * 用途有两处：allowDrop 把「把 A 拖进 A 的子孙」直接判为不可放置（不让用户拖出
 * 一个必然被后端 20400 拒绝的落点），以及 onDrop 的兜底。
 * 与后端 `ensureNotDescendant` 同方向（从 candidate 往上爬祖先链），步数同样有界，
 * 环/悬空都退化成「不是后代」—— 那种数据本来就只会由后端在写入时拒绝。
 */
export function isSelfOrDescendantFolder(
  folders: DashboardFolder[],
  candidateId: string,
  ancestorId: string
): boolean {
  const byId = new Map(folders.map((f) => [f.id, f]));
  let cursor: string | null = candidateId;
  for (let depth = 0; depth <= folders.length; depth++) {
    if (!cursor) return false;
    if (cursor === ancestorId) return true;
    cursor = byId.get(cursor)?.parent_id ?? null;
  }
  return false;
}

/** 展开 key 集合 → 夹 id 集合（Tree 的 expandedKeys 只关心夹）。 */
export function expandedFolderIds(keys: string[]): Set<string> {
  const out = new Set<string>();
  for (const k of keys) {
    const id = folderIdFromKey(k);
    if (id) out.add(id);
  }
  return out;
}

/**
 * 树上「还有多少个盘」的可见性判定：选中某夹时，列表只显示该夹的盘；
 * 选中根时显示全部。放在这里是为了让「表格里看到的行」与「树上显示的数量」
 * 用同一条口径（第一期树上标的是直接子项数，不带递归合计）。
 */
export function filterDashboardsBySelection(
  dashboards: Dashboard[],
  selection: FolderSelection
): Dashboard[] {
  if (selection.kind === 'root') return dashboards;
  return dashboards.filter((d) => d.folder_id === selection.id);
}
