/**
 * 动态维度/指标（issue #134）的纯逻辑：候选槽位投影、会话选择收敛、字段覆盖表。
 *
 * 语义边界：
 * - 候选集存在 `binding.candidates`（不含默认列 `binding.fieldId`）；
 * - 选择是**观看侧会话态**（bindingId → 列 id），绝不写回图表配置；
 * - 选择只对「仍是该槽位候选项」的列生效：字段被删/候选被作者改掉后自动回落默认列，
 *   不让过期会话把查询打到不存在的列上。
 */
import type { BindingInstance, FieldGroup } from '../store';

/** 候选槽位解析只需要列的稳定 id 与展示名（ChartField / DatasetColumn 都满足）。 */
export interface FieldRef {
  id: string;
  name: string;
}

/** 会话选择：bindingId → 本次生效的列 id。 */
export type DynamicFieldSelections = Record<string, string>;

/** 切换器的一个可选项。 */
export interface DynamicSlotOption {
  /** 列的稳定 id（DatasetColumn.id）。 */
  id: string;
  /** 列的展示名。 */
  name: string;
}

/** 一个动态槽位：默认列 + 至少一个候选。 */
export interface DynamicSlot {
  bindingId: string;
  /** 槽位展示名（维度/指标的显示名）；调用方传不出时回落列名。 */
  label: string;
  /** 配置里的默认列（binding.fieldId）。 */
  defaultFieldId: string;
  /** 默认列 + 候选列（按配置顺序，已剔除数据集里不存在的列）。 */
  options: DynamicSlotOption[];
}

/**
 * 某绑定的生效列：会话选择在候选内（或等于默认列）时生效，否则回落默认列。
 * 调用场景：查询组装、渲染标签、切换器当前值——三处必须同口径。
 */
export function effectiveFieldId(
  binding: BindingInstance,
  selections: DynamicFieldSelections | undefined
): string {
  const selected = selections?.[binding.bindingId];
  if (!selected || selected === binding.fieldId) {
    return binding.fieldId;
  }
  return binding.candidates?.includes(selected) ? selected : binding.fieldId;
}

/**
 * 把字段组投影成动态槽位列表：只要候选里**至少有一个列仍存在于数据集**。
 * 字段名解析不到的候选直接剔除（列被删/改数据集后的陈旧配置），避免给出一个
 * 一定会查询失败的选项。
 */
export function dynamicSlotsOf(
  groups: readonly FieldGroup[],
  fields: readonly FieldRef[],
  labelOf?: (binding: BindingInstance, field: FieldRef) => string | undefined
): DynamicSlot[] {
  const fieldMap = new Map(fields.map((field) => [field.id, field]));
  const slots: DynamicSlot[] = [];
  for (const group of groups) {
    for (const binding of group.bindings) {
      const defaultField = fieldMap.get(binding.fieldId);
      if (!defaultField) {
        continue;
      }
      const candidateIds = (binding.candidates ?? []).filter((id) => fieldMap.has(id));
      if (candidateIds.length === 0) {
        continue;
      }
      const options: DynamicSlotOption[] = [
        { id: defaultField.id, name: defaultField.name },
        ...candidateIds.map((id) => {
          const field = fieldMap.get(id) as FieldRef;
          return { id: field.id, name: field.name };
        }),
      ];
      slots.push({
        bindingId: binding.bindingId,
        label: labelOf?.(binding, defaultField) || defaultField.name,
        defaultFieldId: binding.fieldId,
        options,
      });
    }
  }
  return slots;
}

/**
 * 由动态槽位 + 会话选择产出查询/渲染用的覆盖表（bindingId → 列 id）。
 * 只收录**确实与默认列不同**的槽位：形状与「没有动态槽位」的请求保持逐字节一致。
 */
export function fieldOverridesOf(
  slots: readonly DynamicSlot[],
  selections: DynamicFieldSelections | undefined
): Record<string, string> {
  const overrides: Record<string, string> = {};
  for (const slot of slots) {
    const selected = selections?.[slot.bindingId];
    if (!selected || selected === slot.defaultFieldId) {
      continue;
    }
    if (slot.options.some((option) => option.id === selected)) {
      overrides[slot.bindingId] = selected;
    }
  }
  return overrides;
}

/**
 * 把会话选择物化进一份 QueryConfig 副本（bindings 的 fieldId 换成生效列），用于
 * **查询记录/分享快照**：`?q=` 直链要带着「当时看的是哪个候选」还原。图表自身的
 * 持久化配置绝不走这条路径（保持默认列）。
 */
export function applyFieldOverrides<T extends QueryConfigLike>(
  queryConfig: T,
  overrides: Record<string, string>
): T {
  if (Object.keys(overrides).length === 0) {
    return queryConfig;
  }
  const applyGroups = (groups: FieldGroup[]): FieldGroup[] =>
    groups.map((group) => ({
      ...group,
      bindings: group.bindings.map((binding) => {
        const selected = overrides[binding.bindingId];
        if (!selected || selected === binding.fieldId) {
          return binding;
        }
        // 物化后把「原默认列」补进候选集：快照还原出来仍要能在候选间来回切
        // （只换 fieldId 会让默认列不再出现在候选里，切换器退化成单选项）。
        const candidates = [binding.fieldId, ...(binding.candidates ?? [])].filter(
          (candidate, index, all) => candidate !== selected && all.indexOf(candidate) === index
        );
        const next: BindingInstance = { ...binding, fieldId: selected };
        if (candidates.length > 0) {
          next.candidates = candidates;
        } else {
          delete next.candidates;
        }
        return next;
      }),
    }));
  return {
    ...queryConfig,
    dimensionGroups: applyGroups(queryConfig.dimensionGroups),
    metricGroups: applyGroups(queryConfig.metricGroups),
  };
}

/** applyFieldOverrides 只需要字段组 + 其余键透传的最小形状（避免依赖 store 的完整类型）。 */
export interface QueryConfigLike {
  dimensionGroups: FieldGroup[];
  metricGroups: FieldGroup[];
}

/**
 * 把会话选择里已失效的键剔掉（槽位删除 / 候选被改 / 数据集切换后不再存在）。
 * 返回原对象（引用相等）表示无需清理，避免无谓的重渲染与重查。
 */
export function pruneSelections(
  selections: DynamicFieldSelections,
  slots: readonly DynamicSlot[]
): DynamicFieldSelections {
  const valid = new Map(
    slots.map((slot) => [slot.bindingId, new Set(slot.options.map((option) => option.id))])
  );
  let changed = false;
  const next: DynamicFieldSelections = {};
  for (const [bindingId, fieldId] of Object.entries(selections)) {
    if (valid.get(bindingId)?.has(fieldId)) {
      next[bindingId] = fieldId;
    } else {
      changed = true;
    }
  }
  return changed ? next : selections;
}
