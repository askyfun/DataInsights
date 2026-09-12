import {
  AppstoreOutlined,
  AreaChartOutlined,
  BarChartOutlined,
  CodeOutlined,
  DotChartOutlined,
  FieldBinaryOutlined,
  FunctionOutlined,
  LineChartOutlined,
  PieChartOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  TableOutlined,
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
import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Chart, ChartQueryAggregation, ChartQueryRequest } from '../api';
import {
  chartDefinitions,
  normalizeQueryConfigForChartType,
} from '../components/ChartBuilder/chartDefinitions';
import DraggableField, { FieldDragPreview } from '../components/ChartBuilder/DraggableField';
import FilterBuilder from '../components/ChartBuilder/FilterBuilder';
import QueryConfigRow from '../components/ChartBuilder/QueryConfigRow';
import TableChart from '../components/ChartBuilder/TableChart';
import {
  type ChartConfigDocument,
  type ChartMeta,
  type ChartType,
  migrateChartConfig,
} from '../lib/chartConfigSchema';
import {
  ChartConfig,
  ChartField,
  ChartQueryOptions,
  ChartStyleConfig,
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
 * 按字段 id 去重，同时保留第一次出现的顺序。
 * 调用场景：右侧配置面板会聚合多个字段组的已选字段，同一字段可能出现在多个组里。
 * 主要逻辑：使用 Set 过滤重复 id，避免 React 列表出现重复 key，同时不影响查询配置原始分组。
 */
const dedupeFieldsById = (fields: ChartField[]): ChartField[] => {
  const seen = new Set<string>();
  return fields.filter((field) => {
    if (seen.has(field.id)) {
      return false;
    }
    seen.add(field.id);
    return true;
  });
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

interface ChartQueryRequestInput {
  datasetId: number;
  chartType: ChartConfig['chartType'];
  queryConfig: QueryConfig;
  fields: ChartField[];
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  tablePagination: { page: number; pageSize: number };
  /** 返回对象是否携带 sort 键。两个自动查询 effect 历史上从不发送 sort， 请求键集合测试也钉死了这一点，统一会让 sort 生效并改变 wire 语义。 */
  includeSort: boolean;
}

/**
 * 图表查询请求的唯一构造出口（纯函数，所有输入经参数传入）。
 * 调用场景：手动执行查询（含排序/翻页覆盖）与两个自动查询 effect 共用。
 * 主要逻辑：按图表定义裁剪字段组、字段 id 映射回列名、组装 dims/metrics/filters/pagination； 维度与指标同时为空时返回 null 表示不发起查询。
 */
const composeChartQueryRequest = (input: ChartQueryRequestInput): ChartQueryRequest | null => {
  const {
    datasetId,
    chartType,
    queryConfig,
    fields,
    metricAggregations,
    metricAliases,
    tablePagination,
    includeSort,
  } = input;

  const activeGroups = getActiveFieldGroups(chartType, queryConfig);
  const fieldMap = new Map(fields.map((f) => [f.id, f]));
  const dimensionFields = activeGroups.dimensionGroups
    .flatMap((group) => group.fields)
    .map((id) => fieldMap.get(id))
    .filter((f): f is ChartField => f !== undefined);
  const metricFields = activeGroups.metricGroups
    .flatMap((group) => group.fields)
    .map((id) => fieldMap.get(id))
    .filter((f): f is ChartField => f !== undefined);

  if (dimensionFields.length === 0 && metricFields.length === 0) {
    return null;
  }

  return {
    dataset_id: datasetId,
    chart_type: chartType,
    dims: dimensionFields.map((f) => f.name),
    metrics: metricFields.map((f) => ({
      field: f.name,
      agg: (metricAggregations[f.id] || 'sum') as ChartQueryAggregation,
      alias: metricAliases[f.id] || f.name,
    })),
    filters: queryConfig.filters.map((f) => {
      const field = fields.find((candidate) => candidate.id === f.field);
      return {
        field: field?.name || f.field,
        operator: f.operator,
        value: f.value,
        value_end: f.valueEnd,
        logic: f.logic,
      };
    }),
    ...(includeSort
      ? {
          sort: queryConfig.sort
            ? { field: queryConfig.sort.field, order: queryConfig.sort.order }
            : undefined,
        }
      : {}),
    pagination:
      chartType === 'table'
        ? {
            page: tablePagination.page,
            page_size: tablePagination.pageSize,
          }
        : undefined,
  };
};

interface ChartCanvasProps {
  config: ChartConfig;
  data: any[];
  loading: boolean;
  dimensionLabels: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  chartStyle: ChartStyleConfig;
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
  const getChartOption = useCallback(() => {
    const { queryConfig, chartBuilderFields } = useStore.getState();
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));

    const dimensionIds = queryConfig.dimensionGroups.flatMap((g) => g.fields);
    const dimensionFields = dimensionIds
      .map((id) => fieldMap.get(id)?.name)
      .filter(Boolean) as string[];

    const metricIds = queryConfig.metricGroups.flatMap((g) => g.fields);
    const metricFields = metricIds.map((id) => fieldMap.get(id)?.name).filter(Boolean) as string[];
    const metricNameMap = new Map(
      metricIds.map((id) => {
        const field = fieldMap.get(id);
        const baseName = field?.name || id;
        const alias = metricAliases[id];
        const unit = metricUnits[id];
        const displayName = alias || baseName;
        return [baseName, unit ? `${displayName} (${unit})` : displayName];
      })
    );

    // scatter only needs 2 metrics, dims are optional
    const isScatter = config.chartType === 'scatter';
    if (
      (!isScatter && dimensionFields.length === 0) ||
      metricFields.length === 0 ||
      data.length === 0
    ) {
      return null;
    }

    const xAxisField = dimensionFields[0];
    const xAxisData = data.map((item) => item[xAxisField]);

    const commonOptions = {
      title: {
        text: config.title,
        left: 'center',
      },
      tooltip: {
        trigger: 'axis',
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        containLabel: true,
      },
    };

    // 多维度时 series key 是维度值，单维度时是指标名
    const dataKeys = data.length > 0 ? Object.keys(data[0]) : [];
    const seriesFields = dataKeys.filter((k) => k !== xAxisField);
    const effectiveSeries = seriesFields.length > 0 ? seriesFields : metricFields;

    switch (config.chartType) {
      case 'line':
        return {
          ...commonOptions,
          xAxis: {
            type: 'category',
            data: xAxisData,
            name: dimensionLabels[queryConfig.dimensionGroups[0]?.fields[0] || ''] || xAxisField,
            axisLabel: {
              rotate: xAxisData.length > 10 ? 30 : 0,
              formatter: (val: string) => {
                // 截断 "+0000 UTC" 后缀，保留日期+时间
                return val.replace(/\s*\+\d{4}\s*UTC$/, '');
              },
            },
          },
          yAxis: {
            type: 'value',
          },
          series: effectiveSeries.map((yField) => ({
            name: metricNameMap.get(yField) || yField,
            type: 'line',
            smooth: chartStyle.smooth,
            connectNulls: true,
            data: data.map((item) => item[yField] ?? null),
          })),
          color: chartStyle.colors.length > 0 ? chartStyle.colors : undefined,
        };

      case 'bar':
        return {
          ...commonOptions,
          xAxis: {
            type: 'category',
            data: xAxisData,
            axisLabel: {
              rotate: xAxisData.length > 10 ? 30 : 0,
              formatter: (val: string) => val.replace(/\s*\+\d{4}\s*UTC$/, ''),
            },
          },
          yAxis: {
            type: 'value',
          },
          series: effectiveSeries.map((yField) => ({
            name: metricNameMap.get(yField) || yField,
            type: 'bar',
            data: data.map((item) => item[yField] ?? null),
          })),
          color: chartStyle.colors.length > 0 ? chartStyle.colors : undefined,
        };

      case 'pie': {
        const pieData = data.map((item) => ({
          name: item[xAxisField] || item.name,
          value: item[metricFields[0] || ''] || item.value,
        }));
        return {
          ...commonOptions,
          tooltip: {
            trigger: 'item',
            formatter: '{b}: {c} ({d}%)',
          },
          legend: {
            orient: 'vertical',
            left: 'left',
          },
          series: [
            {
              name: metricFields[0] || 'Value',
              type: 'pie',
              radius: '50%',
              data: pieData,
              emphasis: {
                itemStyle: {
                  shadowBlur: 10,
                  shadowOffsetX: 0,
                  shadowColor: 'rgba(0, 0, 0, 0.5)',
                },
              },
              label: {
                formatter: '{b}: {d}%',
              },
            },
          ],
          color: chartStyle.colors.length > 0 ? chartStyle.colors : undefined,
        };
      }

      case 'area':
        return {
          ...commonOptions,
          xAxis: {
            type: 'category',
            data: xAxisData,
            axisLabel: {
              rotate: xAxisData.length > 10 ? 30 : 0,
              formatter: (val: string) => val.replace(/\s*\+\d{4}\s*UTC$/, ''),
            },
          },
          yAxis: {
            type: 'value',
          },
          series: effectiveSeries.map((yField) => ({
            name: metricNameMap.get(yField) || yField,
            type: 'line',
            areaStyle: {},
            smooth: chartStyle.smooth,
            connectNulls: true,
            data: data.map((item) => item[yField] ?? null),
          })),
          color: chartStyle.colors.length > 0 ? chartStyle.colors : undefined,
        };

      case 'scatter':
        return {
          ...commonOptions,
          series: [
            {
              type: 'scatter',
              // ScatterResponse.data is [number, number][] — access by index
              data: data.map((item) => [item[0], item[1]]),
            },
          ],
        };

      default:
        return null;
    }
  }, [
    chartStyle.colors,
    chartStyle.smooth,
    config,
    data,
    dimensionLabels,
    metricAliases,
    metricUnits,
  ]);

  const chartOption = getChartOption();

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
  dimensionFields: ChartField[];
  metricFields: ChartField[];
  onDimensionLabelChange: (fieldId: string, label: string) => void;
  onMetricUnitChange: (fieldId: string, unit: string) => void;
  onMetricFormatChange: (fieldId: string, format: string) => void;
  onChartStyleChange: (style: Partial<ChartStyleConfig>) => void;
}

