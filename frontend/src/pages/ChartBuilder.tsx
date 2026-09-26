import {
  AppstoreOutlined,
  BarChartOutlined,
  CodeOutlined,
  DownOutlined,
  ExportOutlined,
  FieldBinaryOutlined,
  FunctionOutlined,
  LinkOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  Alert,
  Button,
  Card,
  ColorPicker,
  Drawer,
  Dropdown,
  Empty,
  Input,
  InputNumber,
  Layout,
  Modal,
  message,
  Select,
  Space,
  Spin,
  Switch,
  Tooltip,
  Typography,
} from 'antd';
import ReactECharts from 'echarts-for-react';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  Chart,
  type ChartDataResponse,
  ChartDimensionField,
  ChartDimensionGroup,
  ChartMetricField,
  ChartMetricGroup,
  ChartQueryAggregation,
  ChartQueryRequest,
  isPivotV2Payload,
  type QueryRecord,
  queriesApi,
} from '../api';
import {
  chartDefinitions,
  normalizeQueryConfigForChartType,
} from '../components/ChartBuilder/chartDefinitions';
import DraggableField, {
  FieldDragPreview,
  fieldTagColor,
} from '../components/ChartBuilder/DraggableField';
import { BindingDragData, BindingSlotDropData } from '../components/ChartBuilder/FieldDropZone';
import FieldSettingsModal, {
  type FieldSettingsValue,
} from '../components/ChartBuilder/FieldSettingsModal';
import FilterConfigModal, {
  type FilterConfigPatch,
} from '../components/ChartBuilder/FilterConfigModal';
import FilterDropZone from '../components/ChartBuilder/FilterDropZone';
import FilterValueControl from '../components/ChartBuilder/FilterValueControl';
import KpiCard from '../components/ChartBuilder/KpiCard';
import PivotTable from '../components/ChartBuilder/PivotTable';
import QueryConfigRow from '../components/ChartBuilder/QueryConfigRow';
import TableChart from '../components/ChartBuilder/TableChart';
import DateFilterControl from '../components/DateFilter/DateFilterControl';
import DateFilterModal, {
  type DateFilterModalPayload,
} from '../components/DateFilter/DateFilterModal';
import {
  type ChartConfigDocument,
  type ChartType,
  migrateChartConfig,
} from '../lib/chartConfigSchema';
import { buildChartOption, isEmptyPayload } from '../lib/chartOptions';
import { classifyFieldKind, normalizeDataType } from '../lib/dataTypes';
import {
  type DateFilterIntent,
  type DateFilterValue,
  dateFilterValueFromLegacy,
  expandDateFilterIntent,
  isDateFilterValue,
} from '../lib/dateFilter';
import {
  buildChartConfigDocument,
  buildQuerySpecDocument,
  parseQuerySpecDocument,
} from '../lib/querySpec';
import { useResolvedTheme } from '../lib/theme';
import {
  BindingInstance,
  BoundField,
  ChartConfig,
  ChartField,
  ChartQueryOptions,
  ChartStyleConfig,
  FieldGroup,
  FilterCondition,
  QueryConfig,
  useStore,
} from '../store';

const { Header, Sider, Content } = Layout;
const { Text } = Typography;

interface DragFieldData {
  type: 'field';
  field: ChartField;
  fieldType: ChartField['type'];
}

/**
 * 判断 dnd-kit active data 是否携带图表字段信息。
 * 调用场景：拖拽开始和结束时都需要安全读取 active.data.current。
 * 主要逻辑：校验 data.type 与 field 对象结构，避免在未知拖拽源上误取值。
 */
const isDragFieldData = (value: unknown): value is DragFieldData => {
  if (!value || typeof value !== 'object') {
    return false;
  }

  const data = value as Partial<DragFieldData>;
  return data.type === 'field' && !!data.field;
};

/**
 * 从 dnd-kit 事件数据中提取当前拖拽字段。
 * 调用场景：拖拽 overlay 预览和 drop 处理共用同一套字段解析逻辑。
 * 主要逻辑：只有侧边栏字段拖拽才返回字段对象，其他拖拽源统一返回 null。
 */
const getDraggedField = (value: unknown): ChartField | null => {
  if (!isDragFieldData(value)) {
    return null;
  }
  return value.field;
};

/**
 * 判断 dnd-kit active data 是否来自查询配置区里的字段标签。
 * 调用场景：配置区内部的"搬字段"（换组 / 换序）与侧边栏"新加字段"共用同一个 DndContext，
 * 必须在入口处区分，否则两种拖拽会走错处理分支。
 */
const isDragBindingData = (value: unknown): value is BindingDragData => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const data = value as Partial<BindingDragData>;
  return (
    data.type === 'binding-source' &&
    typeof data.bindingId === 'string' &&
    (data.kind === 'dimension' || data.kind === 'metric') &&
    typeof data.groupIndex === 'number'
  );
};

/** 字段标签自身作为落点的载荷判别（插到该标签之前）。 */
const isBindingSlotData = (value: unknown): value is BindingSlotDropData => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const data = value as Partial<BindingSlotDropData>;
  return (
    data.type === 'binding-slot' &&
    (data.kind === 'dimension' || data.kind === 'metric') &&
    typeof data.groupIndex === 'number' &&
    typeof data.index === 'number'
  );
};

/** 字段组区域（维度/指标/筛选）落点载荷判别。 */
interface ZoneDropData {
  type: 'dimension' | 'metric' | 'filter';
  groupIndex?: number;
}

const isZoneDropData = (value: unknown): value is ZoneDropData => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const data = value as Partial<ZoneDropData>;
  return data.type === 'dimension' || data.type === 'metric' || data.type === 'filter';
};

/** 拖拽浮层预览内容（侧边栏字段与配置区字段标签共用）。 */
interface DragPreview {
  label: string;
  color: string;
}

/**
 * 按当前图表定义裁剪字段组，只保留当前类型实际会渲染的那部分配置。
 * 调用场景：切换图表类型后，queryConfig 可能还残留上一个图表的额外组；构造查询请求时不能把隐藏组一并发送。
 * 主要逻辑：分别统计当前定义需要的维度组/指标组数量，再按顺序截取对应 groups。
 */
const getActiveFieldGroups = (chartType: ChartConfig['chartType'], queryConfig: QueryConfig) => {
  const definition = chartDefinitions[chartType];
  const dimensionGroupCount = definition.fieldGroups.filter(
    (group) => group.kind === 'dimension'
  ).length;
  const metricGroupCount = definition.fieldGroups.filter((group) => group.kind === 'metric').length;

  return {
    dimensionGroups: queryConfig.dimensionGroups.slice(0, dimensionGroupCount),
    metricGroups: queryConfig.metricGroups.slice(0, metricGroupCount),
  };
};

/**
 * 把图表定义中的顺序索引映射为 kind 内部索引，避免维度组和指标组共用一套下标。
 * 调用场景：定义驱动渲染 QueryConfigRow，以及拖拽 drop zone 回写 queryConfig 时。
 * 主要逻辑：只统计当前定义中同 kind 且位于当前组之前的数量，得到 dimensionGroups/metricGroups 的真实下标。
 */
const getFieldGroupKindIndex = (
  chartType: ChartConfig['chartType'],
  definitionIndex: number,
  kind: 'dimension' | 'metric'
): number => {
  return (
    chartDefinitions[chartType].fieldGroups
      .slice(0, definitionIndex + 1)
      .filter((group) => group.kind === kind).length - 1
  );
};

/**
 * 在裁剪后的活动字段组里查找 sort 引用的 binding（Task 1-7/R-50）。
 * 调用场景：composeChartQueryRequest 构造 sort wire payload 前定位排序目标。
 * 主要逻辑：按 bindingId 精确匹配（bindingId 全局唯一），指标组优先、维度组其次；
 * 绑定已被移除（悬挂引用）时返回 undefined，调用方据此丢弃 sort。
 */
const findSortBinding = (
  activeGroups: { dimensionGroups: FieldGroup[]; metricGroups: FieldGroup[] },
  bindingId: string
): { binding: BindingInstance; kind: 'metric' | 'dimension' } | undefined => {
  for (const group of activeGroups.metricGroups) {
    const binding = group.bindings.find((b) => b.bindingId === bindingId);
    if (binding) {
      return { binding, kind: 'metric' };
    }
  }
  for (const group of activeGroups.dimensionGroups) {
    const binding = group.bindings.find((b) => b.bindingId === bindingId);
    if (binding) {
      return { binding, kind: 'dimension' };
    }
  }
  return undefined;
};

/**
 * 指标在 wire（SQL AS 子句 / 响应列名 / 排序键）上的输出列名 = 字段本身的列名。
 * 别名（显示名）是纯展示配置，永远不进 SQL——后端 safeIdentifier 会拒绝中文等
 * 标识符（退化成 _invalid_identifier 切断列数据），且查询本就该用原始字段表达式。
 * 展示名只留在前端的 labels / 列头配置里。
 */
const wireAliasOf = (_alias: string | undefined, columnName: string): string => columnName;

/**
 * 按表格输出列名（TableChart sorter.field）反查对应绑定的 bindingId。
 * 调用场景：handleSortChange 把用户点击的排序列翻译回 queryConfig.sort 的引用键。
 * 主要逻辑：输出键与后端行值键一致——指标 = metricAliases[bindingId] || 列名，
 * 维度 = 列名；同名冲突时指标优先（按值排序是主场景）；无对应绑定返回 undefined。
 */
const findBindingIdByOutputName = (
  queryConfig: QueryConfig,
  fields: ChartField[],
  metricAliases: Record<string, string>,
  outputName: string
): string | undefined => {
  const fieldMap = new Map(fields.map((f) => [f.id, f]));
  const outputNameOf = (
    binding: BindingInstance,
    kind: 'metric' | 'dimension'
  ): string | undefined => {
    const columnName = fieldMap.get(binding.fieldId)?.name;
    if (columnName === undefined) {
      return undefined;
    }
    return kind === 'metric'
      ? wireAliasOf(metricAliases[binding.bindingId], columnName)
      : columnName;
  };
  for (const group of queryConfig.metricGroups) {
    for (const binding of group.bindings) {
      if (outputNameOf(binding, 'metric') === outputName) {
        return binding.bindingId;
      }
    }
  }
  for (const group of queryConfig.dimensionGroups) {
    for (const binding of group.bindings) {
      if (outputNameOf(binding, 'dimension') === outputName) {
        return binding.bindingId;
      }
    }
  }
  return undefined;
};

/**
 * 按绑定 id 反查该绑定在表格输出里的列名（TableChart 的 sortField 输入）。
 * 调用场景：把 store 的 queryConfig.sort（bindingId 引用）翻译成表头受控排序状态。
 * 主要逻辑：与 findBindingIdByOutputName 同一套命名口径——指标 = metricAliases[bindingId]
 * || 列名，维度 = 列名；找不到绑定（已移除）时返回 undefined，表头即视为无排序。
 */
const findOutputNameByBindingId = (
  queryConfig: QueryConfig,
  fields: ChartField[],
  metricAliases: Record<string, string>,
  bindingId: string
): string | undefined => {
  const fieldMap = new Map(fields.map((f) => [f.id, f]));
  for (const group of queryConfig.metricGroups) {
    for (const binding of group.bindings) {
      if (binding.bindingId !== bindingId) continue;
      const columnName = fieldMap.get(binding.fieldId)?.name;
      return columnName === undefined
        ? undefined
        : wireAliasOf(metricAliases[binding.bindingId], columnName);
    }
  }
  for (const group of queryConfig.dimensionGroups) {
    for (const binding of group.bindings) {
      if (binding.bindingId !== bindingId) continue;
      return fieldMap.get(binding.fieldId)?.name;
    }
  }
  return undefined;
};

export interface ChartQueryRequestInput {
  datasetId: number;
  chartType: ChartConfig['chartType'];
  queryConfig: QueryConfig;
  fields: ChartField[];
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  tablePagination: { page: number; pageSize: number };
  /** 图表查询选项（持久化 camelCase 模型）；histogram 从中取 binCount 发 wire bin_count。 */
  queryOptions: ChartQueryOptions;
  /** 返回对象是否可携带 sort 键（仅当 queryConfig.sort 存在且 bindingId 在活动组内可解析时实际携带）。历史上两个自动查询 effect 传 false（从不发送 sort，R-50 bug）；Task 1-7 起三个调用点统一为 true。 */
  includeSort: boolean;
}

/**
 * combo 双轴组合图的两个指标槽位名（与 chartDefinitions.ts 的 fieldGroups[].id 对齐）。
 * 只要图型定义了其中之一，就必须走 v2 槽位协议——v1 平铺的 metrics[] 无法表达某个指标
 * 属于主轴还是次轴。scatter 虽然也有两个指标槽位（x_metric/y_metric），但它按位置消费
 * （metrics[0]/metrics[1]，v1 保序即可），不在此集合内，故继续走 v1、行为不变。
 */
const DUAL_AXIS_METRIC_SLOTS = new Set(['primary_values', 'secondary_values']);

/**
 * 必须走 v2 槽位协议的维度槽位名（与 chartDefinitions.ts 的 fieldGroups[].id 对齐）：
 * radar 的 indicators，以及 pivot 的 rows/columns。
 * 这些槽位存在语义（雷达轴、交叉表的行/列），v1 平铺的 dims[] 只是无序列表，
 * 后端 resolveRadarSlots / resolvePivotSlots 按 ast.GroupName 归槽，拿不到槽位名就只能
 * 退化成"行透传"的平表格——透视表会静默变成一个普通表格。因此只要图型定义了这些槽位，
 * 就必须发 v2。
 */
