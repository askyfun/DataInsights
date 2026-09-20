/**
 * 查询记录的 spec_json 读写边界（纯逻辑，不依赖组件运行时）。
 *
 * 一条查询记录（bi_query.spec_json）的结构是 `{ v: 1, document: ChartConfigDocument }`：
 * 外层 v 管记录级 schema 演进，内层 document 就是图表保存时用的同一份 v2 文档。
 * 复用同一份文档是刻意的——「保存成图表」与「分享地址栏」必须还原出逐字段一致的
 * 配置，两套序列化逻辑迟早会漂移；同时 document 自带 version:2 与
 * migrateChartConfig 迁移函数，旧记录不需要在这里再写一次兼容分支。
 */
import type { ChartConfig, ChartQueryOptions, ChartStyleConfig, QueryConfig } from '../store';
import { type ChartConfigDocument, type ChartMeta, migrateChartConfig } from './chartConfigSchema';

/** 记录级 spec 版本；与后端 entity.CurrentQuerySpecVersion 必须一致。 */
export const CURRENT_QUERY_SPEC_VERSION = 1;

/** bi_query.spec_json 的顶层信封（对应后端 entity.QuerySpecEnvelope）。 */
export interface QuerySpecDocument {
  v: typeof CURRENT_QUERY_SPEC_VERSION;
  document: ChartConfigDocument;
}

/** 组装图表配置文档所需的全部运行时状态（与保存图表时的口径完全一致）。 */
export interface ChartConfigDocumentInput {
  chartType: ChartConfig['chartType'];
  title: string;
  /**
   * 全量查询配置：filters / sort / limit 的来源。过滤与排序是图型无关的公共语义，
   * 不随槽位裁剪而变，因此取全量而非裁剪后的值。
   */
  queryConfig: QueryConfig;
  /**
   * 已按当前图型裁剪过的字段组（getActiveFieldGroups 的结果）：**只有字段组**取它。
   * 隐藏槽位的绑定一旦落库，ShareView / GetData 的平铺路径会把它们当真实维度发出去
   * （表现为重复列）。裁剪结果天然没有 filters/sort，所以类型取 QueryConfig 的子集。
   */
  activeQueryConfig: Pick<QueryConfig, 'dimensionGroups' | 'metricGroups'>;
  dimensionLabels: Record<string, string>;
  metricAggregations: Record<string, string>;
  metricAliases: Record<string, string>;
  metricUnits: Record<string, string>;
  metricFormats: Record<string, string>;
  chartStyle: ChartStyleConfig;
  chartQueryOptions: ChartQueryOptions;
}

/**
 * 把运行时状态序列化为 v2 图表配置文档。
 *
 * 5 个平铺 Record 在序列化边界收敛为 fieldMeta（键为 bindingId），空值条目丢弃：
 * 这是 bi_chart.config 与 bi_query.spec_json 共用的唯一出口。
 */
export function buildChartConfigDocument(input: ChartConfigDocumentInput): ChartConfigDocument {
  const fieldMeta: Record<string, ChartMeta> = {};
  const assignMeta = (record: Record<string, string>, key: keyof ChartMeta) => {
    for (const [bindingId, value] of Object.entries(record)) {
      if (value) {
        fieldMeta[bindingId] = { ...fieldMeta[bindingId], [key]: value };
      }
    }
  };
  assignMeta(input.dimensionLabels, 'label');
  assignMeta(input.metricAggregations, 'aggregation');
  assignMeta(input.metricAliases, 'alias');
  assignMeta(input.metricUnits, 'unit');
  assignMeta(input.metricFormats, 'format');

  return {
    version: 2,
    chartType: input.chartType,
    title: input.title,
    query: {
      dimensionGroups: input.activeQueryConfig.dimensionGroups,
      metricGroups: input.activeQueryConfig.metricGroups,
      filters: input.queryConfig.filters,
      sort: input.queryConfig.sort,
      limit: input.queryConfig.limit,
    },
    fieldMeta,
    style: input.chartStyle,
    queryOptions: input.chartQueryOptions,
  };
}

/** 组装一份可直接提交给 POST /api/queries 的记录信封。 */
export function buildQuerySpecDocument(input: ChartConfigDocumentInput): QuerySpecDocument {
  return { v: CURRENT_QUERY_SPEC_VERSION, document: buildChartConfigDocument(input) };
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * 把服务端返回的 spec 解析为合法的 v2 配置文档；信封不合法时返回 null。
 *
 * 文档内层一律经 migrateChartConfig 读取：它不抛异常，任何损坏输入都退化成一份
 * 合法空文档，所以这里只需挡住信封本身（版本不符 / document 不是对象）。
 */
export function parseQuerySpecDocument(spec: unknown): ChartConfigDocument | null {
  if (!isPlainObject(spec)) {
    return null;
  }
  if (spec.v !== CURRENT_QUERY_SPEC_VERSION) {
    return null;
  }
  if (!isPlainObject(spec.document)) {
    return null;
  }
  // fallbackType 仅在文档 chartType 缺失/非法时生效；正常记录不会走到它
  return migrateChartConfig(JSON.stringify(spec.document), 'table');
}