const chartTypeOptions = [
  { type: 'table', icon: <TableOutlined />, label: chartDefinitions.table.label },
  { type: 'bar', icon: <BarChartOutlined />, label: chartDefinitions.bar.label },
  { type: 'line', icon: <LineChartOutlined />, label: chartDefinitions.line.label },
  { type: 'pie', icon: <PieChartOutlined />, label: chartDefinitions.pie.label },
  { type: 'area', icon: <AreaChartOutlined />, label: chartDefinitions.area.label },
  { type: 'scatter', icon: <DotChartOutlined />, label: chartDefinitions.scatter.label },
  { type: 'pivot', icon: <AppstoreOutlined />, label: chartDefinitions.pivot.label },
] as const;

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
}) => {
  return (
    <div>
      <Card title="可视化类型" size="small" style={{ marginBottom: 12 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 8 }}>
          {chartTypeOptions.map((opt) => (
            <Button
              key={opt.type}
              type={config.chartType === opt.type ? 'primary' : 'default'}
              icon={opt.icon}
              onClick={() => onConfigChange({ chartType: opt.type })}
              style={{ height: 40 }}
            >
              {opt.label}
            </Button>
          ))}
        </div>
      </Card>

      <Card title="图表配置" size="small" style={{ marginBottom: 12 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          <div>
            <Text strong>图表标题</Text>
            <Input
              style={{ width: '100%', marginTop: 4 }}
              value={config.title}
              onChange={(e) => onConfigChange({ title: e.target.value })}
              placeholder="输入图表标题"
            />
          </div>

          <div>
            <Text strong>平滑曲线</Text>
            <div style={{ marginTop: 4 }}>
              <Switch
                checked={chartStyle.smooth}
                onChange={(checked) => onChartStyleChange({ smooth: checked })}
                disabled={config.chartType !== 'line' && config.chartType !== 'area'}
              />
            </div>
          </div>

          <div>
            <Text strong>主色</Text>
            <div style={{ marginTop: 4 }}>
              <ColorPicker
                value={chartStyle.colors[0] || '#1677ff'}
                onChange={(color) => onChartStyleChange({ colors: [color.toHexString()] })}
              />
            </div>
          </div>

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
        </Space>
      </Card>

      {dimensionFields.length > 0 && (
        <Card title="维度属性" size="small" style={{ marginBottom: 12 }}>
          <Space direction="vertical" style={{ width: '100%' }} size="small">
            {dimensionFields.map((field) => (
              <div key={field.id}>
                <Text strong>{field.name}</Text>
                <Input
                  style={{ width: '100%', marginTop: 4 }}
                  value={dimensionLabels[field.id] || ''}
                  onChange={(e) => onDimensionLabelChange(field.id, e.target.value)}
                  placeholder="显示名称"
                />
              </div>
            ))}
          </Space>
        </Card>
      )}

      {metricFields.length > 0 && (
        <Card title="指标属性" size="small" style={{ marginBottom: 12 }}>
          <Space direction="vertical" style={{ width: '100%' }} size="small">
            {metricFields.map((field) => (
              <div key={field.id}>
                <Text strong>{field.name}</Text>
                <Input
                  style={{ width: '100%', marginTop: 4, marginBottom: 4 }}
                  value={metricUnits[field.id] || ''}
                  onChange={(e) => onMetricUnitChange(field.id, e.target.value)}
                  placeholder="单位，例如 元 / %"
                />
                <Input
                  style={{ width: '100%' }}
                  value={metricFormats[field.id] || ''}
                  onChange={(e) => onMetricFormatChange(field.id, e.target.value)}
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

  const getDimensionFields = useCallback(() => {
    const dimensionIds = queryConfig.dimensionGroups.flatMap((g) => g.fields);
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return dimensionIds.map((id) => fieldMap.get(id)).filter(Boolean) as ChartField[];
  }, [queryConfig.dimensionGroups, chartBuilderFields]);

  const getMetricFields = useCallback(() => {
    const metricIds = queryConfig.metricGroups.flatMap((g) => g.fields);
    const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
    return metricIds.map((id) => fieldMap.get(id)).filter(Boolean) as ChartField[];
  }, [queryConfig.metricGroups, chartBuilderFields]);

  /**
   * 按维度组索引读取字段，供定义驱动的查询配置面板复用。
   * 调用场景：一个图表类型需要多个维度组时，例如透视表的行/列维度。
   * 主要逻辑：从指定 group 的 field id 列表映射回完整字段对象。
   */
  const getDimensionFieldsByGroup = useCallback(
    (groupIndex: number) => {
      const fieldIds = queryConfig.dimensionGroups[groupIndex]?.fields || [];
      const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
      return fieldIds.map((id) => fieldMap.get(id)).filter(Boolean) as ChartField[];
    },
    [queryConfig.dimensionGroups, chartBuilderFields]
  );

  /**
   * 按指标组索引读取字段，供定义驱动的查询配置面板复用。
   * 调用场景：一个图表类型需要多个指标组时，例如散点图的 X/Y 指标。
   * 主要逻辑：从指定 group 的 field id 列表映射回完整字段对象。
   */
  const getMetricFieldsByGroup = useCallback(
    (groupIndex: number) => {
      const fieldIds = queryConfig.metricGroups[groupIndex]?.fields || [];
      const fieldMap = new Map(chartBuilderFields.map((f) => [f.id, f]));
      return fieldIds.map((id) => fieldMap.get(id)).filter(Boolean) as ChartField[];
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
          onRemoveField={(fieldId) =>
            group.kind === 'dimension'
              ? removeDimensionField(fieldId, groupIndex)
              : removeMetricField(fieldId, groupIndex)
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
              ? (field) => {
                  const alias = prompt('输入字段别名:', field.name);
                  if (alias !== null) {
                    setMetricAlias(field.id, alias);
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

  const buildChartQueryRequest = useCallback((): ChartQueryRequest | null => {
    if (!selectedDatasetId) return null;

    return composeChartQueryRequest({
      datasetId: selectedDatasetId,
      chartType: chartBuilderConfig.chartType,
      queryConfig,
      fields: chartBuilderFields,
      metricAggregations,
      metricAliases,
      tablePagination: { page: tablePagination.page, pageSize: tablePagination.pageSize },
      includeSort: true,
    });
  }, [
    selectedDatasetId,
    metricAggregations,
    metricAliases,
    queryConfig,
    chartBuilderConfig.chartType,
    tablePagination.page,
    tablePagination.pageSize,
    chartBuilderFields,
  ]);

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
      setQueryConfig({ sort });
      const request = buildChartQueryRequest();
      if (request) {
        request.sort = sort;
        executeChartQuery(request);
      }
    },
    [buildChartQueryRequest, executeChartQuery, setQueryConfig]
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
      includeSort: false,
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
      includeSort: false,
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
                    field: doc.query.sort.field,
                    order: doc.query.sort.order === 'desc' ? 'desc' : 'asc',
                  }
                : undefined,
              limit: doc.query.limit,
            })
          );

          // v1 fieldMeta（键为列名）→ 5 个运行时 Record
          const restoredLabels: Record<string, string> = {};
          const restoredAggregations: Record<string, string> = {};
          const restoredAliases: Record<string, string> = {};
          const restoredUnits: Record<string, string> = {};
          const restoredFormats: Record<string, string> = {};
          for (const [name, meta] of Object.entries(doc.fieldMeta)) {
            if (meta.label) restoredLabels[name] = meta.label;
            if (meta.aggregation) restoredAggregations[name] = meta.aggregation;
            if (meta.alias) restoredAliases[name] = meta.alias;
            if (meta.unit) restoredUnits[name] = meta.unit;
            if (meta.format) restoredFormats[name] = meta.format;
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
      // v1 持久化文档：字段组已是列名（fieldId === name），
      // 5 个平铺 Record 在序列化边界收敛为 fieldMeta（键为列名），仅保留非空条目。
      const fieldMeta: Record<string, ChartMeta> = {};
      const assignMeta = (record: Record<string, string>, key: keyof ChartMeta) => {
        for (const [name, value] of Object.entries(record)) {
          if (value) {
            fieldMeta[name] = { ...fieldMeta[name], [key]: value };
          }
        }
      };
      assignMeta(dimensionLabels, 'label');
      assignMeta(metricAggregations, 'aggregation');
      assignMeta(metricAliases, 'alias');
      assignMeta(metricUnits, 'unit');
      assignMeta(metricFormats, 'format');

      const doc: ChartConfigDocument = {
        version: 1,
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
    if (chartBuilderConfig.chartType === 'table' || chartBuilderConfig.chartType === 'pivot') {
      const dimensionFields = getDimensionFields();
      const metricFields = getMetricFields();
      return (
        <TableChart
          data={chartData}
          loading={chartDataLoading}
          columns={tableColumns}
          columnLabels={Object.fromEntries(
            getDimensionFields().map((field) => [
              field.name,
              dimensionLabels[field.id] || field.name,
            ])
          )}
          dimensionNames={dimensionFields.map((f) => f.name)}
          metricNames={metricFields.map((f) => f.name)}
          rowSize={chartStyle.tableRowSize}
          pagination={chartBuilderConfig.chartType === 'table' ? tablePagination : undefined}
          onPageChange={chartBuilderConfig.chartType === 'table' ? handlePageChange : undefined}
          onSortChange={chartBuilderConfig.chartType === 'table' ? handleSortChange : undefined}
        />
      );
    }
    return (
      <ChartCanvas
        config={chartBuilderConfig}
        data={chartData}
        loading={chartDataLoading}
        dimensionLabels={dimensionLabels}
        metricAliases={metricAliases}
        metricUnits={metricUnits}
        chartStyle={chartStyle}
      />
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
              disabled={chartData.length === 0}
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
          {chartData.length > 0 && (
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
            dimensionFields={dedupeFieldsById(getDimensionFields())}
            metricFields={dedupeFieldsById(getMetricFields())}
            onDimensionLabelChange={setDimensionLabel}
            onMetricUnitChange={setMetricUnit}
            onMetricFormatChange={setMetricFormat}
            onChartStyleChange={setChartStyle}
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
          width={300}
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
          width={300}
        >
          <ConfigPanel
            config={chartBuilderConfig}
            dimensionLabels={dimensionLabels}
            metricUnits={metricUnits}
            metricFormats={metricFormats}
            chartStyle={chartStyle}
            dimensionFields={dedupeFieldsById(getDimensionFields())}
            metricFields={dedupeFieldsById(getMetricFields())}
            onDimensionLabelChange={setDimensionLabel}
            onMetricUnitChange={setMetricUnit}
            onMetricFormatChange={setMetricFormat}
            onChartStyleChange={setChartStyle}
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