const SLOT_PROTOCOL_DIMENSION_SLOTS = new Set(['indicators', 'rows', 'columns']);

/**
 * 图表查询请求的唯一构造出口（纯函数，所有输入经参数传入）。
 * 调用场景：手动执行查询（含排序/翻页覆盖）与两个自动查询 effect 共用。
 * 主要逻辑：按图表定义裁剪字段组、字段 id 映射回列名、组装 filters/pagination；
 * 维度与指标同时为空时返回 null 表示不发起查询。
 *
 * v1/v2 判别（裁定B）：当当前图型定义了 combo 的双轴指标槽位（primary_values/secondary_values），
 * **或** 定义了具名维度槽位（radar 的 indicators、pivot 的 rows/columns）时，发出 v2 格式请求
 * （spec_version:2 + dimension_groups/metric_groups，携带真实槽位名与每个字段的 binding_id）——
 * 否则主/次轴与行/列的槽位区分信息会在 v1 平铺 dims[]/metrics[] 中丢失。
 * 其余情况（bar/line/area、table/pie/scatter/kpi/histogram/funnel/boxplot）
 * 继续发出 v1 平铺格式（dims/metrics）。
 *
 * 2026-09-19：bar/line/area 的 color_group 与 radar 的 series_group 槽位已下线，
 * 因此这两个图型不再有"槽位非空才发 v2"的分支，恒走 v1。
 */
export const composeChartQueryRequest = (
  input: ChartQueryRequestInput
): ChartQueryRequest | null => {
  const {
    datasetId,
    chartType,
    queryConfig,
    fields,
    metricAggregations,
    metricAliases,
    tablePagination,
    queryOptions,
    includeSort,
  } = input;

  const activeGroups = getActiveFieldGroups(chartType, queryConfig);
  const fieldMap = new Map(fields.map((f) => [f.id, f]));

  // 过滤条件展开：日期条件带上的是「意图」（最近 7 天 / 本月 …），在**下发请求这一刻**
  // 才解析成具体区间——这样动态日期是真的动态，页面挂着跨天也不会失真；其余条件照旧直传。
  // 一条日期意图可能展开成两条（区间 + `包含空日期` 的 IS NULL，用 OR 连接）。
  const filtersPayload = queryConfig.filters.flatMap((f) => {
    const field = fields.find((candidate) => candidate.id === f.fieldId);
    // wire 引用列的稳定 id；SQL 输出别名与响应负载键才用展示名
    const fieldRef = field?.id || f.fieldId;
    if (f.date && isDateFilterValue(f.date.value)) {
      return expandDateFilterIntent(f.date, fieldRef);
    }
    return [
      {
        field: fieldRef,
        operator: f.operator,
        value: f.value,
        value_end: f.valueEnd,
        logic: f.logic,
      },
    ];
  });
  const paginationPayload =
    chartType === 'table'
      ? {
          page: tablePagination.page,
          page_size: tablePagination.pageSize,
        }
      : undefined;

  const definition = chartDefinitions[chartType];
  const dimensionDefs = definition.fieldGroups.filter((group) => group.kind === 'dimension');
  const metricDefs = definition.fieldGroups.filter((group) => group.kind === 'metric');

  // v2 触发条件：图型带 combo 双轴指标槽位，**或** 带具名维度槽位（radar 的 indicators、
  // pivot 的 rows/columns）。后端按 ast.GroupName 解析槽位，v1 平铺请求会丢槽位区分。
  // 详见两个常量与 composeChartQueryRequest 的 doc。
  const requiresSlotProtocol =
    metricDefs.some((group) => DUAL_AXIS_METRIC_SLOTS.has(group.id)) ||
    dimensionDefs.some((group) => SLOT_PROTOCOL_DIMENSION_SLOTS.has(group.id));

  // sort wire payload（Task 1-7/R-50）：queryConfig.sort 以 bindingId 引用排序目标。
  // - v2 槽位协议：field 直接发送 bindingId（组内携带 binding_id，后端 resolveSortAlias
  //   按 AST 的 BindingID 查出输出别名后渲染 ORDER BY）；
  // - v1 平铺协议：请求不携带任何 binding_id，后端无从反查，field 发送该绑定的输出列名
  //   （指标 = metricAliases[bindingId] || 列名，维度 = 列名）——与本任务前直接发送
  //   TableChart sorter.field（列名/别名）的 wire 字节等价，v1 行为不变；
  // - bindingId 悬挂（绑定已移除）或对应列已不在数据集字段里：丢弃 sort，
  //   避免后端把无法解析的引用渲染成 _invalid_identifier 造成 SQL 报错；
  // - 无有效 sort 时不携带 sort 键（与 includeSort:false 时代的请求形状一致）。
  const sortWireField = (() => {
    if (!includeSort || !queryConfig.sort) {
      return undefined;
    }
    const sortBinding = findSortBinding(activeGroups, queryConfig.sort.bindingId);
    if (!sortBinding) {
      return undefined;
    }
    const columnName = fieldMap.get(sortBinding.binding.fieldId)?.name;
    if (columnName === undefined) {
      return undefined;
    }
    if (requiresSlotProtocol) {
      return sortBinding.binding.bindingId;
    }
    return sortBinding.kind === 'metric'
      ? wireAliasOf(metricAliases[sortBinding.binding.bindingId], columnName)
      : columnName;
  })();
  // funnel（漏斗图，R-59）强制降序（plan §3.3，验收行767，裁定 F）：sort 恒为 value
  // 槽位输出名的 desc，无条件覆盖 includeSort 与 queryConfig.sort（用户没有 funnel 的
  // sort UI，但持久化文档恢复可能带任意 sort）。funnel 走 v1 平铺路径，输出名与 v1
  // sortWireField 同口径：metricAliases[bindingId] || 列名。value 绑定缺失/列名查不到
  // （异常防御）时返回**空 payload**（不带 sort）——而非 undefined，否则下方 `??` 会回退到
  // 原始 sort 表达式、把恢复的用户 sort 泄露进请求，违反"funnel 用户不可覆盖"不变量。
  const funnelSortPayload = (() => {
    if (chartType !== 'funnel') {
      return undefined;
    }
    const valueBinding = activeGroups.metricGroups[0]?.bindings[0];
    const columnName = valueBinding ? fieldMap.get(valueBinding.fieldId)?.name : undefined;
    if (!valueBinding || columnName === undefined) {
      return {};
    }
    return {
      sort: {
        field: wireAliasOf(metricAliases[valueBinding.bindingId], columnName),
        order: 'desc' as const,
      },
    };
  })();
  const sortPayload =
    funnelSortPayload ??
    (sortWireField !== undefined && queryConfig.sort
      ? { sort: { field: sortWireField, order: queryConfig.sort.order } }
      : {});

  // histogram（R-57）专属 query_options：wire 用 snake_case bin_count，缺省 20（与后端
  // HistogramProcessor 默认一致）；持久化文档的 camelCase binCount → wire 的翻译只发生在
  // 这里。其余图型不带 query_options 键，请求形状与本任务改动前完全一致。
  const queryOptionsPayload =
    chartType === 'histogram' ? { query_options: { bin_count: queryOptions.binCount ?? 20 } } : {};

  if (requiresSlotProtocol) {
    // v2 槽位协议：dimension_groups/metric_groups 携带真实槽位名与 binding_id
    const dimensionGroupsPayload: ChartDimensionGroup[] = [];
    dimensionDefs.forEach((def, index) => {
      const group = activeGroups.dimensionGroups[index];
      if (!group) {
        return;
      }
      const groupFields: ChartDimensionField[] = group.bindings.flatMap((binding) => {
        const chartField = fieldMap.get(binding.fieldId);
        if (!chartField) {
          return [];
        }
        return [{ field: chartField.id, binding_id: binding.bindingId }];
      });
      dimensionGroupsPayload.push({ name: def.id, label: def.label, fields: groupFields });
    });

    const metricGroupsPayload: ChartMetricGroup[] = [];
    metricDefs.forEach((def, index) => {
      const group = activeGroups.metricGroups[index];
      if (!group) {
        return;
      }
      const groupFields: ChartMetricField[] = group.bindings.flatMap((binding) => {
        const chartField = fieldMap.get(binding.fieldId);
        if (!chartField) {
          return [];
        }
        return [
          {
            field: chartField.id,
            agg: (metricAggregations[binding.bindingId] || 'sum') as ChartQueryAggregation,
            alias: wireAliasOf(metricAliases[binding.bindingId], chartField.name),
            binding_id: binding.bindingId,
          },
        ];
      });
      metricGroupsPayload.push({ name: def.id, label: def.label, fields: groupFields });
    });

    const totalDimensionFields = dimensionGroupsPayload.reduce(
      (sum, group) => sum + group.fields.length,
      0
    );
    const totalMetricFields = metricGroupsPayload.reduce(
      (sum, group) => sum + group.fields.length,
      0
    );
    if (totalDimensionFields === 0 && totalMetricFields === 0) {
      return null;
    }

    return {
      dataset_id: datasetId,
      chart_type: chartType,
      spec_version: 2,
      dimension_groups: dimensionGroupsPayload,
      metric_groups: metricGroupsPayload,
      filters: filtersPayload,
      ...sortPayload,
      pagination: paginationPayload,
    };
  }

  // v1 平铺协议：逻辑与改动前完全一致
  const dimensionFields = activeGroups.dimensionGroups
    .flatMap((group) => group.bindings.map((b) => b.fieldId))
    .map((name) => fieldMap.get(name))
    .filter((f): f is ChartField => f !== undefined);

  // 已知限制：同一列被同一槽位拖入两次时，flatMap 会产生重复的 field（列 id），后端将
  // 收到两个相同的 metric 配置。本阶段按现有逻辑原样发送、不去重——真正区分依赖
  // v2 wire 的 bindingId 别名（v1 平铺协议不带 binding_id）。
  const metrics = activeGroups.metricGroups
    .flatMap((group) => group.bindings)
    .flatMap((binding) => {
      const chartField = fieldMap.get(binding.fieldId);
      if (!chartField) {
        return [];
      }
      return [
        {
          field: chartField.id,
          agg: (metricAggregations[binding.bindingId] || 'sum') as ChartQueryAggregation,
          alias: wireAliasOf(metricAliases[binding.bindingId], chartField.name),
        },
      ];
    });

  if (dimensionFields.length === 0 && metrics.length === 0) {
    return null;
  }

  return {
    dataset_id: datasetId,
    chart_type: chartType,
    dims: dimensionFields.map((f) => f.id),
    metrics,
    filters: filtersPayload,
    ...sortPayload,
    ...queryOptionsPayload,
    pagination: paginationPayload,
  };
};

/**
 * 从查询结果负载里尽力取出「行数」，供查询记录的结果元信息留痕。
 *
 * 各图型返回形状不同（裸数组 / {columns,data} 表格 / pivot v2 的 cells / 轴图表的
 * series / KPI 单值）。只有前两种的「行数」含义无歧义，其余一律返回 undefined：
 * 宁可不上报，也不上报一个会被误读成「这次查询只出了一行」的数字。
 */
const countResultRows = (payload: unknown): number | undefined => {
  if (Array.isArray(payload)) {
    return payload.length;
  }
  if (
    typeof payload === 'object' &&
    payload !== null &&
    'data' in payload &&
    Array.isArray(payload.data)
  ) {
    return payload.data.length;
  }
  return undefined;
};

interface ChartCanvasProps {
  config: ChartConfig;
  data: ChartDataResponse;
  loading: boolean;
  dimensionLabels: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  chartStyle: ChartStyleConfig;
}

/** 离屏主题探针样式：不影响布局，仅供 chartPalette 读取当前主题的 --dr-*。 */
const CHART_PROBE_STYLE: React.CSSProperties = {
  position: 'absolute',
  width: 0,
  height: 0,
  overflow: 'hidden',
  pointerEvents: 'none',
};

/** 图表渲染错误兜底：阻止 ECharts 抛错清空整棵 React 树（白屏丢工作）。 */
class ChartErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean }
> {
  state = { hasError: false };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return (
        <Empty
          description="图表渲染失败，请调整配置"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          style={{ padding: '100px 0' }}
        />
      );
    }
    return this.props.children;
  }
}

