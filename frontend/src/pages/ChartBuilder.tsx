import {
  CodeOutlined,
  FieldBinaryOutlined,
  FunctionOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
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
  Button,
  Card,
  ColorPicker,
  Divider,
  Drawer,
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
} from '../api';
import {
  chartDefinitions,
  normalizeQueryConfigForChartType,
} from '../components/ChartBuilder/chartDefinitions';
import DraggableField, { FieldDragPreview } from '../components/ChartBuilder/DraggableField';
import FilterBuilder from '../components/ChartBuilder/FilterBuilder';
import KpiCard from '../components/ChartBuilder/KpiCard';
import PivotTable from '../components/ChartBuilder/PivotTable';
import QueryConfigRow from '../components/ChartBuilder/QueryConfigRow';
import TableChart from '../components/ChartBuilder/TableChart';
import {
  type ChartConfigDocument,
  type ChartMeta,
  type ChartType,
  migrateChartConfig,
} from '../lib/chartConfigSchema';
import { buildChartOption, isEmptyPayload } from '../lib/chartOptions';
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
    const columnName = fieldMap.get(binding.field)?.name;
    if (columnName === undefined) {
      return undefined;
    }
    return kind === 'metric' ? metricAliases[binding.bindingId] || columnName : columnName;
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
 * 图表查询请求的唯一构造出口（纯函数，所有输入经参数传入）。
 * 调用场景：手动执行查询（含排序/翻页覆盖）与两个自动查询 effect 共用。
 * 主要逻辑：按图表定义裁剪字段组、字段 id 映射回列名、组装 filters/pagination；
 * 维度与指标同时为空时返回 null 表示不发起查询。
 *
 * v1/v2 判别（裁定B）：当当前图型定义了 color_group 槽位（bar/line/area）且该槽位非空，
 * **或** 当前图型定义了 combo 的双轴指标槽位（primary_values/secondary_values）时，发出 v2
 * 格式请求（spec_version:2 + dimension_groups/metric_groups，携带真实槽位名与每个字段的
 * binding_id）；combo 即使 color_group 为空也走 v2（否则主/次轴的槽位区分信息会在 v1 平铺
 * metrics[] 中丢失）。其余情况（color_group 为空的 bar/line/area，或 table/pie/scatter/pivot
 * 根本没有这些槽位）继续发出 v1 平铺格式（dims/metrics），请求形状与本任务改动前完全一致。
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

  const filtersPayload = queryConfig.filters.map((f) => {
    const field = fields.find((candidate) => candidate.id === f.field);
    return {
      field: field?.name || f.field,
      operator: f.operator,
      value: f.value,
      value_end: f.valueEnd,
      logic: f.logic,
    };
  });
  const paginationPayload =
    chartType === 'table'
      ? {
          page: tablePagination.page,
          page_size: tablePagination.pageSize,
        }
      : undefined;

  // color_group 槽位判定：只有 bar/line/area 的 fieldGroups 里定义了 id==='color_group'
  // 的维度槽位；找到它在"仅维度槽位"序列里的序号，映射到 activeGroups.dimensionGroups
  // 的同序号（kind-local index，与 getFieldGroupKindIndex 的既有约定一致）。
  const definition = chartDefinitions[chartType];
  const dimensionDefs = definition.fieldGroups.filter((group) => group.kind === 'dimension');
  const metricDefs = definition.fieldGroups.filter((group) => group.kind === 'metric');
  const colorGroupDefIndex = dimensionDefs.findIndex((group) => group.id === 'color_group');
  const colorGroupBindings =
    colorGroupDefIndex >= 0
      ? (activeGroups.dimensionGroups[colorGroupDefIndex]?.bindings ?? [])
      : [];

  // v2 触发条件：color_group 非空（bar/line/area）**或** 图型带 combo 的双轴指标槽位。
  // 后者保证 combo 即使 color_group 为空也走 v2，主/次轴槽位名不丢失（详见函数 doc）。
  const requiresSlotProtocol =
    colorGroupBindings.length > 0 ||
    metricDefs.some((group) => DUAL_AXIS_METRIC_SLOTS.has(group.id));

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
    const columnName = fieldMap.get(sortBinding.binding.field)?.name;
    if (columnName === undefined) {
      return undefined;
    }
    if (requiresSlotProtocol) {
      return sortBinding.binding.bindingId;
    }
    return sortBinding.kind === 'metric'
      ? metricAliases[sortBinding.binding.bindingId] || columnName
      : columnName;
  })();
  // funnel（漏斗图，R-59）强制降序（plan §3.3，验收行767，裁定 F）：sort 恒为 value
  // 槽位输出名的 desc，无条件覆盖 includeSort 与 queryConfig.sort（用户没有 funnel 的
  // sort UI，但持久化文档恢复可能带任意 sort）。funnel 走 v1 平铺路径，输出名与 v1
  // sortWireField 同口径：metricAliases[bindingId] || 列名。value 绑定缺失/列名查不到
  // （异常防御）时不注入 sort，请求照常发（funnel 仍渲染，只是顺序不保证）。
  const funnelSortPayload = (() => {
    if (chartType !== 'funnel') {
      return undefined;
    }
    const valueBinding = activeGroups.metricGroups[0]?.bindings[0];
    const columnName = valueBinding ? fieldMap.get(valueBinding.field)?.name : undefined;
    if (!valueBinding || columnName === undefined) {
      return undefined;
    }
    return {
      sort: {
        field: metricAliases[valueBinding.bindingId] || columnName,
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
        const chartField = fieldMap.get(binding.field);
        if (!chartField) {
          return [];
        }
        return [{ field: chartField.name, binding_id: binding.bindingId }];
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
        const chartField = fieldMap.get(binding.field);
        if (!chartField) {
          return [];
        }
        return [
          {
            field: chartField.name,
            agg: (metricAggregations[binding.bindingId] || 'sum') as ChartQueryAggregation,
            alias: metricAliases[binding.bindingId] || chartField.name,
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

  // v1 平铺协议（color_group 为空，或该图型没有 color_group 槽位）：逻辑与改动前完全一致
  const dimensionFields = activeGroups.dimensionGroups
    .flatMap((group) => group.bindings.map((b) => b.field))
    .map((name) => fieldMap.get(name))
    .filter((f): f is ChartField => f !== undefined);

  // 已知限制（Task 0-6+0-8 解决）：wire 这一层 metrics[].field 仍是列名，若同一列名
  // 出现在两个不同指标组（D2 场景），flatMap 会产生重复列名，后端将收到两个相同 field
  // 的 metric 配置。本阶段按现有逻辑原样发送、不去重——真正区分要等 v2 wire 的 bindingId 别名。
  const metrics = activeGroups.metricGroups
    .flatMap((group) => group.bindings)
    .flatMap((binding) => {
      const chartField = fieldMap.get(binding.field);
      if (!chartField) {
        return [];
      }
      return [
        {
          field: chartField.name,
          agg: (metricAggregations[binding.bindingId] || 'sum') as ChartQueryAggregation,
          alias: metricAliases[binding.bindingId] || chartField.name,
        },
      ];
    });

  if (dimensionFields.length === 0 && metrics.length === 0) {
    return null;
  }

  return {
    dataset_id: datasetId,
    chart_type: chartType,
    dims: dimensionFields.map((f) => f.name),
    metrics,
    filters: filtersPayload,
    ...sortPayload,
    ...queryOptionsPayload,
    pagination: paginationPayload,
  };
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
      .map((b) => fieldMap.get(b.field)?.name)
      .filter((name): name is string => name !== undefined);
    const metrics = metricBindings
      .map((b) => fieldMap.get(b.field)?.name)
      .filter((name): name is string => name !== undefined);

    // labels 的键保持列名（buildChartOption 按列名/系列名查显示名），但 label/alias/unit
    // 的值按 bindingId 取（五个 Record 已改为 bindingId 键）。
    const labels: Record<string, string> = {};
    for (const binding of dimensionBindings) {
      const name = fieldMap.get(binding.field)?.name;
      const label = dimensionLabels[binding.bindingId];
      if (name && label) {
        labels[name] = label;
      }
    }
    for (const binding of metricBindings) {
      const baseName = fieldMap.get(binding.field)?.name || binding.field;
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
                .map((b) => metricAliases[b.bindingId] || fieldMap.get(b.field)?.name)
                .filter((name): name is string => name !== undefined),
            }))
        : undefined;

    return buildChartOption(config.chartType, data, chartStyle, labels, {
      title: config.title,
      dimensions,
      metrics,
      metricSlots,
    });
  }, [chartStyle, config, data, dimensionLabels, metricAliases, metricUnits]);

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">加载图表数据中...</Text>
        </div>
      </div>
    );
  }

  if (!chartOption) {
    return (
      <Empty
        description="请配置维度和指标以生成图表"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        style={{ padding: '100px 0' }}
      />
    );
  }

  return (
    <ReactECharts
      option={chartOption}
      style={{ height: '100%', width: '100%' }}
      opts={{ renderer: 'canvas' }}
    />
  );
};

