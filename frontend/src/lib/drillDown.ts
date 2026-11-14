/**
 * 维度层级下钻（issue #136）的纯逻辑：把「点击维度值」物化成对 QueryConfig 的改写。
 *
 * 语义：下钻 = 当前维度列换成层级的**下一级列** + 在父级列上追加一条
 * `== 点击值` 的过滤条件（查询收敛到被点击分支的下一级明细）；上钻 = 反向，
 * 维度换回**上一级列** + 移除那条由下钻追加的过滤。
 *
 * 刻意**物化进 queryConfig**（而不是像 #134 动态槽位那样做成会话态），理由：
 * `?q=` 查询记录快照存的是前端 chart config document，物化后下钻状态随快照
 * 持久化、还原即恢复（验收标准 4）。
 *
 * 下钻条件用 `filterId === 'drill-<level>'` 打标记，上钻/清理时按前缀精确移除，
 * 不会误删用户自己配置的过滤条件。
 */
import type {
  ChartField,
  ColumnHierarchy,
  FieldGroup,
  FilterCondition,
  QueryConfig,
} from '../store';

/** 下钻追加的过滤条件 id 前缀（标记该条件由下钻产生，可被上钻精确清除）。 */
export const DRILL_FILTER_PREFIX = 'drill-';

/** 某字段的下一级列 id（下钻目标）；无层级或已是叶子返回 undefined。 */
export function drillNextId(field: ChartField | undefined): string | undefined {
  const hierarchy = field?.hierarchy;
  if (!hierarchy) {
    return undefined;
  }
  return hierarchy.nextId || undefined;
}

/** 某字段的上一级列 id（上钻目标）；无层级或已是根返回 undefined。 */
export function drillParentId(field: ChartField | undefined): string | undefined {
  const hierarchy = field?.hierarchy;
  if (!hierarchy) {
    return undefined;
  }
  return hierarchy.parentId || undefined;
}

/**
 * 当前是否处于下钻态：维度组里存在「其字段带 nextId」的列。
 * 用于决定是否展示「上钻」入口。
 */
export function isDrilledDown(queryConfig: QueryConfig, fields: ChartField[]): boolean {
  const fieldMap = new Map(fields.map((f) => [f.id, f]));
  for (const binding of dimensionBindingsOf(queryConfig)) {
    const field = fieldMap.get(binding.fieldId);
    if (drillParentId(field)) {
      return true;
    }
  }
  return false;
}

/**
 * 物化一次下钻：把命中 binding 的 fieldId 换成层级下一级列，并在父级列上追加
 * 一条 `== 点击值` 过滤条件（id 标记为 drill-<父级列id>，可被上钻精确清除）。
 *
 * @param queryConfig 当前查询配置（不改原对象，返回新副本）
 * @param bindingId   被点击维度所在槽位的 bindingId
 * @param field       当前生效的维度字段（含 hierarchy 定义）
 * @param drillValue  点击到的父级维度值
 * @param nextId      层级下一级列 id（drillNextId(field) 的结果，调用方保证非空）
 */
export function applyDrillDown(
  queryConfig: QueryConfig,
  bindingId: string,
  field: ChartField,
  drillValue: string | number,
  nextId: string
): QueryConfig {
  const dimensionGroups = queryConfig.dimensionGroups.map((group) => ({
    ...group,
    bindings: group.bindings.map((binding) =>
      binding.bindingId === bindingId ? { ...binding, fieldId: nextId } : binding
    ),
  }));

  const drillFilter: FilterCondition = {
    id: `${DRILL_FILTER_PREFIX}${field.id}`,
    fieldId: field.id,
    operator: 'eq',
    value: drillValue,
    logic: 'and',
  };
  // 同一父级列已下钻过（重复点击/回退重下钻）时替换该条件，不叠加多条。
  const existingDrillIndex = queryConfig.filters.findIndex((f) => f.id === drillFilter.id);
  const filters = [...queryConfig.filters];
  if (existingDrillIndex >= 0) {
    filters[existingDrillIndex] = drillFilter;
  } else {
    filters.push(drillFilter);
  }

  return { ...queryConfig, dimensionGroups, filters };
}

/**
 * 物化一次上钻：把命中 binding 的 fieldId 换回层级上一级列，并移除由下钻追加的
 * 过滤条件（按 drill-<parentId> 前缀精确匹配）。父级列不再下钻时返回原配置。
 */
export function applyDrillUp(
  queryConfig: QueryConfig,
  bindingId: string,
  parentId: string
): QueryConfig {
  let changed = false;
  const dimensionGroups = queryConfig.dimensionGroups.map((group) => ({
    ...group,
    bindings: group.bindings.map((binding) => {
      if (binding.bindingId !== bindingId) {
        return binding;
      }
      changed = true;
      return { ...binding, fieldId: parentId };
    }),
  }));
  if (!changed) {
    return queryConfig;
  }

  const filters = queryConfig.filters.filter(
    (f) => !f.id.startsWith(`${DRILL_FILTER_PREFIX}${parentId}`)
  );
  return { ...queryConfig, dimensionGroups, filters };
}

/** 维度组所有 bindings 的平铺（用于「是否有下钻列」判定）。 */
function dimensionBindingsOf(queryConfig: QueryConfig): FieldGroup['bindings'] {
  return queryConfig.dimensionGroups.flatMap((group) => group.bindings);
}

/** 供测试注入的层级查找：从字段列表按列 id 取 hierarchy。 */
export function hierarchyOf(fields: ChartField[], fieldId: string): ColumnHierarchy | undefined {
  return fields.find((f) => f.id === fieldId)?.hierarchy;
}