const ChartCanvas: React.FC<ChartCanvasProps> = ({
  config,
  data,
  loading,
  dimensionLabels,
  metricAliases,
  metricUnits,
  chartStyle,
}) => {
  // 主题宿主探针（与 ChartView 同一机制）：canvas 取不到 CSS 变量，须在构造 option 时
  // 把当前主题的颜色算成字面值。resolvedTheme 入依赖 → 切主题即重建 option 实时重绘。
  const resolvedTheme = useResolvedTheme();
  const hostRef = useRef<HTMLDivElement>(null);
  const [hostEl, setHostEl] = useState<HTMLDivElement | null>(null);
  useEffect(() => {
    setHostEl(hostRef.current);
  }, []);
  // option 构造走共享纯函数 buildChartOption（与 ShareView 同一出口）；
  // 「字段名 → 显示名」映射依赖 store 状态（queryConfig/chartBuilderFields），
  // 在组件体内计算为纯数据 labels 后传入。
  const chartOption = useMemo(() => {
    const { queryConfig, chartBuilderFields } = useStore.getState();
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));

    const dimensionBindings = queryConfig.dimensionGroups.flatMap((g) => g.bindings);
    const metricBindings = queryConfig.metricGroups.flatMap((g) => g.bindings);
    // context.dimensions/metrics 仍是列名：buildChartOption 用它们匹配结构化响应的
    // x_axis / series[].name（后端目前仍按列名/别名返回，未切到 bindingId）。
    const dimensions = dimensionBindings
      .map((b) => fieldMap.get(b.fieldId)?.name)
      .filter((name): name is string => name !== undefined);
    const metrics = metricBindings
      .map((b) => fieldMap.get(b.fieldId)?.name)
      .filter((name): name is string => name !== undefined);

    // labels 的键保持列名（buildChartOption 按列名/系列名查显示名），但 label/alias/unit
    // 的值按 bindingId 取（五个 Record 已改为 bindingId 键）。
    const labels: Record<string, string> = {};
    for (const binding of dimensionBindings) {
      const name = fieldMap.get(binding.fieldId)?.name;
      const label = dimensionLabels[binding.bindingId];
      if (name && label) {
        labels[name] = label;
      }
    }
    for (const binding of metricBindings) {
      const baseName = fieldMap.get(binding.fieldId)?.name || binding.fieldId;
      const alias = metricAliases[binding.bindingId];
      const unit = metricUnits[binding.bindingId];
      const displayName = alias || baseName;
      // 已知限制（Task 0-6+0-8 解决）：同一列名有多个 binding（多个不同 alias）时，
      // 共享的列名键上后写入者覆盖先写入者，图表暂时无法区分显示名。
      labels[baseName] = unit ? `${displayName} (${unit})` : displayName;
    }

    // combo 双轴：按图型定义的 metric 槽位（primary_values/secondary_values）派生 metricSlots。
    // 槽位名取自 chartDefinitions 的 fieldGroups（store 里 metricGroups[].id 是位置 id
    // 'metric-group-N'，并非槽位名），按 kind==='metric' 的顺序与 store 组按 index 对齐——
    // 与 composeChartQueryRequest 组装 v2 metric_groups 的约定一致。非 combo 图型为 undefined。
    // metrics 用 alias 优先（列名兜底），与上方请求 alias 表达式（metricAliases[bindingId]
    // || chartField.name）完全一致——后端 series 名按 ResolveAlias()（alias 优先，列名兜底）
    // 生成，若这里只填列名，带别名的 series 会反查不到槽位而被静默分配到主轴。
    const metricSlots =
      config.chartType === 'combo'
        ? chartDefinitions[config.chartType].fieldGroups
            .filter((group) => group.kind === 'metric')
            .map((def, index) => ({
              slot: def.id,
              metrics: (queryConfig.metricGroups[index]?.bindings ?? [])
                .map((b) =>
                  wireAliasOf(metricAliases[b.bindingId], fieldMap.get(b.fieldId)?.name ?? '')
                )
                .filter((name): name is string => name !== undefined),
            }))
        : undefined;

    return buildChartOption(
      config.chartType,
      data,
      chartStyle,
      labels,
      {
        title: config.title,
        dimensions,
        metrics,
        metricSlots,
      },
      resolvedTheme,
      hostEl
    );
  }, [
    chartStyle,
    config,
    data,
    dimensionLabels,
    metricAliases,
    metricUnits,
    hostEl,
    resolvedTheme,
  ]);

  if (loading) {
    return (
      <div
        style={{
          height: '100%',
          minHeight: 220,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 12,
        }}
      >
        <Spin size="large" />
        <Text style={{ fontSize: 12, color: 'var(--dr-text-3)' }}>加载图表数据中...</Text>
      </div>
    );
  }

  if (!chartOption) {
    return (
      <div
        style={{
          height: '100%',
          minHeight: 220,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 12,
        }}
      >
        {/* 空态做成"待生成的画布"而不是一张白纸：下沉面 + 虚线框，让这块区域即使
            什么都没画，也看得出"这里是一块画布"，而不是"没加载出来"。
            （改动前是一个巨大的空 Empty，整页最大的一片白就在这里。） */}
        <div
          style={{
            width: '100%',
            maxWidth: 420,
            minHeight: 150,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 6,
            padding: '20px 16px',
            borderRadius: 8,
            border: '1px dashed var(--dr-border-strong)',
            background: 'var(--dr-sunken)',
          }}
        >
          <BarChartOutlined style={{ fontSize: 20, color: 'var(--dr-text-4)' }} />
          <Text style={{ fontSize: 13, color: 'var(--dr-text-2)' }}>
            请配置维度和指标以生成图表
          </Text>
          <Text style={{ fontSize: 12, color: 'var(--dr-text-3)' }}>
            从左侧字段列表拖入，或点槽位内的 + 添加
          </Text>
        </div>
      </div>
    );
  }

  return (
    <>
      <div ref={hostRef} className="dr-chart-host" aria-hidden style={CHART_PROBE_STYLE} />
      <ReactECharts
        option={chartOption}
        style={{ height: '100%', width: '100%' }}
        opts={{ renderer: 'canvas' }}
      />
    </>
  );
};

interface FieldListPanelProps {
  fields: ChartField[];
  loading: boolean;
}

/**
 * 字段列表的分组标题：3px 语义色条 + 12px/600 标签。
 * 调用场景：左栏「可用字段」的维度 / 指标两组。
 * 主要逻辑：色条与中栏槽位标签共用同一套语义色（维度=蓝 / 指标=绿），
 * 「维度和指标各是什么颜色」只需学一次，就能在左栏、中栏、字段标签三处复用。
 * 约束：标签与计数必须留在**同一个元素**里（`维度 (13)`）——拆成两个兄弟元素后
 * 「维度」会独立成一个文本节点，与中栏的槽位标签文案撞车（测试按文本精确定位）。
 */
const FieldGroupHeader: React.FC<{ label: string; count: number; color: string }> = ({
  label,
  count,
  color,
}) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
    <span
      style={{
        width: 3,
        height: 11,
        borderRadius: 2,
        background: color,
        flexShrink: 0,
      }}
    />
    <Text strong style={{ fontSize: 12, color: 'var(--dr-text-2)' }}>
      {label} ({count})
    </Text>
  </div>
);

const FieldListPanel: React.FC<FieldListPanelProps> = ({ fields, loading }) => {
  const dimensions = fields.filter((f) => f.type === 'dimension');
  const metrics = fields.filter((f) => f.type === 'metric');

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '40px 0' }}>
        <Spin />
        <div style={{ marginTop: 8 }}>
          <Text type="secondary">加载字段中...</Text>
        </div>
      </div>
    );
  }

  if (fields.length === 0) {
    return <Empty description="请先选择数据集" image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  }

  return (
    <div>
      {/* 原为 Divider 分隔两组：带色条的分组标题本身就是更强的分隔符，
          去掉 Divider 省下 ~17px 纵向空白（本页约定：留白只服务于"看得清"）。 */}
      <div style={{ marginBottom: 10 }}>
        <FieldGroupHeader label="维度" count={dimensions.length} color="var(--dr-dim)" />
        {dimensions.map((field) => (
          <DraggableField key={field.id} field={field} />
        ))}
      </div>

      <div>
        <FieldGroupHeader label="指标" count={metrics.length} color="var(--dr-metric)" />
        {metrics.map((field) => (
          <DraggableField key={field.id} field={field} />
        ))}
      </div>
    </div>
  );
};

interface ConfigPanelProps {
  config: ChartConfig;
  onConfigChange: (config: Partial<ChartConfig>) => void;
  chartStyle: ChartStyleConfig;
  onChartStyleChange: (style: Partial<ChartStyleConfig>) => void;
  /** 图表查询选项（histogram 的 binCount 在此读写）。 */
  queryOptions: ChartQueryOptions;
  onQueryOptionsChange: (options: Partial<ChartQueryOptions>) => void;
}

const chartTypeOptions = Object.values(chartDefinitions).map((def) => ({
  type: def.type,
  icon: def.icon,
  label: def.label,
}));

const chartTypeOptionMap = new Map(chartTypeOptions.map((opt) => [opt.type, opt]));

/**
 * 图型按使用目的分组：每行一个分析意图，行内小图标按钮互为同类替代
 * （如趋势行内柱/折/面积/组合之间切换），用户先定位行再挑图型。
 */
const chartTypeGroups: {
  label: string;
  types: Array<(typeof chartTypeOptions)[number]['type']>;
}[] = [
  { label: '表格', types: ['table', 'pivot'] },
  { label: '趋势', types: ['bar', 'line', 'area', 'combo'] },
  { label: '分布', types: ['pie', 'histogram', 'boxplot'] },
  { label: '其他', types: ['scatter', 'radar', 'kpi', 'funnel'] },
];

/**
 * 配置面板里的单行设置：左侧标签、右侧控件。
 * 调用场景：图表配置面板的样式/选项区——标签与控件各占固定位置，一行一项，
 * 避免控件与标签在同一行里挤成一团、看不出哪一项是什么。
 */
const SettingRow: React.FC<{ label: string; children: React.ReactNode }> = ({
  label,
  children,
}) => (
  <div
    data-testid="config-setting-row"
    style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      gap: 12,
      minHeight: 32,
      padding: '2px 0',
    }}
  >
    <span style={{ fontSize: 13, color: 'var(--dr-text-2)', flex: '0 0 auto' }}>{label}</span>
    <div style={{ flex: '1 1 auto', display: 'flex', justifyContent: 'flex-end' }}>{children}</div>
  </div>
);

const ConfigPanel: React.FC<ConfigPanelProps> = ({
  config,
  onConfigChange,
  chartStyle,
  onChartStyleChange,
  queryOptions,
  onQueryOptionsChange,
}) => {
  // styleKeys 决定当前图型显示哪些样式控件（Task 0-4 声明、本任务首次真正接线）。
  // 7 种图型现在都应显式声明 styleKeys（见 chartDefinitions.ts），undefined 理论上
  // 不应再出现；防御性地按"不显示任何样式控件"处理，避免误渲染出该图型不消费的开关。
  const styleKeys = chartDefinitions[config.chartType].styleKeys;
  const showStyleControl = (key: keyof ChartStyleConfig) => styleKeys?.includes(key) ?? false;

  // 面板最上方只放图表标题：标题是最常改、也最需要一眼看到的信息，
  // 不应和样式开关混在一起滚到面板底部。
  const hasStyleSection =
    showStyleControl('smooth') ||
    showStyleControl('colors') ||
    showStyleControl('stack') ||
    showStyleControl('orientation') ||
    showStyleControl('donut') ||
    showStyleControl('tableRowSize') ||
    config.chartType === 'histogram';

  return (
    <div>
      <Card
        title="图表标题"
        size="small"
        style={{ marginBottom: 6 }}
        styles={{ body: { padding: '4px 6px' } }}
      >
        <Input
          value={config.title}
          onChange={(e) => onConfigChange({ title: e.target.value })}
          placeholder="输入图表标题"
        />
      </Card>

      <Card
        title="可视化类型"
        size="small"
        style={{ marginBottom: 6 }}
        styles={{ body: { padding: '4px 6px' } }}
      >
        {/* 每行一个分析意图，行内图标按钮互为同类替代（如趋势行内柱/折/面积/组合之间切换）。
            整行包一层下沉面 + 圆角，读起来是"一组可切换的分段控件"，而不是散落一地的图标网格
            ——散落的图标需要用户逐个去猜它到底是不是按钮。 */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {chartTypeGroups.map((group) => (
            <div
              key={group.label}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                padding: '2px 4px 2px 6px',
                borderRadius: 6,
                background: 'var(--dr-sunken)',
              }}
            >
              <span
                style={{
                  fontSize: 12,
                  color: 'var(--dr-text-3)',
                  flex: '0 0 34px',
                }}
              >
                {group.label}
              </span>
              {group.types.map((type) => {
                const opt = chartTypeOptionMap.get(type);
                if (!opt) {
                  return null;
                }
                const selected = config.chartType === opt.type;
                return (
                  <Tooltip
                    key={opt.type}
                    title={opt.label}
                    placement="top"
                    mouseEnterDelay={0}
                    mouseLeaveDelay={0}
                  >
                    <Button
                      type={selected ? 'primary' : 'text'}
                      icon={React.createElement(opt.icon)}
                      // 图型按钮只有图标，文字挂在 Tooltip 上不进 accessible name：
                      // 补 aria-label 让读屏与测试都能按名称定位（否则按 role+name 查不到）。
                      aria-label={opt.label}
                      onClick={() => onConfigChange({ chartType: opt.type })}
                      style={{ width: 32, height: 28, padding: 0 }}
                    />
                  </Tooltip>
                );
              })}
            </div>
          ))}
        </div>
      </Card>

      {hasStyleSection && (
        <Card
          title="图表配置"
          size="small"
          style={{ marginBottom: 6 }}
          styles={{ body: { padding: '4px 6px' } }}
        >
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {showStyleControl('smooth') && (
              <SettingRow label="平滑曲线">
                <Switch
                  checked={chartStyle.smooth}
                  onChange={(checked) => onChartStyleChange({ smooth: checked })}
                />
              </SettingRow>
            )}

            {showStyleControl('colors') && (
              <SettingRow label="主色">
                <ColorPicker
                  value={chartStyle.colors[0] || '#1677ff'}
                  onChange={(color) => onChartStyleChange({ colors: [color.toHexString()] })}
                />
              </SettingRow>
            )}

            {config.chartType === 'histogram' && (
              <SettingRow label="分箱数量">
                <InputNumber
                  style={{ width: 120 }}
                  min={1}
                  max={1000}
                  precision={0}
                  value={queryOptions.binCount ?? 20}
                  onChange={(value) => onQueryOptionsChange({ binCount: value ?? undefined })}
                />
              </SettingRow>
            )}

            {showStyleControl('stack') && (
              <SettingRow label="堆叠模式">
                <Select
                  style={{ width: 160 }}
                  value={chartStyle.stack ?? 'none'}
                  onChange={(value) => onChartStyleChange({ stack: value })}
                  options={[
                    { value: 'none', label: '不堆叠' },
                    { value: 'normal', label: '堆叠' },
                    { value: 'percent', label: '百分比堆叠' },
                  ]}
                />
              </SettingRow>
            )}

            {showStyleControl('orientation') && (
              <SettingRow label="方向">
                <Select
                  style={{ width: 160 }}
                  value={chartStyle.orientation ?? 'vertical'}
                  onChange={(value) => onChartStyleChange({ orientation: value })}
                  options={[
                    { value: 'vertical', label: '纵向' },
                    { value: 'horizontal', label: '横向' },
                  ]}
                />
              </SettingRow>
            )}

            {showStyleControl('donut') && (
              <SettingRow label="环形图">
                <Switch
                  checked={chartStyle.donut ?? false}
                  onChange={(checked) => onChartStyleChange({ donut: checked })}
                />
              </SettingRow>
            )}

            {showStyleControl('tableRowSize') && (
              <SettingRow label="表格行尺寸">
                <Select
                  style={{ width: 160 }}
                  value={chartStyle.tableRowSize}
                  onChange={(value) => onChartStyleChange({ tableRowSize: value })}
                  options={[
                    { value: 'small', label: '紧凑' },
                    { value: 'middle', label: '默认' },
                    { value: 'large', label: '宽松' },
                  ]}
                />
              </SettingRow>
            )}
          </div>
        </Card>
      )}
    </div>
  );
};