interface FieldListPanelProps {
  fields: ChartField[];
  loading: boolean;
}

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
      <div style={{ marginBottom: 12 }}>
        <Text strong type="secondary" style={{ display: 'block', marginBottom: 6 }}>
          维度 ({dimensions.length})
        </Text>
        {dimensions.map((field) => (
          <DraggableField key={field.id} field={field} />
        ))}
      </div>

      <Divider style={{ margin: '8px 0' }} />

      <div>
        <Text strong type="secondary" style={{ display: 'block', marginBottom: 6 }}>
          指标 ({metrics.length})
        </Text>
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
  dimensionLabels: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
  chartStyle: ChartStyleConfig;
  dimensionFields: BoundField[];
  metricFields: BoundField[];
  onDimensionLabelChange: (bindingId: string, label: string) => void;
  onMetricUnitChange: (bindingId: string, unit: string) => void;
  onMetricFormatChange: (bindingId: string, format: string) => void;
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

const ConfigPanel: React.FC<ConfigPanelProps> = ({
  config,
  onConfigChange,
  dimensionLabels,
  metricUnits,
  metricFormats,
  chartStyle,
  dimensionFields,
  metricFields,
  onDimensionLabelChange,
  onMetricUnitChange,
  onMetricFormatChange,
  onChartStyleChange,
  queryOptions,
  onQueryOptionsChange,
}) => {
  // styleKeys 决定当前图型显示哪些样式控件（Task 0-4 声明、本任务首次真正接线）。
  // 7 种图型现在都应显式声明 styleKeys（见 chartDefinitions.ts），undefined 理论上
  // 不应再出现；防御性地按"不显示任何样式控件"处理，避免误渲染出该图型不消费的开关。
  const styleKeys = chartDefinitions[config.chartType].styleKeys;
  const showStyleControl = (key: keyof ChartStyleConfig) => styleKeys?.includes(key) ?? false;

  return (
    <div>
      <Card title="可视化类型" size="small" style={{ marginBottom: 12 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 8 }}>
          {chartTypeOptions.map((opt) => (
            <Button
              key={opt.type}
              type={config.chartType === opt.type ? 'primary' : 'default'}
              icon={React.createElement(opt.icon)}
              onClick={() => onConfigChange({ chartType: opt.type })}
              style={{ height: 40 }}
            >
              {opt.label}
            </Button>
          ))}
        </div>
      </Card>

      <Card title="图表配置" size="small" style={{ marginBottom: 12 }}>
        <Space orientation="vertical" style={{ width: '100%' }} size="small">
          <div>
            <Text strong>图表标题</Text>
            <Input
              style={{ width: '100%', marginTop: 4 }}
              value={config.title}
              onChange={(e) => onConfigChange({ title: e.target.value })}
              placeholder="输入图表标题"
            />
          </div>

          {showStyleControl('smooth') && (
            <div>
              <Text strong>平滑曲线</Text>
              <div style={{ marginTop: 4 }}>
                <Switch
                  checked={chartStyle.smooth}
                  onChange={(checked) => onChartStyleChange({ smooth: checked })}
                />
              </div>
            </div>
          )}

          {showStyleControl('colors') && (
            <div>
              <Text strong>主色</Text>
              <div style={{ marginTop: 4 }}>
                <ColorPicker
                  value={chartStyle.colors[0] || '#1677ff'}
                  onChange={(color) => onChartStyleChange({ colors: [color.toHexString()] })}
                />
              </div>
            </div>
          )}

          {config.chartType === 'histogram' && (
            <div>
              <Text strong>分箱数量</Text>
              <div style={{ marginTop: 4 }}>
                <InputNumber
                  style={{ width: '100%' }}
                  min={1}
                  max={1000}
                  precision={0}
                  value={queryOptions.binCount ?? 20}
                  onChange={(value) => onQueryOptionsChange({ binCount: value ?? undefined })}
                />
              </div>
            </div>
          )}

          {showStyleControl('stack') && (
            <div>
              <Text strong>堆叠模式</Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={chartStyle.stack ?? 'none'}
                onChange={(value) => onChartStyleChange({ stack: value })}
                options={[
                  { value: 'none', label: '不堆叠' },
                  { value: 'normal', label: '堆叠' },
                  { value: 'percent', label: '百分比堆叠' },
                ]}
              />
            </div>
          )}

          {showStyleControl('orientation') && (
            <div>
              <Text strong>方向</Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={chartStyle.orientation ?? 'vertical'}
                onChange={(value) => onChartStyleChange({ orientation: value })}
                options={[
                  { value: 'vertical', label: '纵向' },
                  { value: 'horizontal', label: '横向' },
                ]}
              />
            </div>
          )}

          {showStyleControl('donut') && (
            <div>
              <Text strong>环形图</Text>
              <div style={{ marginTop: 4 }}>
                <Switch
                  checked={chartStyle.donut ?? false}
                  onChange={(checked) => onChartStyleChange({ donut: checked })}
                />
              </div>
            </div>
          )}

          {showStyleControl('tableRowSize') && (
            <div>
              <Text strong>表格行尺寸</Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={chartStyle.tableRowSize}
                onChange={(value) => onChartStyleChange({ tableRowSize: value })}
                options={[
                  { value: 'small', label: '紧凑' },
                  { value: 'middle', label: '默认' },
                  { value: 'large', label: '宽松' },
                ]}
              />
            </div>
          )}
        </Space>
      </Card>

      {dimensionFields.length > 0 && (
        <Card title="维度属性" size="small" style={{ marginBottom: 12 }}>
          <Space orientation="vertical" style={{ width: '100%' }} size="small">
            {dimensionFields.map((bound) => (
              <div key={bound.binding.bindingId}>
                <Text strong>{bound.field.name}</Text>
                <Input
                  style={{ width: '100%', marginTop: 4 }}
                  value={dimensionLabels[bound.binding.bindingId] || ''}
                  onChange={(e) => onDimensionLabelChange(bound.binding.bindingId, e.target.value)}
                  placeholder="显示名称"
                />
              </div>
            ))}
          </Space>
        </Card>
      )}

      {metricFields.length > 0 && (
        <Card title="指标属性" size="small" style={{ marginBottom: 12 }}>
          <Space orientation="vertical" style={{ width: '100%' }} size="small">
            {metricFields.map((bound) => (
              <div key={bound.binding.bindingId}>
                <Text strong>{bound.field.name}</Text>
                <Input
                  style={{ width: '100%', marginTop: 4, marginBottom: 4 }}
                  value={metricUnits[bound.binding.bindingId] || ''}
                  onChange={(e) => onMetricUnitChange(bound.binding.bindingId, e.target.value)}
                  placeholder="单位，例如 元 / %"
                />
                <Input
                  style={{ width: '100%' }}
                  value={metricFormats[bound.binding.bindingId] || ''}
                  onChange={(e) => onMetricFormatChange(bound.binding.bindingId, e.target.value)}
                  placeholder="格式，例如 0,0.00"
                />
              </div>
            ))}
          </Space>
        </Card>
      )}

      <Card title="当前配置" size="small" style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 12 }}>
          <div style={{ marginBottom: 6 }}>
            <Text type="secondary">类型: </Text>
            <Text strong>{config.chartType.toUpperCase()}</Text>
          </div>
          <div style={{ marginBottom: 6 }}>
            <Text type="secondary">标题: </Text>
            <Text strong>{config.title}</Text>
          </div>
        </div>
      </Card>
    </div>
  );
};