/**
 * 「预览」卡头右侧的状态指示：一个语义色圆点 + 当前图型名。
 * 调用场景：预览卡片的 extra。
 * 主要逻辑：把"有没有数据 / 正在查"从卡体提到卡头 —— 卡体里是图表本身，此前这三种
 * 状态只能靠读那片空白区域猜。颜色沿用拖放区的语义色：蓝=查询中，绿=有数据，灰=空。
 */
const QueryStatusBadge: React.FC<{
  loading: boolean;
  hasData: boolean;
  chartTypeLabel: string;
}> = ({ loading, hasData, chartTypeLabel }) => {
  const dotColor = loading ? 'var(--dr-accent)' : hasData ? 'var(--dr-metric)' : 'var(--dr-text-4)';
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
      <span
        aria-hidden
        style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          flexShrink: 0,
          backgroundColor: dotColor,
        }}
      />
      <span>{chartTypeLabel}</span>
    </span>
  );
};

/**
 * 图表构建页负责组装字段拖拽、查询配置和图表渲染三块交互。
 * 调用场景：`/chart-builder` 页面。
 * 主要逻辑：同步 store 状态、处理字段拖放、并在拖拽期间渲染 overlay 预览。
 */
const ChartBuilder: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedDatasetId, setSelectedDatasetId] = useState<number | null>(null);
  const [editingChartId, setEditingChartId] = useState<number | null>(null);
  const [sqlModalVisible, setSqlModalVisible] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const [leftDrawerOpen, setLeftDrawerOpen] = useState(false);
  const [rightDrawerOpen, setRightDrawerOpen] = useState(false);
  const [activeDragField, setActiveDragField] = useState<ChartField | null>(null);
  const [activeDragBinding, setActiveDragBinding] = useState<DragPreview | null>(null);
  // 过滤配置弹窗：id=正在编辑的条件，isNew=本次拖入/下拉新建（取消时回收）。
  // 弹窗打开期间自动查询被挂起——输入中途态不进请求，确定才写回 store。
  const [filterEditing, setFilterEditing] = useState<{ id: string; isNew: boolean } | null>(null);
  // 日期筛选弹窗：日期字段走这条独立的编辑器（五种模式 + 粒度 + 快捷选项）。
  const [dateFilterEditing, setDateFilterEditing] = useState<{ id: string; isNew: boolean } | null>(
    null
  );
  // 字段属性弹窗：芯片扳手按钮打开，收拢原右下角维度/指标属性面板的配置。
  const [fieldSettings, setFieldSettings] = useState<{
    kind: 'dimension' | 'metric';
    bound: BoundField;
  } | null>(null);
  // 左侧字段栏宽度（可拖拽调节）
  const [leftSiderWidth, setLeftSiderWidth] = useState(150);
  const [resizeHandleHover, setResizeHandleHover] = useState(false);
  // 编辑态图表详情缓存（id + 请求 Promise），供配置加载 effect 重跑时复用
  const editChartCache = useRef<{ id: number; promise: Promise<Chart> } | null>(null);
  // 当前地址栏短码（= 复制按钮复制的那条链接的 id）
  const [shareShortId, setShareShortId] = useState<string | null>(null);
  // 直链还原的降级提示（记录过期 / 短码无效 / 配置版本不识别）
  const [queryRecordNotice, setQueryRecordNotice] = useState<{
    kind: 'warning' | 'error';
    text: string;
  } | null>(null);
  /**
   * 已「结清」的地址栏短码：本次会话自己落库签发的、以及已经还原过一遍的，都记在
   * 这里。没有它会出现死循环——还原 → 自动查询 → 落库 → 地址栏换成新短码 →
   * 还原 effect 再触发 → ……
   *
   * ⚠️ 记账必须发生在**还原成功之后**。提前到请求之前会让 dev 环境（`main.tsx` 的
   * StrictMode 会 mount → cleanup → 再 mount）的第二次挂载被自己的标记挡在门外，
   * 而第一次的响应又被 cleanup 的 cancelled 丢弃 —— 表现为「打开分享链接毫无反应」。
   */
  const settledShortIdRef = useRef<string | null>(null);
  /** 短码 → 在途还原请求：同一短码的并发/重复执行共用一个 GET（hit_count 是真实指标）。 */
  const inflightRestoreRef = useRef<{ shortId: string; promise: Promise<QueryRecord> } | null>(
    null
  );
  /** 上次成功落库的 spec 快照（JSON 串）：同配置不重复提交，翻页/重查不抖动地址栏。 */
  const lastPersistedSpecRef = useRef<string | null>(null);

  /** 左侧栏右缘拖拽调节宽度：按住手柄水平拖动，宽度限制在 [120, 320]。 */
  const startLeftSiderResize = (e: React.MouseEvent) => {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = leftSiderWidth;
    const onMove = (ev: MouseEvent) => {
      setLeftSiderWidth(Math.min(320, Math.max(120, startWidth + ev.clientX - startX)));
    };
    const onUp = () => {
      document.body.style.cursor = '';
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    document.body.style.cursor = 'col-resize';
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  };

  const {
    datasets,
    fetchDatasets,
    chartBuilderFields,
    chartBuilderFieldsLoading,
    chartBuilderConfig,
    chartData,
    chartDataLoading,
    setChartBuilderConfig,
    fetchDatasetFields,
    resetChartBuilder,
    addChart,
    updateChart,
    queryConfig,
    setQueryConfig,
    addFilter,
    removeFilter,
    updateFilter,
    addDimensionField,
    removeDimensionField,
    reorderDimensionField,
    addMetricField,
    removeMetricField,
    reorderMetricField,
    moveBinding,
    swapDimensionGroups,
    setMetricAggregation,
    setMetricAlias,
    setMetricAggregations,
    setMetricAliases,
    setDimensionLabel,
    setDimensionLabels,
    setMetricUnit,
    setMetricUnits,
    setMetricFormat,
    setMetricFormats,
    setChartStyle,
    setChartStyleState,
    setChartQueryOptionsState,
    autoQuery,
    toggleAutoQuery,
    dimensionLabels,
    metricAggregations,
    metricAliases,
    metricUnits,
    metricFormats,
    chartStyle,
    chartQueryOptions,
    executeChartQuery,
    tablePagination,
    tableColumns,
    setTablePagination,
    chartQueryResponse,
  } = useStore();

  /** 桌面端左栏顶部的数据集标识取对象而非 id，免得模板里重复 find。 */
  const currentDataset = useMemo(
    () => datasets.find((ds) => ds.id === selectedDatasetId) ?? null,
    [datasets, selectedDatasetId]
  );

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 5,
      },
    })
  );

  /**
   * 记录当前正在拖拽的对象，用于渲染跟随鼠标移动的 overlay 预览。
   * 调用场景：从左侧字段列表拖起字段，或从查询配置区拖起已有字段标签。
   * 主要逻辑：两种拖拽源都产出同一份 {label, color} 预览；无法识别时清空。
   */
  const handleDragStart = useCallback((event: DragStartEvent) => {
    const data = event.active.data.current;
    const field = getDraggedField(data);
    if (field) {
      setActiveDragField(field);
      setActiveDragBinding(null);
      return;
    }
    const binding = isDragBindingData(data) ? { label: data.label, color: data.color } : null;
    setActiveDragField(null);
    setActiveDragBinding(binding);
  }, []);

  /**
   * 在取消拖拽时清理 overlay 预览状态。
   * 调用场景：用户松手但未命中 drop zone，或拖拽流程被中断。
   * 主要逻辑：把当前拖拽对象置空，移除浮层预览。
   */
  const handleDragCancel = useCallback(() => {
    setActiveDragField(null);
    setActiveDragBinding(null);
  }, []);

  /**
   * 处理拖拽完成后的查询配置更新。
   * 调用场景：拖拽结束并命中某个落点。
   * 主要逻辑：分两条互斥路径——
   *   1) 配置区字段标签（binding-source）：落到别的组即换组、落到同组某个标签前即换序，
   *      统一交给 store.moveBinding 原子完成；跨 kind 或目标组已有同名列时给出提示。
   *   2) 左侧字段（field）：按落点类型新增到维度/指标/过滤字段组（原有行为）。
   */
  // ===== 过滤配置弹窗 =====
  const editingFilterCondition = filterEditing
    ? queryConfig.filters.find((f) => f.id === filterEditing.id)
    : undefined;
  // 按列 ID 反查字段：fieldId 存的是列的稳定标识（DatasetColumn.id），不是可变列名。
  const editingFilterField = editingFilterCondition
    ? (chartBuilderFields.find((f) => f.id === editingFilterCondition.fieldId) ?? null)
    : null;
  const editingDateFilterCondition = dateFilterEditing
    ? queryConfig.filters.find((f) => f.id === dateFilterEditing.id)
    : undefined;
  const editingDateFilterField = editingDateFilterCondition
    ? (chartBuilderFields.find((f) => f.id === editingDateFilterCondition.fieldId) ?? null)
    : null;

  /**
   * 拖入 / 下拉选择字段 → 新建一条空条件并打开弹窗；取消时由 isNew 回收。
   * 日期字段进日期筛选弹窗（动态日期/快捷选项/粒度），其余字段进通用筛选弹窗。
   */
  const openFilterEditor = useCallback(
    (field: ChartField) => {
      addFilter({ fieldId: field.id });
      const filters = useStore.getState().queryConfig.filters;
      const last = filters[filters.length - 1];
      if (!last) return;
      if (classifyFieldKind(field.dataType) === 'date') {
        setDateFilterEditing({ id: last.id, isNew: true });
        return;
      }
      setFilterEditing({ id: last.id, isNew: true });
    },
    [addFilter]
  );

  /** 点击已有条件：日期条件（无论新旧）都进日期筛选弹窗，其余进通用弹窗。 */
  const openExistingFilterEditor = useCallback(
    (filter: FilterCondition, field: ChartField | undefined) => {
      const isDateField = field !== undefined && classifyFieldKind(field.dataType) === 'date';
      if (filter.date || isDateField) {
        setDateFilterEditing({ id: filter.id, isNew: false });
        return;
      }
      setFilterEditing({ id: filter.id, isNew: false });
    },
    []
  );

  const handleFilterModalOk = useCallback(
    (patch: FilterConfigPatch) => {
      if (!filterEditing) return;
      updateFilter(filterEditing.id, patch);
      setFilterEditing(null);
    },
    [filterEditing, updateFilter]
  );

  const handleFilterModalCancel = useCallback(() => {
    if (filterEditing?.isNew) {
      removeFilter(filterEditing.id);
    }
    setFilterEditing(null);
  }, [filterEditing, removeFilter]);

  /**
   * 把日期意图的**当前区间**同步进 operator/value：这是给「不认识日期的下游」的兜底快照
   * （后端读配置的路径、以及保存时的物化出口）。页面查询本身走 filtersPayload 的动态解析，
   * 不依赖这两个字段。
   */
  const dateIntentSnapshotPatch = useCallback(
    (intent: DateFilterIntent): Partial<FilterCondition> => {
      // 主条件恒为第一条；「包含空日期」追加的 IS NULL 排在其后，不该被当成主条件。
      const primary = expandDateFilterIntent(intent, '')[0];
      return {
        date: intent,
        operator: (primary?.operator ?? 'eq') as FilterCondition['operator'],
        value: primary?.value ?? '',
        valueEnd: primary?.value_end,
        // 「作为筛选器」统一落条件顶层（date.asFilter/label 保留作旧数据兼容）。
        asFilter: intent.asFilter,
        filterLabel: intent.label,
      };
    },
    []
  );

  /**
   * 日期筛选弹窗确定：只写「意图」（`最近 7 天` 这类），区间留到下发请求时再解析，
   * 页面挂着过夜也不会变成过期快照。同时把此刻的区间写进 operator/value 作为兜底，
   * 供不认识日期的下游使用（保存时还会重物化一次，见 buildChartConfigDocument）。
   */
  const handleDateFilterModalOk = useCallback(
    (payload: DateFilterModalPayload) => {
      if (!dateFilterEditing) return;
      updateFilter(
        dateFilterEditing.id,
        dateIntentSnapshotPatch({
          value: payload.value,
          granularity: payload.granularity,
          weekStart: payload.weekStart,
          asFilter: payload.asFilter,
          label: payload.filterLabel,
          withTime: editingDateFilterField
            ? normalizeDataType(editingDateFilterField.dataType) === 'datetime'
            : false,
        })
      );
      setDateFilterEditing(null);
    },
    [dateFilterEditing, dateIntentSnapshotPatch, editingDateFilterField, updateFilter]
  );

  const handleDateFilterModalCancel = useCallback(() => {
    if (dateFilterEditing?.isNew) {
      removeFilter(dateFilterEditing.id);
    }
    setDateFilterEditing(null);
  }, [dateFilterEditing, removeFilter]);

  /** 删除条件时若其编辑弹窗正开着，一并关闭，避免残留空弹窗。 */
  const handleFilterRemove = useCallback(
    (id: string) => {
      removeFilter(id);
      if (filterEditing?.id === id) {
        setFilterEditing(null);
      }
      if (dateFilterEditing?.id === id) {
        setDateFilterEditing(null);
      }
    },
    [dateFilterEditing, filterEditing, removeFilter]
  );

  /**
   * 「作为筛选器」的条件在预览区上方出一枚行内控件（对齐火山引擎智能洞察）。
   * 三族通用：日期族用 DateFilterControl，字符串/数值族用 FilterValueControl。
   * 浮层里改条件即时生效、不设确认。旧图表的勾选存在 date.asFilter，渲染时兜底读取。
   */
  const renderDateFilterBar = () => {
    const controls = queryConfig.filters.filter(
      (filter) => filter.asFilter === true || filter.date?.asFilter === true
    );
    if (controls.length === 0) return null;
    return (
      <div
        style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}
        data-testid="chart-date-filter-bar"
      >
        {controls.map((filter) => {
          const date = filter.date;
          const field = chartBuilderFields.find((candidate) => candidate.id === filter.fieldId);
          // 日期意图仍是权威的日期条件（含只写了 date.asFilter 的旧数据）走日期控件。
          if (date?.asFilter === true) {
            return (
              <DateFilterControl
                key={filter.id}
                label={filter.filterLabel || date.label || field?.name || filter.fieldId}
                value={date.value}
                settings={{
                  granularity: date.granularity,
                  weekStart: date.weekStart,
                  withTime: date.withTime,
                }}
                withTime={date.withTime}
                allowModeSwitch
                allowClear
                onChange={(next: DateFilterValue) =>
                  updateFilter(filter.id, dateIntentSnapshotPatch({ ...date, value: next }))
                }
                onClear={() =>
                  updateFilter(
                    filter.id,
                    dateIntentSnapshotPatch({ ...date, value: { kind: 'dynamic' } })
                  )
                }
              />
            );
          }
          if (!field || date) return null;
          const kind = classifyFieldKind(field.dataType);
          if (kind === 'date') return null; // 日期意图缺失的日期条件没有行内编辑语义
          return (
            <FilterValueControl
              key={filter.id}
              field={field}
              filter={filter}
              datasetId={selectedDatasetId}
              onChange={(patch) => updateFilter(filter.id, patch)}
            />
          );
        })}
      </div>
    );
  };

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActiveDragField(null);
      setActiveDragBinding(null);

      const { active, over } = event;

      if (!over) return;

      const overData = over.data.current;

      const draggedBinding = isDragBindingData(active.data.current) ? active.data.current : null;
      if (draggedBinding) {
        const source = {
          kind: draggedBinding.kind,
          groupIndex: draggedBinding.groupIndex,
          bindingId: draggedBinding.bindingId,
        };

        if (isBindingSlotData(overData)) {
          if (
            moveBinding(source, {
              kind: overData.kind,
              groupIndex: overData.groupIndex,
              index: overData.index,
            }) === 'rejected'
          ) {
            message.warning('目标区域已存在同名字段');
          }
          return;
        }

        if (isZoneDropData(overData)) {
          if (overData.type === 'filter') {
            message.warning('筛选区不接受从查询配置拖入的字段');
            return;
          }
          if (overData.type !== draggedBinding.kind) {
            message.warning(
              draggedBinding.kind === 'dimension' ? '维度只能拖到维度区域' : '指标只能拖到指标区域'
            );
            return;
          }
          if (
            moveBinding(source, {
              kind: overData.type,
              groupIndex: typeof overData.groupIndex === 'number' ? overData.groupIndex : 0,
            }) === 'rejected'
          ) {
            message.warning('目标区域已存在同名字段');
          }
        }
        return;
      }

      const field = getDraggedField(active.data.current);
      if (!field) return;

      if (!isZoneDropData(overData)) return;
      const dropZoneType = overData.type;
      const groupIndex = typeof overData.groupIndex === 'number' ? overData.groupIndex : 0;

      if (dropZoneType === 'dimension') {
        if (field.type === 'dimension') {
          addDimensionField(field, groupIndex);
        } else {
          message.warning('请将指标拖入指标区域');
        }
      } else if (dropZoneType === 'metric') {
        if (field.type === 'metric') {
          addMetricField(field, groupIndex);
        } else {
          message.warning('请将维度拖入维度区域');
        }
      } else if (dropZoneType === 'filter') {
        // 过滤条件按**列的稳定 id** 记录（与 `binding.fieldId` 同口径）。同一字段可重复加入——
        // 区间筛选就是同字段各拖一次 >= 与 <=，因此这里不做去重。
        // 拖入即弹过滤配置弹窗：条件在弹窗里完成输入，确定才生效。
        openFilterEditor(field);
      }
    },
    [addDimensionField, addMetricField, openFilterEditor, moveBinding]
  );

  // 当前图型激活的字段组（getActiveFieldGroups 裁剪）：切图型后 queryConfig 可能残留
  // 隐藏组的绑定（如柱图 → KPI 卡后 X 轴维度组仍在——目标图型无维度槽位时刻意不裁剪 state），属性面板与保存配置必须
  // 与查询请求同口径，否则隐藏绑定会隐形泄漏——表现为重复维度列/多余属性条目。
  const activeQueryConfig = useMemo(
    () => getActiveFieldGroups(chartBuilderConfig.chartType, queryConfig),
    [chartBuilderConfig.chartType, queryConfig]
  );

  /**
   * 把一份 v2 图表配置文档灌进 store。`?edit=`（图表库）与 `?q=`（查询记录直链）
   * 两条还原路径共用同一个出口——两份实现迟早会漏掉 fieldMeta 或 style 其中之一。
   *
   * titleFallback 只在文档自身没有标题时兜底：编辑态用图表名，分享态用文档标题本身。
   */
  const applyConfigDocument = useCallback(
    (doc: ChartConfigDocument, titleFallback: string) => {
      setChartBuilderConfig({
        chartType: doc.chartType,
        title: doc.title || titleFallback,
        xAxisField: null,
        yAxisFields: [],
      });

      setQueryConfig(
        normalizeQueryConfigForChartType(doc.chartType, {
          dimensionGroups: doc.query.dimensionGroups,
          metricGroups: doc.query.metricGroups,
          filters: doc.query.filters as FilterCondition[],
          sort: doc.query.sort
            ? {
                bindingId: doc.query.sort.bindingId,
                order: doc.query.sort.order === 'desc' ? 'desc' : 'asc',
              }
            : undefined,
          limit: doc.query.limit,
        })
      );

      // v2 fieldMeta（键为 bindingId）→ 5 个运行时 Record
      const restoredLabels: Record<string, string> = {};
      const restoredAggregations: Record<string, string> = {};
      const restoredAliases: Record<string, string> = {};
      const restoredUnits: Record<string, string> = {};
      const restoredFormats: Record<string, string> = {};
      for (const [bindingId, meta] of Object.entries(doc.fieldMeta)) {
        if (meta.label) restoredLabels[bindingId] = meta.label;
        if (meta.aggregation) restoredAggregations[bindingId] = meta.aggregation;
        if (meta.alias) restoredAliases[bindingId] = meta.alias;
        if (meta.unit) restoredUnits[bindingId] = meta.unit;
        if (meta.format) restoredFormats[bindingId] = meta.format;
      }
      setDimensionLabels(restoredLabels);
      setMetricAggregations(restoredAggregations);
      setMetricAliases(restoredAliases);
      setMetricUnits(restoredUnits);
      setMetricFormats(restoredFormats);

      // 仅在配置携带内容时覆盖，避免空文档抹掉默认样式
      const restoredStyle = doc.style as ChartStyleConfig;
      if (restoredStyle && Object.keys(restoredStyle).length > 0) {
        setChartStyleState(restoredStyle);
      }
      const restoredQueryOptions = doc.queryOptions as ChartQueryOptions;
      if (restoredQueryOptions && Object.keys(restoredQueryOptions).length > 0) {
        setChartQueryOptionsState(restoredQueryOptions);
      }
    },
    [
      setChartBuilderConfig,
      setQueryConfig,
      setDimensionLabels,
      setMetricAggregations,
      setMetricAliases,
      setMetricUnits,
      setMetricFormats,
      setChartStyleState,
      setChartQueryOptionsState,
    ]
  );

  const getDimensionFields = useCallback((): BoundField[] => {
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return activeQueryConfig.dimensionGroups.flatMap((g) =>
      g.bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.fieldId);
        return field ? [{ binding, field }] : [];
      })
    );
  }, [activeQueryConfig.dimensionGroups, chartBuilderFields]);

  const getMetricFields = useCallback((): BoundField[] => {
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return activeQueryConfig.metricGroups.flatMap((g) =>
      g.bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.fieldId);
        return field ? [{ binding, field }] : [];
      })
    );
  }, [activeQueryConfig.metricGroups, chartBuilderFields]);

  /**
   * 按维度组索引读取字段，供定义驱动的查询配置面板复用。
   * 调用场景：一个图表类型需要多个维度组时，例如透视表的行/列维度。
   * 主要逻辑：从指定 group 的 bindings 映射回完整字段对象（保留 bindingId 供 key/元数据查找）。
   */
  const getDimensionFieldsByGroup = useCallback(
    (groupIndex: number): BoundField[] => {
      const bindings = queryConfig.dimensionGroups[groupIndex]?.bindings || [];
      const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
      return bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.fieldId);
        return field ? [{ binding, field }] : [];
      });
    },
    [queryConfig.dimensionGroups, chartBuilderFields]
  );

  /**
   * 按指标组索引读取字段，供定义驱动的查询配置面板复用。
   * 调用场景：一个图表类型需要多个指标组时，例如散点图的 X/Y 指标。
   * 主要逻辑：从指定 group 的 bindings 映射回完整字段对象（保留 bindingId 供 key/元数据查找）。
   */
  const getMetricFieldsByGroup = useCallback(
    (groupIndex: number): BoundField[] => {
      const bindings = queryConfig.metricGroups[groupIndex]?.bindings || [];
      const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
      return bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.fieldId);
        return field ? [{ binding, field }] : [];
      });
    },
    [queryConfig.metricGroups, chartBuilderFields]
  );

  /**
   * 切换图表类型时同步补齐最小字段组数量，避免定义驱动 UI 缺少必要槽位。
   * 调用场景：用户点击右侧图表类型按钮。
   * 主要逻辑：先更新 chartType，再根据新定义对 queryConfig 做最小补齐。
   */
  const handleChartTypeChange = useCallback(
    (chartType: ChartConfig['chartType']) => {
      setChartBuilderConfig({ chartType });
      setQueryConfig(normalizeQueryConfigForChartType(chartType, useStore.getState().queryConfig));
    },
    [setChartBuilderConfig, setQueryConfig]
  );

  /**
   * 根据当前图表定义动态渲染字段组配置行。
   * 调用场景：桌面端和移动端的查询配置区域共用。
   * 主要逻辑：把图表定义中的组标签、空态文案映射到 QueryConfigRow。
   */
  const renderQueryConfigRows = useCallback(() => {
    const definition = chartDefinitions[chartBuilderConfig.chartType];

    return definition.fieldGroups.map((group, index) => {
      const groupIndex = getFieldGroupKindIndex(chartBuilderConfig.chartType, index, group.kind);
      const fields =
        group.kind === 'dimension'
          ? getDimensionFieldsByGroup(groupIndex)
          : getMetricFieldsByGroup(groupIndex);

      return (
        <QueryConfigRow
          key={`${chartBuilderConfig.chartType}-${group.id}`}
          rowType={group.kind}
          groupIndex={groupIndex}
          label={group.label}
          emptyText={group.emptyText}
          fields={fields}
          availableFields={chartBuilderFields}
          aggregations={metricAggregations}
          aliases={metricAliases}
          onRemoveField={(bindingId) =>
            group.kind === 'dimension'
              ? removeDimensionField(bindingId, groupIndex)
              : removeMetricField(bindingId, groupIndex)
          }
          onAggregationChange={setMetricAggregation}
          onAddField={(field) =>
            group.kind === 'dimension'
              ? addDimensionField(field, groupIndex)
              : addMetricField(field, groupIndex)
          }
          onReorderField={(oldIndex, newIndex) =>
            group.kind === 'dimension'
              ? reorderDimensionField(oldIndex, newIndex, groupIndex)
              : reorderMetricField(oldIndex, newIndex, groupIndex)
          }
          onOpenSettings={(bound) => setFieldSettings({ kind: group.kind, bound })}
        />
      );
    });
  }, [
    addDimensionField,
    addMetricField,
    chartBuilderConfig.chartType,
    chartBuilderFields,
    getDimensionFieldsByGroup,
    getMetricFieldsByGroup,
    metricAggregations,
    metricAliases,
    removeDimensionField,
    removeMetricField,
    reorderDimensionField,
    reorderMetricField,
    setMetricAggregation,
  ]);

  /**
   * 透视表「行列切换」快捷按钮：把行维度组（下标 0）与列维度组（下标 1）的绑定整体对调。
   * 调用场景：查询配置卡片的 extra，仅 pivot 渲染。
   * 主要逻辑：只对调 bindings，不重建分组，因此 bindingId 与随之携带的别名/单位/格式元数据原样保留；
   * 两组同时为空时禁用，避免点了没有任何反馈。
   */
  const renderPivotSwapButton = () => {
    if (chartBuilderConfig.chartType !== 'pivot') {
      return undefined;
    }

    const [rowsGroup, columnsGroup] = queryConfig.dimensionGroups;
    const totalBindings = (rowsGroup?.bindings.length ?? 0) + (columnsGroup?.bindings.length ?? 0);

    return (
      <Tooltip title="把行维度与列维度整体对调">
        <Button
          size="small"
          icon={<SwapOutlined />}
          disabled={totalBindings === 0}
          onClick={() => swapDimensionGroups(0, 1)}
          data-testid="pivot-swap-rows-columns"
        >
          行列切换
        </Button>
      </Tooltip>
    );
  };

  // queryConfigOverride：调用方持有比组件闭包更新的 queryConfig 时显式传入
  // （handleSortChange 的 setQueryConfig 尚未触发重渲染），保证 compose 看到最新 sort。
  const buildChartQueryRequest = useCallback(
    (queryConfigOverride?: QueryConfig): ChartQueryRequest | null => {
      if (!selectedDatasetId) return null;

      return composeChartQueryRequest({
        datasetId: selectedDatasetId,
        chartType: chartBuilderConfig.chartType,
        queryConfig: queryConfigOverride ?? queryConfig,
        fields: chartBuilderFields,
        metricAggregations,
        metricAliases,
        tablePagination: { page: tablePagination.page, pageSize: tablePagination.pageSize },
        queryOptions: chartQueryOptions,
        includeSort: true,
      });
    },
    [
      selectedDatasetId,
      metricAggregations,
      metricAliases,
      queryConfig,
      chartBuilderConfig.chartType,
      tablePagination.page,
      tablePagination.pageSize,
      chartBuilderFields,
      chartQueryOptions,
    ]
  );

  /**
   * 查询 → 落库 → 地址栏换成短码。这是「地址栏即分享」的唯一写入点。
   *
   * 三条刻意的约定：
   * ① 只在查询**成功**后落库——失败配置不该变成一条能被分享的链接；
   * ② spec 与上次落库的完全一致就跳过：spec_hash 由后端按内容算，同配置重复提交
   *    只会拿回同一个短码，跳过纯粹是为了不白发请求；
   * ③ 落库失败只记控制台——结果已经渲染在屏幕上，少一条分享链接不该把一次成功的
   *    查询变成失败弹窗。
   */
  const runChartQuery = useCallback(
    async (request: ChartQueryRequest) => {
      const startedAt = Date.now();
      const succeeded = await executeChartQuery(request);
      const durationMs = Date.now() - startedAt;
      if (!succeeded || !selectedDatasetId) {
        return;
      }

      const spec = buildQuerySpecDocument({
        chartType: chartBuilderConfig.chartType,
        title: chartBuilderConfig.title,
        queryConfig,
        activeQueryConfig,
        dimensionLabels,
        metricAggregations,
        metricAliases,
        metricUnits,
        metricFormats,
        chartStyle,
        chartQueryOptions,
      });

      const specKey = JSON.stringify(spec);
      if (specKey === lastPersistedSpecRef.current) {
        return;
      }

      const resultRows = countResultRows(useStore.getState().chartData);
      try {
        const response = await queriesApi.save({
          dataset_id: selectedDatasetId,
          chart_id: editingChartId ?? 0,
          spec,
          source_type: 'build',
          ...(resultRows === undefined ? {} : { row_count: resultRows }),
          duration_ms: durationMs,
        });
        const shortId = response.data.data.query_id;
        lastPersistedSpecRef.current = specKey;
        // 自签发的短码必须记账：否则地址栏一变就会触发 ?q= 还原，还原又触发自动
        // 查询与落库，形成死循环（见 settledShortIdRef 的说明）。
        settledShortIdRef.current = shortId;
        setShareShortId(shortId);
        // 地址栏只留短码：q 存在时 edit / datasetId 整体失效（PRD §5.2 优先级）。
        // replace 而不是 push，免得每次改条件都往浏览器历史里塞一条。
        setSearchParams({ q: shortId }, { replace: true });
      } catch (error) {
        console.error('查询记录落库失败：', error);
      }
    },
    [
      activeQueryConfig,
      chartBuilderConfig.chartType,
      chartBuilderConfig.title,
      chartQueryOptions,
      chartStyle,
      dimensionLabels,
      editingChartId,
      executeChartQuery,
      metricAggregations,
      metricAliases,
      metricFormats,
      metricUnits,
      queryConfig,
      selectedDatasetId,
      setSearchParams,
    ]
  );

  /** 复制当前地址栏（= 本条查询的分享链接）。 */
  const handleCopyShareLink = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(window.location.href);
      message.success('链接已复制，发给谁谁就能看到同一张图');
    } catch (error) {
      console.error('复制链接失败：', error);
      message.error('复制失败，请手动复制地址栏');
    }
  }, []);

  const handleExecuteQuery = useCallback(() => {
    const request = buildChartQueryRequest();
    if (request) {
      void runChartQuery(request);
    }
  }, [buildChartQueryRequest, runChartQuery]);

  const handlePageChange = useCallback(
    (page: number, pageSize: number) => {
      setTablePagination({ ...tablePagination, page, pageSize });
      const request = buildChartQueryRequest();
      if (request) {
        request.pagination = { page, page_size: pageSize };
        void runChartQuery(request);
      }
    },
    [buildChartQueryRequest, runChartQuery, setTablePagination, tablePagination]
  );

  const handleSortChange = useCallback(
    (sort: { field: string; order: 'asc' | 'desc' } | null) => {
      // TableChart 回传 null 表示用户取消了排序（第三次点击表头）：清掉 sort 后重查，
      // 让数据回到后端的自然顺序，而不是留着一个已经点掉的箭头状态。
      if (!sort) {
        const cleared: QueryConfig = { ...queryConfig, sort: undefined };
        setQueryConfig({ sort: undefined });
        const request = buildChartQueryRequest(cleared);
        if (request) {
          void runChartQuery(request);
        }
        return;
      }

      // TableChart 回传的是被点击的输出列名（sorter.field）：先反查对应 bindingId
      // 再写入 queryConfig.sort（R-50：sort 引用 bindingId）。列无对应绑定时忽略本次点击。
      const bindingId = findBindingIdByOutputName(
        queryConfig,
        chartBuilderFields,
        metricAliases,
        sort.field
      );
      if (!bindingId) return;
      const nextSort = { bindingId, order: sort.order };
      setQueryConfig({ sort: nextSort });
      // 闭包里的 queryConfig 还是旧值（setQueryConfig 尚未触发重渲染），显式传覆盖
      const request = buildChartQueryRequest({ ...queryConfig, sort: nextSort });
      if (request) {
        void runChartQuery(request);
      }
    },
    [
      buildChartQueryRequest,
      chartBuilderFields,
      runChartQuery,
      metricAliases,
      queryConfig,
      setQueryConfig,
    ]
  );

  useEffect(() => {
    fetchDatasets();
  }, [fetchDatasets]);

  useEffect(() => {
    // 优先级 q > edit > datasetId（PRD §5.2）：带短码时整条地址栏以记录为准，
    // 其余参数一律忽略，免得两条还原路径互相覆盖。
    if (searchParams.get('q')) {
      return;
    }
    const editId = searchParams.get('edit');
    const datasetIdParam = searchParams.get('datasetId');

    if (editId && datasetIdParam) {
      const chartId = parseInt(editId, 10);
      const dsId = parseInt(datasetIdParam, 10);

      if (!Number.isNaN(chartId) && !Number.isNaN(dsId)) {
        setSelectedDatasetId(dsId);
        setEditingChartId(chartId);
      }
    }
  }, [searchParams]);

  /**
   * `?q=<短码>` 直链还原（R-30'）。记录里存的是完整图表配置文档，因此还原出来
   * 的界面与发送方逐字段一致——这也是「复制地址栏即分享」的全部实现。
   *
   * 只做一次的判定见 settledShortIdRef：自签发的短码与已还原过的短码都不会再进
   * 这个分支，否则「还原 → 自动查询 → 落库 → 地址栏换短码 → 再还原」会死循环。
   */
  useEffect(() => {
    const shortId = searchParams.get('q');
    if (!shortId || shortId === settledShortIdRef.current) {
      return;
    }

    let cancelled = false;
    const restore = async () => {
      try {
        // 同一短码共用一次请求：StrictMode 的二次挂载（以及任何快速重挂载）都会再跑
        // 一遍本 effect，直接复用即可，既不重复打后端也不影响结果可用性。
        if (inflightRestoreRef.current?.shortId !== shortId) {
          inflightRestoreRef.current = {
            shortId,
            promise: queriesApi.getByShortId(shortId).then((response) => response.data.data),
          };
        }
        const record = await inflightRestoreRef.current.promise;
        if (cancelled) {
          return;
        }
        // 记录已被消费才记账（见 settledShortIdRef 的说明）：放在请求之前会把
        // 重挂载后的第二次执行提前挡掉，还原就再也没机会执行。
        settledShortIdRef.current = shortId;

        // 记录来自地址栏而不是图表库：保存必须新建，绝不覆盖别人的图表。
        setEditingChartId(null);
        setSelectedDatasetId(record.dataset_id);
        setShareShortId(shortId);

        const document = parseQuerySpecDocument(record.spec);
        if (!document) {
          setQueryRecordNotice({
            kind: 'warning',
            text: '这条查询记录的配置版本无法识别，已回退为空白查询',
          });
          return;
        }
        applyConfigDocument(document, document.title);
        if (record.expired) {
          // 软过期只影响默认视图排序，直链照常完整还原（R-42 / U-10）：
          // 条件 100% 可用，只是提醒使用者口径可能已经变了。
          setQueryRecordNotice({ kind: 'warning', text: '此查询已过期，条件可能已不适用' });
        }
      } catch (error: unknown) {
        // 失败不留缓存，重新进入该链接时照常重试。
        inflightRestoreRef.current = null;
        if (cancelled) {
          return;
        }
        console.error('查询记录还原失败：', error);
        setQueryRecordNotice({
          kind: 'error',
          text: error instanceof Error ? error.message : '分享链接无效或已失效',
        });
      }
    };
    void restore();

    return () => {
      cancelled = true;
    };
  }, [searchParams, applyConfigDocument]);

  /**
   * runChartQuery 的最新引用。查询 effect 的依赖列表沿用改动前的口径（图型 / 配置 /
   * 字段 / 分页 / 选项），**不能**直接依赖 runChartQuery——它还依赖别名、单位、格式、
   * 样式，挂上去会让「改个单位显示」也触发一次查询，并在查询记录里留下一条谁都没要
   * 的记录。用户动作回调（执行 / 翻页 / 排序）不受此限制，照常直接依赖。
   */
  const runChartQueryRef = useRef(runChartQuery);
  useEffect(() => {
    runChartQueryRef.current = runChartQuery;
  }, [runChartQuery]);

  useEffect(() => {
    if (selectedDatasetId) {
      fetchDatasetFields(selectedDatasetId);
    } else {
      resetChartBuilder();
    }
  }, [selectedDatasetId, fetchDatasetFields, resetChartBuilder]);

  useEffect(() => {
    if (!selectedDatasetId) return;

    const state = useStore.getState();
    const request = composeChartQueryRequest({
      datasetId: selectedDatasetId,
      chartType: state.chartBuilderConfig.chartType,
      queryConfig: state.queryConfig,
      fields: state.chartBuilderFields,
      metricAggregations: state.metricAggregations,
      metricAliases: state.metricAliases,
      tablePagination: state.tablePagination,
      queryOptions: state.chartQueryOptions,
      includeSort: true,
    });
    if (request) {
      void runChartQueryRef.current(request);
    }
  }, [selectedDatasetId]);

  useEffect(() => {
    if (!autoQuery) return;
    if (!selectedDatasetId) return;
    // 过滤弹窗打开期间挂起：条件输入是拖拽的中途态，不发起查询。
    // 关闭弹窗时本 effect 经 filterEditing 依赖重跑，确定/取消都会自然补一次查询。
    if (filterEditing) return;

    const request = composeChartQueryRequest({
      datasetId: selectedDatasetId,
      chartType: chartBuilderConfig.chartType,
      queryConfig,
      fields: chartBuilderFields,
      metricAggregations,
      metricAliases,
      tablePagination: { page: tablePagination.page, pageSize: tablePagination.pageSize },
      queryOptions: chartQueryOptions,
      includeSort: true,
    });
    if (request) {
      void runChartQueryRef.current(request);
    }
  }, [
    autoQuery,
    chartBuilderConfig.chartType,
    selectedDatasetId,
    filterEditing,
    queryConfig,
    metricAggregations,
    metricAliases,
    chartBuilderFields,
    tablePagination.page,
    tablePagination.pageSize,
    chartQueryOptions,
  ]);

  useEffect(() => {
    const loadChartConfig = async () => {
      if (editingChartId && selectedDatasetId) {
        try {
          // 图表详情按 editingChartId 缓存：字段列表到达后本 effect 会重跑，
          // 需要用「当前已加载的 chartBuilderFields」重新解析同一份 config，
          // 而不是再次请求后端。
          if (!editChartCache.current || editChartCache.current.id !== editingChartId) {
            editChartCache.current = {
              id: editingChartId,
              promise: import('../api').then(({ chartsApi }) =>
                chartsApi.getById(editingChartId).then((response) => response.data.data)
              ),
            };
          }
          const chart = await editChartCache.current.promise;

          // 统一经迁移函数读取：旧结构（位置 id + 5 个平铺 Record）在解析边界
          // 翻译为列名 + fieldMeta；损坏输入回退到 chart.chart_type / chart.name
          // （migrateChartConfig 不抛异常）。
          // 运行时 fieldId 已等于列名，旧位置 id 无法直接命中，故按列顺序重建
          // field-N → 列名 的位置映射交给迁移函数；v1 文档的列名 id 解析不到时
          // 原样保留（无损）。
          const doc = migrateChartConfig(
            chart.config,
            chart.chart_type as ChartType,
            chartBuilderFields.map((field, index) => ({
              // id：列的稳定标识（历史文档的列名引用在此升级为列 ID）
              id: field.id,
              name: field.name,
              // legacyId：旧结构（无 version）里的位置 id，仅用于解析那类文档
              legacyId: `field-${index}`,
            }))
          );

          applyConfigDocument(doc, chart.name);
        } catch (error) {
          editChartCache.current = null;
          console.error('Failed to load chart config:', error);
        }
      }
    };

    loadChartConfig();
  }, [editingChartId, selectedDatasetId, chartBuilderFields, applyConfigDocument]);

  const handleSave = async () => {
    if (!selectedDatasetId) {
      message.error('请先选择数据集');
      return;
    }

    try {
      // v2 持久化文档：与查询记录（bi_query.spec_json）走同一个构造出口
      // lib/querySpec.buildChartConfigDocument——两条路径必须还原出逐字段一致的配置。
      const doc: ChartConfigDocument = buildChartConfigDocument({
        chartType: chartBuilderConfig.chartType,
        title: chartBuilderConfig.title,
        queryConfig,
        activeQueryConfig,
        dimensionLabels,
        metricAggregations,
        metricAliases,
        metricUnits,
        metricFormats,
        chartStyle,
        chartQueryOptions,
      });
      const configJson = JSON.stringify(doc);

      if (editingChartId) {
        await updateChart(editingChartId, {
          name: chartBuilderConfig.title,
          dataset_id: selectedDatasetId,
          chart_type: chartBuilderConfig.chartType,
          config: configJson,
        });
        message.success('图表更新成功');
      } else {
        const newChart = await addChart({
          name: chartBuilderConfig.title,
          dataset_id: selectedDatasetId,
          chart_type: chartBuilderConfig.chartType,
          config: configJson,
        });
        message.success('图表保存成功');
        setEditingChartId(newChart.id);
      }
    } catch (error: any) {
      message.error(error.message || '保存失败');
    }
  };

  const handleReset = () => {
    resetChartBuilder();
    setSelectedDatasetId(null);
    setEditingChartId(null);
    message.info('已重置');
  };

  const handleDatasetChange = (value: number | null) => {
    // 重新选择同一个数据集也要重拉字段：字段改名发生在数据集详情页时，本页 store
    // 里的 chartBuilderFields 是旧快照——而「同值赋值」不会触发 selectedDatasetId
    // 的 effect。不重拉的话查询配置里的字段芯片会一直显示旧名。
    if (value !== null && value === selectedDatasetId) {
      void fetchDatasetFields(value);
    }
    setSelectedDatasetId(value);
    setEditingChartId(null);
  };

  /**
   * 数据集菜单行右侧的「跳转详情」按钮：新窗口打开 /datasets/:id。
   * 调用场景：桌面端数据集 Dropdown 与移动端 Select 的每一行。
   * 主要逻辑：stopPropagation + preventDefault，避免触发所在行的选中行为；
   * window.open(_, '_blank') 新窗口打开，不丢当前查询页的工作现场。
   */
  const renderDatasetJumpAction = (datasetId: number, datasetName: string) => (
    <Button
      type="text"
      size="small"
      icon={<ExportOutlined />}
      aria-label={`在新窗口打开数据集 ${datasetName}`}
      title="在新窗口打开数据集详情"
      data-testid={`dataset-jump-${datasetId}`}
      onClick={(e) => {
        e.stopPropagation();
        e.preventDefault();
        window.open(`/datasets/${datasetId}`, '_blank');
      }}
      style={{ marginLeft: 'auto', flexShrink: 0 }}
    />
  );

  const renderPreview = () => {
    // pivot v2（交叉表形状 cells+col_headers+row_headers）走 PivotTable。按响应形状
    // 判别而非仅 chartType：v1 平铺 pivot（{columns,data}）仍落到下方 TableChart 分支。
    if (chartBuilderConfig.chartType === 'pivot' && isPivotV2Payload(chartData)) {
      return (
        <PivotTable
          data={chartData}
          loading={chartDataLoading}
          columnLabels={Object.fromEntries(
            getDimensionFields().map((bound) => [
              bound.field.name,
              dimensionLabels[bound.binding.bindingId] || bound.field.name,
            ])
          )}
        />
      );
    }
    if (chartBuilderConfig.chartType === 'table' || chartBuilderConfig.chartType === 'pivot') {
      const dimensionFields = getDimensionFields();
      const metricFields = getMetricFields();
      // table/pivot 的结构化响应是 {columns, data}，TableChart 消费行数组：
      // 按形状判别安全提取，空数组/形状不匹配时回退为 []。
      const tableRows =
        !Array.isArray(chartData) && 'columns' in chartData && Array.isArray(chartData.data)
          ? chartData.data
          : [];
      return (
        <TableChart
          data={tableRows}
          loading={chartDataLoading}
          columns={tableColumns}
          columnLabels={Object.fromEntries([
            // columnLabels 仍按列名索引（TableChart 按列名取行值）；同一列名有多个
            // 维度 binding 时后写入的 label 覆盖先写入的（Task 0-6+0-8 前的已知歧义）。
            ...getDimensionFields().map((bound) => [
              bound.field.name,
              dimensionLabels[bound.binding.bindingId] || bound.field.name,
            ]),
            // 指标列头：显示名 = 别名 || 列名（别名纯展示），有单位时追加「 (单位)」
            // ——与图表 label 的拼接规则一致；键仍是 wire 输出列名（= 字段列名）。
            ...getMetricFields().map((bound) => {
              const displayName = metricAliases[bound.binding.bindingId] || bound.field.name;
              const unit = metricUnits[bound.binding.bindingId];
              return [bound.field.name, unit ? `${displayName} (${unit})` : displayName];
            }),
          ])}
          dimensionNames={dimensionFields.map((bound) => bound.field.name)}
          metricNames={metricFields.map((bound) =>
            wireAliasOf(metricAliases[bound.binding.bindingId], bound.field.name)
          )}
          // 「格式」（如 0,0.00）按输出列名映射给 TableChart 做展示层格式化；
          // 只传有配置的列，排序仍按原始行值。
          metricFormats={Object.fromEntries(
            metricFields
              .map((bound) => {
                const outputName = wireAliasOf(
                  metricAliases[bound.binding.bindingId],
                  bound.field.name
                );
                return [outputName, metricFormats[bound.binding.bindingId]] as const;
              })
              .filter(([, format]) => Boolean(format))
          )}
          rowSize={chartStyle.tableRowSize}
          pagination={chartBuilderConfig.chartType === 'table' ? tablePagination : undefined}
          // 排序状态受控：单一事实源是 queryConfig.sort（bindingId 引用），
          // 表头箭头只反映它，避免"看起来排了、数据没排"的不一致。
          sortField={
            queryConfig.sort
              ? findOutputNameByBindingId(
                  queryConfig,
                  chartBuilderFields,
                  metricAliases,
                  queryConfig.sort.bindingId
                )
              : undefined
          }
          sortOrder={queryConfig.sort?.order}
          onPageChange={chartBuilderConfig.chartType === 'table' ? handlePageChange : undefined}
          onSortChange={chartBuilderConfig.chartType === 'table' ? handleSortChange : undefined}
        />
      );
    }
    if (chartBuilderConfig.chartType === 'kpi') {
      // kpi 的结构化响应是标量 {value, label}（ChartKpiResponse）：按形状判别安全提取，
      // 未查询/形状不匹配（如 legacy 裸行数组）时回退 0/''。unit/format 按 kpi 唯一
      // metric 槽位（metricGroups[0] 的首个 binding）的 bindingId 从 store 取——后端
      // KpiResponse.Unit/Format 恒为空（wire 协议限制，见 KpiProcessor 注释），
      // 展示信息只来自前端配置侧。kpi 不走 ECharts，在 ChartCanvas 之前 return。
      const kpiData =
        !Array.isArray(chartData) && 'value' in chartData && 'label' in chartData
          ? chartData
          : null;
      const kpiBindingId = queryConfig.metricGroups[0]?.bindings[0]?.bindingId;
      return (
        <KpiCard
          value={kpiData?.value ?? 0}
          label={kpiData?.label ?? ''}
          unit={kpiBindingId ? metricUnits[kpiBindingId] : undefined}
          format={kpiBindingId ? metricFormats[kpiBindingId] : undefined}
          loading={chartDataLoading}
        />
      );
    }
    return (
      <ChartErrorBoundary>
        <ChartCanvas
          config={chartBuilderConfig}
          data={chartData}
          loading={chartDataLoading}
          dimensionLabels={dimensionLabels}
          metricAliases={metricAliases}
          metricUnits={metricUnits}
          chartStyle={chartStyle}
        />
      </ChartErrorBoundary>
    );
  };

  const renderHeader = () => {
    if (isMobile) {
      return (
        <Header
          style={{
            background: 'var(--dr-surface)',
            padding: '8px 12px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid var(--dr-border)',
            flexWrap: 'wrap',
            gap: 8,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text strong style={{ fontSize: 14 }}>
              数据集:
            </Text>
            <Select
              style={{ width: 160 }}
              placeholder="选择数据集"
              value={selectedDatasetId}
              onChange={handleDatasetChange}
              allowClear
              size="small"
              options={datasets.map((ds) => ({
                value: ds.id,
                label: ds.name,
                // 每行右侧的跳转按钮：optionRender 渲染在选项行内，与桌面端 Dropdown 同一交互。
                optionRender: () => (
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
                    <span
                      style={{
                        flex: 1,
                        minWidth: 0,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {ds.name}
                    </span>
                    {renderDatasetJumpAction(ds.id, ds.name)}
                  </span>
                ),
              }))}
            />
          </div>
          <div style={{ display: 'flex', gap: 4 }}>
            <Button
              size="small"
              icon={<FieldBinaryOutlined />}
              onClick={() => setLeftDrawerOpen(true)}
              title="字段"
            />
            <Button
              size="small"
              icon={<PlayCircleOutlined />}
              onClick={handleExecuteQuery}
              disabled={!selectedDatasetId}
              type="primary"
              title="执行"
            />
            <Button
              size="small"
              icon={<CodeOutlined />}
              onClick={() => setSqlModalVisible(true)}
              disabled={isEmptyPayload(chartData)}
              title="SQL"
            />
            <Button
              size="small"
              icon={<SaveOutlined />}
              onClick={handleSave}
              disabled={!selectedDatasetId}
              type="primary"
              title="保存"
            />
            <Button
              size="small"
              icon={<LinkOutlined />}
              onClick={handleCopyShareLink}
              disabled={!shareShortId}
              title="复制链接"
              aria-label="复制分享链接"
            />
            <Button
              size="small"
              icon={<FunctionOutlined />}
              onClick={() => setRightDrawerOpen(true)}
              title="配置"
            />
          </div>
        </Header>
      );
    }

    // 桌面端不再渲染顶部 Header：数据集选择器挪到左侧栏（可用字段上方），
    // 自动查询/执行/查看 SQL/保存/重置挪到「查询配置」卡片标题行（行列切换右侧）。
    return null;
  };

  const renderContent = () => {
    if (isMobile) {
      return (
        <Content
          style={{
            padding: '8px',
            background: 'var(--dr-canvas)',
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
          }}
        >
          <Card
            title="查询配置"
            size="small"
            style={{ flex: '0 0 auto' }}
            extra={renderPivotSwapButton()}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {renderQueryConfigRows()}

              <QueryConfigRow rowType="filter" label="筛选">
                <FilterDropZone
                  filters={queryConfig.filters}
                  availableFields={chartBuilderFields}
                  onAdd={openFilterEditor}
                  onEdit={openExistingFilterEditor}
                  onRemove={handleFilterRemove}
                />
              </QueryConfigRow>
            </div>
          </Card>

          {renderDateFilterBar()}

          <Card
            title="预览"
            size="small"
            style={{ flex: 1, minHeight: 300 }}
            extra={
              <QueryStatusBadge
                loading={chartDataLoading}
                hasData={!isEmptyPayload(chartData)}
                chartTypeLabel={chartDefinitions[chartBuilderConfig.chartType].label}
              />
            }
          >
            <div style={{ height: 'calc(100vh - 400px)', minHeight: 250 }}>{renderPreview()}</div>
          </Card>
        </Content>
      );
    }

    return (
      <Layout>
        <Sider
          width={leftSiderWidth}
          style={{
            // 画布色（与中栏同源）：白卡片浮在灰底上，三栏结构由"明度差"表达。
            // 上下 0 与中栏 Content 的 padding:0 对齐（否则三栏顶部错开 8px），
            // 左右 4px 是卡片的呼吸边。
            // 竖向分栏线已移除：三栏同底色后，1px 线在两个同色面之间只是"缝"，
            // 而 4px 留白 + 6px 卡间距已经构成一条 ~14px 的灰色间隔带，足够读出分栏。
            // （移除也顺带省掉 2px 横向像素。）
            background: 'var(--dr-canvas)',
            padding: '0 4px',
            position: 'relative',
            // antd Sider 默认 transition: all .2s 会把拖宽变成"追鼠标"的动画，
            // 必须禁掉才跟手（拖宽是逐帧更新，不该有过渡）。
            transition: 'none',
          }}
        >
          {/* 数据集以纯文本展示（点击弹出选择菜单），避免下拉框截断长名称 */}
          <Dropdown
            trigger={['click']}
            placement="bottomLeft"
            menu={{
              // 每行 = 数据集名（点击切换）+ 右侧跳转按钮（新窗口开详情页）。
              items: datasets.map((ds) => ({
                key: String(ds.id),
                label: (
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 180 }}>
                    <span
                      style={{
                        flex: 1,
                        minWidth: 0,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {ds.name}
                    </span>
                    {renderDatasetJumpAction(ds.id, ds.name)}
                  </span>
                ),
              })),
              selectedKeys: selectedDatasetId != null ? [String(selectedDatasetId)] : [],
              onClick: ({ key }) => handleDatasetChange(Number(key)),
            }}
          >
            <Button
              // default（描边）而非 text：数据集是本页最重要的输入项，在灰画布上
              // 必须读起来像"一个可点的控件"，而不是一段飘着的文字。
              type="default"
              size="small"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 4,
                width: '100%',
                // 26 = 与拖放区同高，左栏顶部与中栏槽位横向对齐
                height: 26,
                marginBottom: 6,
                padding: '0 6px',
                borderRadius: 6,
                background: 'var(--dr-surface)',
                borderColor: 'var(--dr-border)',
                boxShadow: 'var(--dr-shadow-card)',
              }}
            >
              {/* 图标与左侧导航的「数据集」保持一致，让这块一眼可辨为数据集 */}
              <AppstoreOutlined
                style={{ color: 'var(--dr-text-3)', fontSize: 12, flexShrink: 0 }}
              />
              <Text
                strong={currentDataset != null}
                type={currentDataset != null ? undefined : 'secondary'}
                style={{
                  flex: 1,
                  minWidth: 0,
                  fontSize: 12,
                  textAlign: 'left',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {currentDataset?.name ?? '选择数据集'}
              </Text>
              {/* Dropdown 原先没有任何"可展开"的可见提示，补一个朝下箭头作为可供性 */}
              <DownOutlined
                style={{ color: 'var(--dr-text-4)', fontSize: 9, flexShrink: 0 }}
                aria-hidden
              />
            </Button>
          </Dropdown>
          <Card title="可用字段" size="small" styles={{ body: { padding: '2px 2px 4px' } }}>
            <FieldListPanel fields={chartBuilderFields} loading={chartBuilderFieldsLoading} />
          </Card>
          {/* 右缘拖拽手柄：按住可调节左侧栏宽度 */}
          {/* biome-ignore lint/a11y/noStaticElementInteractions: 鼠标拖拽调节手柄，非交互控件 */}
          <div
            onMouseDown={startLeftSiderResize}
            onMouseEnter={() => setResizeHandleHover(true)}
            onMouseLeave={() => setResizeHandleHover(false)}
            title="拖拽调节宽度"
            style={{
              position: 'absolute',
              top: 0,
              right: 0,
              width: 6,
              height: '100%',
              cursor: 'col-resize',
              zIndex: 10,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: resizeHandleHover ? 'rgba(22,119,255,0.18)' : 'transparent',
            }}
          >
            <div
              style={{
                width: 2,
                height: 32,
                borderRadius: 2,
                backgroundColor: resizeHandleHover ? 'var(--dr-accent)' : 'var(--dr-border-strong)',
              }}
            />
          </div>
        </Sider>

        <Content
          style={{
            // 页级容器不再留白：中栏卡片直接顶到左右分栏线，横向省 16px、纵向省 16px。
            // 卡片间距靠 gap 6 分隔，画布色只在缝隙里露出（就是这 6px 让卡片读得出边界）。
            padding: 0,
            background: 'var(--dr-canvas)',
            display: 'flex',
            flexDirection: 'column',
            gap: 6,
          }}
        >
          <Card
            title="查询配置"
            size="small"
            style={{ flex: '0 0 auto' }}
            styles={{ body: { padding: '4px 6px' } }}
            extra={
              <Space size={4} wrap>
                {renderPivotSwapButton()}
                <Space size={4}>
                  <Text type="secondary">自动查询</Text>
                  <Switch checked={autoQuery} onChange={toggleAutoQuery} size="small" />
                </Space>
                {!autoQuery && (
                  <Button
                    size="small"
                    type="primary"
                    icon={<PlayCircleOutlined />}
                    onClick={handleExecuteQuery}
                    disabled={!selectedDatasetId}
                  >
                    执行查询
                  </Button>
                )}
                {!isEmptyPayload(chartData) && (
                  <Button
                    size="small"
                    icon={<CodeOutlined />}
                    onClick={() => setSqlModalVisible(true)}
                  >
                    查看 SQL
                  </Button>
                )}
                {shareShortId && (
                  <Tooltip title="复制地址栏链接：对方打开看到的就是这一屏">
                    <Button size="small" icon={<LinkOutlined />} onClick={handleCopyShareLink}>
                      复制链接
                    </Button>
                  </Tooltip>
                )}
                <Button
                  size="small"
                  type="primary"
                  icon={<SaveOutlined />}
                  onClick={handleSave}
                  disabled={!selectedDatasetId}
                >
                  {editingChartId ? '更新' : '保存'}
                </Button>
                <Button size="small" icon={<ReloadOutlined />} onClick={handleReset}>
                  重置
                </Button>
              </Space>
            }
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {renderQueryConfigRows()}

              <QueryConfigRow rowType="filter" label="筛选">
                <FilterDropZone
                  filters={queryConfig.filters}
                  availableFields={chartBuilderFields}
                  onAdd={openFilterEditor}
                  onEdit={openExistingFilterEditor}
                  onRemove={handleFilterRemove}
                />
              </QueryConfigRow>
            </div>
          </Card>

          {renderDateFilterBar()}

          <Card
            title="预览"
            size="small"
            style={{ flex: 1, minHeight: 400 }}
            styles={{ body: { padding: 4 } }}
            extra={
              <QueryStatusBadge
                loading={chartDataLoading}
                hasData={!isEmptyPayload(chartData)}
                chartTypeLabel={chartDefinitions[chartBuilderConfig.chartType].label}
              />
            }
          >
            <div style={{ height: 'calc(100vh - 380px)', minHeight: 300 }}>{renderPreview()}</div>
          </Card>
        </Content>

        <Sider width={300} style={{ background: 'var(--dr-canvas)', padding: '0 4px' }}>
          <ConfigPanel
            config={chartBuilderConfig}
            chartStyle={chartStyle}
            onChartStyleChange={setChartStyle}
            queryOptions={chartQueryOptions}
            onQueryOptionsChange={(options) =>
              setChartQueryOptionsState({ ...chartQueryOptions, ...options })
            }
            onConfigChange={(config) => {
              if (config.chartType) {
                handleChartTypeChange(config.chartType);
                return;
              }
              setChartBuilderConfig(config);
            }}
          />
        </Sider>
      </Layout>
    );
  };

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragCancel={handleDragCancel}
      onDragEnd={handleDragEnd}
    >
      <Layout className="chart-builder-page" style={{ minHeight: 'calc(100vh - 120px)' }}>
        {renderHeader()}

        {queryRecordNotice && (
          <Alert
            type={queryRecordNotice.kind}
            message={queryRecordNotice.text}
            showIcon
            closable
            onClose={() => setQueryRecordNotice(null)}
            style={{ margin: '0 6px 6px' }}
          />
        )}

        {renderContent()}

        <Drawer
          title="可用字段"
          placement="left"
          onClose={() => setLeftDrawerOpen(false)}
          open={leftDrawerOpen}
          size={300}
        >
          <Card title="字段列表" size="small">
            <FieldListPanel fields={chartBuilderFields} loading={chartBuilderFieldsLoading} />
          </Card>
        </Drawer>

        <Drawer
          title="图表配置"
          placement="right"
          onClose={() => setRightDrawerOpen(false)}
          open={rightDrawerOpen}
          size={300}
        >
          <ConfigPanel
            config={chartBuilderConfig}
            chartStyle={chartStyle}
            onChartStyleChange={setChartStyle}
            queryOptions={chartQueryOptions}
            onQueryOptionsChange={(options) =>
              setChartQueryOptionsState({ ...chartQueryOptions, ...options })
            }
            onConfigChange={(config) => {
              if (config.chartType) {
                handleChartTypeChange(config.chartType);
                return;
              }
              setChartBuilderConfig(config);
            }}
          />
        </Drawer>
      </Layout>
      <DragOverlay>
        {activeDragField ? (
          <FieldDragPreview label={activeDragField.name} color={fieldTagColor(activeDragField)} />
        ) : activeDragBinding ? (
          <FieldDragPreview label={activeDragBinding.label} color={activeDragBinding.color} />
        ) : null}
      </DragOverlay>
      <Modal
        title="生成的 SQL"
        open={sqlModalVisible}
        onCancel={() => setSqlModalVisible(false)}
        footer={null}
        width={800}
      >
        {chartQueryResponse && (
          <div>
            <Text strong>数据查询:</Text>
            <pre
              style={{
                // --dr-* token 定义在 :root 上，portal 到 body 的 Modal 同样继承得到，
                // 故这里可以直接消费变量（早期版本把 token 挂在 .chart-builder-page 上，
                // 那时才必须写字面量）。
                background: 'var(--dr-sunken)',
                border: '1px solid var(--dr-border)',
                padding: 12,
                borderRadius: 6,
                overflow: 'auto',
                maxHeight: 300,
                fontSize: 12,
                margin: 0,
              }}
            >
              {chartQueryResponse.select_sql || '无'}
            </pre>
            {chartQueryResponse.count_sql && (
              <>
                <Text strong style={{ marginTop: 16, display: 'block' }}>
                  计数查询:
                </Text>
                <pre
                  style={{
                    background: 'var(--dr-sunken)',
                    border: '1px solid var(--dr-border)',
                    padding: 12,
                    borderRadius: 6,
                    overflow: 'auto',
                    maxHeight: 200,
                    fontSize: 12,
                    margin: 0,
                  }}
                >
                  {chartQueryResponse.count_sql}
                </pre>
              </>
            )}
          </div>
        )}
      </Modal>

      <FilterConfigModal
        open={filterEditing !== null && editingFilterCondition !== undefined}
        field={editingFilterField}
        initial={editingFilterCondition}
        isNew={filterEditing?.isNew}
        datasetId={selectedDatasetId}
        onOk={handleFilterModalOk}
        onCancel={handleFilterModalCancel}
      />

      <DateFilterModal
        open={dateFilterEditing !== null && editingDateFilterCondition !== undefined}
        fieldName={editingDateFilterField?.name ?? editingDateFilterCondition?.fieldId}
        withTime={
          editingDateFilterField
            ? normalizeDataType(editingDateFilterField.dataType) === 'datetime'
            : false
        }
        initial={
          editingDateFilterCondition?.date?.value ??
          (editingDateFilterCondition
            ? (dateFilterValueFromLegacy(
                editingDateFilterCondition.operator,
                editingDateFilterCondition.value,
                editingDateFilterCondition.valueEnd
              ) ?? undefined)
            : undefined)
        }
        initialGranularity={editingDateFilterCondition?.date?.granularity}
        initialWeekStart={editingDateFilterCondition?.date?.weekStart}
        initialAsFilter={editingDateFilterCondition?.date?.asFilter}
        initialFilterLabel={editingDateFilterCondition?.date?.label}
        onOk={handleDateFilterModalOk}
        onCancel={handleDateFilterModalCancel}
      />

      <FieldSettingsModal
        open={fieldSettings !== null}
        kind={fieldSettings?.kind ?? 'metric'}
        field={fieldSettings?.bound.field ?? null}
        initial={
          fieldSettings
            ? {
                alias:
                  fieldSettings.kind === 'metric'
                    ? (metricAliases[fieldSettings.bound.binding.bindingId] ?? '')
                    : (dimensionLabels[fieldSettings.bound.binding.bindingId] ?? ''),
                unit: metricUnits[fieldSettings.bound.binding.bindingId] ?? '',
                format: metricFormats[fieldSettings.bound.binding.bindingId] ?? '',
              }
            : { alias: '', unit: '', format: '' }
        }
        onOk={(value: FieldSettingsValue) => {
          if (!fieldSettings) return;
          const { bindingId } = fieldSettings.bound.binding;
          if (fieldSettings.kind === 'metric') {
            setMetricAlias(bindingId, value.alias);
            setMetricUnit(bindingId, value.unit);
            setMetricFormat(bindingId, value.format);
          } else {
            setDimensionLabel(bindingId, value.alias);
          }
          setFieldSettings(null);
        }}
        onCancel={() => setFieldSettings(null)}
      />
    </DndContext>
  );
};

export default ChartBuilder;