/**
 * 图表构建页负责组装字段拖拽、查询配置和图表渲染三块交互。
 * 调用场景：`/chart-builder` 页面。
 * 主要逻辑：同步 store 状态、处理字段拖放、并在拖拽期间渲染 overlay 预览。
 */
const ChartBuilder: React.FC = () => {
  const [searchParams] = useSearchParams();
  const [selectedDatasetId, setSelectedDatasetId] = useState<number | null>(null);
  const [editingChartId, setEditingChartId] = useState<number | null>(null);
  const [sqlModalVisible, setSqlModalVisible] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const [leftDrawerOpen, setLeftDrawerOpen] = useState(false);
  const [rightDrawerOpen, setRightDrawerOpen] = useState(false);
  const [activeDragField, setActiveDragField] = useState<ChartField | null>(null);
  // 编辑态图表详情缓存（id + 请求 Promise），供配置加载 effect 重跑时复用
  const editChartCache = useRef<{ id: number; promise: Promise<Chart> } | null>(null);

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
   * 记录当前正在拖拽的字段，用于渲染跟随鼠标移动的 overlay 预览。
   * 调用场景：字段从左侧字段列表开始拖动时。
   * 主要逻辑：从 active.data.current 提取字段并写入本地状态。
   */
  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveDragField(getDraggedField(event.active.data.current));
  }, []);

  /**
   * 在取消拖拽时清理 overlay 预览状态。
   * 调用场景：用户松手但未命中 drop zone，或拖拽流程被中断。
   * 主要逻辑：将当前拖拽字段置空，移除浮层预览。
   */
  const handleDragCancel = useCallback(() => {
    setActiveDragField(null);
  }, []);

  /**
   * 处理字段拖放完成后的查询配置更新。
   * 调用场景：字段拖到维度、指标或筛选区域后触发。
   * 主要逻辑：先清理 overlay，再根据目标区域把字段加入对应查询配置。
   */
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActiveDragField(null);

      const { active, over } = event;

      if (!over) return;

      const field = getDraggedField(active.data.current);
      if (!field) return;

      const overData = over.data.current;
      const dropZoneType = overData?.type as 'dimension' | 'metric' | 'filter';
      const groupIndex = typeof overData?.groupIndex === 'number' ? overData.groupIndex : 0;

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
        addFilter({
          id: `filter-${Date.now()}`,
          field: field.id,
          operator: 'eq',
          value: '',
          logic: 'and',
        });
      }
    },
    [addDimensionField, addMetricField, addFilter]
  );

  const getDimensionFields = useCallback((): BoundField[] => {
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return queryConfig.dimensionGroups.flatMap((g) =>
      g.bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.field);
        return field ? [{ binding, field }] : [];
      })
    );
  }, [queryConfig.dimensionGroups, chartBuilderFields]);

  const getMetricFields = useCallback((): BoundField[] => {
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return queryConfig.metricGroups.flatMap((g) =>
      g.bindings.flatMap((binding) => {
        const field = fieldMap.get(binding.field);
        return field ? [{ binding, field }] : [];
      })
    );
  }, [queryConfig.metricGroups, chartBuilderFields]);

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
        const field = fieldMap.get(binding.field);
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
        const field = fieldMap.get(binding.field);
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
          onOpenSettings={
            group.kind === 'metric'
              ? (bound) => {
                  const alias = prompt('输入字段别名:', bound.field.name);
                  if (alias !== null) {
                    setMetricAlias(bound.binding.bindingId, alias);
                  }
                }
              : undefined
          }
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
    setMetricAlias,
  ]);

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

  const handleExecuteQuery = useCallback(() => {
    const request = buildChartQueryRequest();
    if (request) {
      executeChartQuery(request);
    }
  }, [buildChartQueryRequest, executeChartQuery]);

  const handlePageChange = useCallback(
    (page: number, pageSize: number) => {
      setTablePagination({ ...tablePagination, page, pageSize });
      const request = buildChartQueryRequest();
      if (request) {
        request.pagination = { page, page_size: pageSize };
        executeChartQuery(request);
      }
    },
    [buildChartQueryRequest, executeChartQuery, setTablePagination, tablePagination]
  );

  const handleSortChange = useCallback(
    (sort: { field: string; order: 'asc' | 'desc' }) => {
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
        executeChartQuery(request);
      }
    },
    [
      buildChartQueryRequest,
      chartBuilderFields,
      executeChartQuery,
      metricAliases,
      queryConfig,
      setQueryConfig,
    ]
  );

  useEffect(() => {
    fetchDatasets();
  }, [fetchDatasets]);

  useEffect(() => {
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
      executeChartQuery(request);
    }
  }, [selectedDatasetId, executeChartQuery]);

  useEffect(() => {
    if (!autoQuery) return;
    if (!selectedDatasetId) return;

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
      executeChartQuery(request);
    }
  }, [
    autoQuery,
    chartBuilderConfig.chartType,
    selectedDatasetId,
    executeChartQuery,
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
              id: `field-${index}`,
              name: field.name,
            }))
          );

          setChartBuilderConfig({
            chartType: doc.chartType,
            title: doc.title || chart.name,
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
        } catch (error) {
          editChartCache.current = null;
          console.error('Failed to load chart config:', error);
        }
      }
    };

    loadChartConfig();
  }, [
    editingChartId,
    selectedDatasetId,
    chartBuilderFields,
    setChartBuilderConfig,
    setMetricAggregations,
    setMetricAliases,
    setDimensionLabels,
    setMetricUnits,
    setMetricFormats,
    setChartStyleState,
    setChartQueryOptionsState,
    setQueryConfig,
  ]);

  const handleSave = async () => {
    if (!selectedDatasetId) {
      message.error('请先选择数据集');
      return;
    }

    try {
      // v2 持久化文档：字段组是 bindings（bindingId + 列名），5 个 Record 在序列化
      // 边界收敛为 fieldMeta（键为 bindingId），仅保留非空条目。
      const fieldMeta: Record<string, ChartMeta> = {};
      const assignMeta = (record: Record<string, string>, key: keyof ChartMeta) => {
        for (const [bindingId, value] of Object.entries(record)) {
          if (value) {
            fieldMeta[bindingId] = { ...fieldMeta[bindingId], [key]: value };
          }
        }
      };
      assignMeta(dimensionLabels, 'label');
      assignMeta(metricAggregations, 'aggregation');
      assignMeta(metricAliases, 'alias');
      assignMeta(metricUnits, 'unit');
      assignMeta(metricFormats, 'format');

      const doc: ChartConfigDocument = {
        version: 2,
        chartType: chartBuilderConfig.chartType,
        title: chartBuilderConfig.title,
        query: {
          dimensionGroups: queryConfig.dimensionGroups,
          metricGroups: queryConfig.metricGroups,
          filters: queryConfig.filters,
          sort: queryConfig.sort,
          limit: queryConfig.limit,
        },
        fieldMeta,
        style: chartStyle,
        queryOptions: chartQueryOptions,
      };
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
    setSelectedDatasetId(value);
    setEditingChartId(null);
  };

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
          columnLabels={Object.fromEntries(
            // columnLabels 仍按列名索引（TableChart 按列名取行值）；同一列名有多个
            // 维度 binding 时后写入的 label 覆盖先写入的（Task 0-6+0-8 前的已知歧义）。
            getDimensionFields().map((bound) => [
              bound.field.name,
              dimensionLabels[bound.binding.bindingId] || bound.field.name,
            ])
          )}
          dimensionNames={dimensionFields.map((bound) => bound.field.name)}
          metricNames={metricFields.map((bound) => bound.field.name)}
          rowSize={chartStyle.tableRowSize}
          pagination={chartBuilderConfig.chartType === 'table' ? tablePagination : undefined}
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
            background: '#fff',
            padding: '8px 12px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #f0f0f0',
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
              icon={<FunctionOutlined />}
              onClick={() => setRightDrawerOpen(true)}
              title="配置"
            />
          </div>
        </Header>
      );
    }

    return (
      <Header
        style={{
          background: '#fff',
          padding: '0 16px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid #f0f0f0',
        }}
      >
        <Space>
          <Text strong style={{ fontSize: 16 }}>
            数据集:
          </Text>
          <Select
            style={{ width: 240 }}
            placeholder="选择数据集"
            value={selectedDatasetId}
            onChange={handleDatasetChange}
            allowClear
            options={datasets.map((ds) => ({
              value: ds.id,
              label: ds.name,
            }))}
          />
        </Space>
        <Space>
          <Space>
            <Text type="secondary">自动查询</Text>
            <Switch checked={autoQuery} onChange={toggleAutoQuery} size="small" />
          </Space>
          {!autoQuery && (
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              onClick={handleExecuteQuery}
              disabled={!selectedDatasetId}
            >
              执行查询
            </Button>
          )}
          {!isEmptyPayload(chartData) && (
            <Button icon={<CodeOutlined />} onClick={() => setSqlModalVisible(true)}>
              查看 SQL
            </Button>
          )}
          <Button
            type="primary"
            icon={<SaveOutlined />}
            onClick={handleSave}
            disabled={!selectedDatasetId}
          >
            {editingChartId ? '更新' : '保存'}
          </Button>
          <Button icon={<ReloadOutlined />} onClick={handleReset}>
            重置
          </Button>
        </Space>
      </Header>
    );
  };

  const renderContent = () => {
    if (isMobile) {
      return (
        <Content
          style={{
            padding: '12px',
            background: '#fafafa',
            display: 'flex',
            flexDirection: 'column',
            gap: 12,
          }}
        >
          <Card title="查询配置" size="small" style={{ flex: '0 0 auto' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {renderQueryConfigRows()}

              <FilterBuilder
                fields={chartBuilderFields}
                filters={queryConfig.filters}
                onAdd={() => addFilter()}
                onRemove={removeFilter}
                onUpdate={updateFilter}
              />
            </div>
          </Card>

          <Card title="预览" size="small" style={{ flex: 1, minHeight: 300 }}>
            <div style={{ height: 'calc(100vh - 400px)', minHeight: 250 }}>{renderPreview()}</div>
          </Card>
        </Content>
      );
    }

    return (
      <Layout>
        <Sider
          width={180}
          style={{ background: '#fff', padding: '12px', borderRight: '1px solid #f0f0f0' }}
        >
          <Card title="可用字段" size="small">
            <FieldListPanel fields={chartBuilderFields} loading={chartBuilderFieldsLoading} />
          </Card>
        </Sider>

        <Content
          style={{
            padding: '12px',
            background: '#fafafa',
            display: 'flex',
            flexDirection: 'column',
            gap: 12,
          }}
        >
          <Card title="查询配置" size="small" style={{ flex: '0 0 auto' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {renderQueryConfigRows()}

              <FilterBuilder
                fields={chartBuilderFields}
                filters={queryConfig.filters}
                onAdd={() => addFilter()}
                onRemove={removeFilter}
                onUpdate={updateFilter}
              />
            </div>
          </Card>

          <Card title="预览" size="small" style={{ flex: 1, minHeight: 400 }}>
            <div style={{ height: 'calc(100vh - 480px)', minHeight: 300 }}>{renderPreview()}</div>
          </Card>
        </Content>

        <Sider
          width={260}
          style={{ background: '#fff', padding: '12px', borderLeft: '1px solid #f0f0f0' }}
        >
          <ConfigPanel
            config={chartBuilderConfig}
            dimensionLabels={dimensionLabels}
            metricUnits={metricUnits}
            metricFormats={metricFormats}
            chartStyle={chartStyle}
            dimensionFields={getDimensionFields()}
            metricFields={getMetricFields()}
            onDimensionLabelChange={setDimensionLabel}
            onMetricUnitChange={setMetricUnit}
            onMetricFormatChange={setMetricFormat}
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
      <Layout style={{ minHeight: 'calc(100vh - 120px)' }}>
        {renderHeader()}

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
            dimensionLabels={dimensionLabels}
            metricUnits={metricUnits}
            metricFormats={metricFormats}
            chartStyle={chartStyle}
            dimensionFields={getDimensionFields()}
            metricFields={getMetricFields()}
            onDimensionLabelChange={setDimensionLabel}
            onMetricUnitChange={setMetricUnit}
            onMetricFormatChange={setMetricFormat}
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
        {activeDragField ? <FieldDragPreview field={activeDragField} /> : null}
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
                background: '#f5f5f5',
                padding: 12,
                borderRadius: 4,
                overflow: 'auto',
                maxHeight: 300,
                fontSize: 12,
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
                    background: '#f5f5f5',
                    padding: 12,
                    borderRadius: 4,
                    overflow: 'auto',
                    maxHeight: 200,
                    fontSize: 12,
                  }}
                >
                  {chartQueryResponse.count_sql}
                </pre>
              </>
            )}
          </div>
        )}
      </Modal>
    </DndContext>
  );
};

export default ChartBuilder;
