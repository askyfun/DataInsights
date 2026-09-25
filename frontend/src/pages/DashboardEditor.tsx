import {
  ArrowLeftOutlined,
  DashboardOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import {
  Alert,
  App,
  Button,
  Card,
  Empty,
  Input,
  Result,
  Select,
  Space,
  Spin,
  Typography,
} from 'antd';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
// react-grid-layout v2：主入口没有 WidthProvider（那是 v1 HOC），改用
// useContainerWidth()；栅格参数走分组 props（gridConfig / dragConfig / resizeConfig），
// compactType="vertical" 已改为 compactor={verticalCompactor}。
// 样式只需 RGL 自己的那个文件——v2 已把 react-resizable 的手柄样式合并进去，
// 而 react-resizable 未被 hoist 到 node_modules 根，单独 import 它的 css 会解析失败。
import { GridLayout, type Layout, useContainerWidth, verticalCompactor } from 'react-grid-layout';
import { useIntl } from 'react-intl';
import { useNavigate, useParams } from 'react-router-dom';
import 'react-grid-layout/css/styles.css';

import {
  type Chart,
  type ChartDataResponse,
  chartsApi,
  type Dashboard,
  type DashboardQueryResult,
  dashboardsApi,
  datasetsApi,
} from '../api';
import ChartView from '../components/ChartView/ChartView';
import AddFilterWidgetModal, {
  type NewFilterWidgetConfig,
} from '../components/DashboardFilterBlock/AddFilterWidgetModal';
import DashboardFilterBlock from '../components/DashboardFilterBlock/DashboardFilterBlock';
import DateFilterModal, {
  type DateFilterModalPayload,
} from '../components/DateFilter/DateFilterModal';
import PageHeader from '../components/PageHeader';
import {
  type DashboardQueryFilterValue,
  dashboardFiltersPayload,
  dateFilterWidgetSettings,
  filterWidgetFamily,
  initialFilterWidgetValues,
} from '../lib/dashboardFilterValue';
import {
  createWidgetId,
  DASHBOARD_DEFAULT_SIZE,
  DASHBOARD_GRID_COLS,
  DASHBOARD_MIN_H,
  DASHBOARD_MIN_W,
  type DashboardChartWidget,
  type DashboardFilterWidget,
  type DashboardLayoutDocument,
  type DashboardWidget,
  findFreePlacement,
  migrateDashboardLayout,
  normalizePlacement,
  serializeDashboardLayout,
} from '../lib/dashboardLayoutSchema';
import type { DateGranularity, WeekStart } from '../lib/dateFilter';
import { isDateFilterValue } from '../lib/dateFilter';
import { useStore } from '../store';

const { Text } = Typography;

/** 挑出布局里的筛选器块（类型守卫版本，多处复用）。 */
function filterWidgetsOf(widgets: readonly DashboardWidget[]): DashboardFilterWidget[] {
  return widgets.filter((widget): widget is DashboardFilterWidget => widget.type === 'filter');
}

/** 草稿 key：PRD §11-8 的「key 命名」在本批定为 `<前缀><dashboardId>`。 */
const DRAFT_PREFIX = 'dashboard-draft:';

interface DashboardDraft {
  name?: string;
  layout: DashboardLayoutDocument;
}

/** 读取本地草稿。坏内容一律当没有草稿处理（返回 null），绝不抛异常。 */
function readDraft(raw: string): DashboardDraft | null {
  try {
    const parsed = JSON.parse(raw) as { name?: unknown; layout?: unknown };
    if (typeof parsed.layout !== 'string') {
      return null;
    }
    return {
      layout: migrateDashboardLayout(parsed.layout),
      name: typeof parsed.name === 'string' ? parsed.name : undefined,
    };
  } catch {
    return null;
  }
}

/**
 * 从盘级取数结果的 `data` 里取出图表负载。
 *
 * 该字段是**完整的 ChartDataResult**（`{ data, select_sql, count_sql? }`），
 * 与 `GET /api/charts/{id}/data` 的信封同形，故内层 `data` 才是 ChartView 要的形状
 * （对齐 `frontend/src/api/index.ts` 文件头关于「Response 是信封包装」的迁移规则）。
 */
function extractChartData(raw: unknown): ChartDataResponse | null {
  if (raw && typeof raw === 'object' && 'data' in raw) {
    return (raw as { data: ChartDataResponse }).data;
  }
  return null;
}

const BlockPlaceholder: React.FC<{
  status: 'warning' | 'error';
  title: string;
  description?: string;
}> = ({ status, title, description }) => (
  <div
    style={{
      height: '100%',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      overflow: 'hidden',
    }}
  >
    <Result status={status} title={title} subTitle={description} />
  </div>
);

const DashboardEditor: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const { width, mounted, containerRef } = useContainerWidth();
  // message 取 App 上下文实例，不用静态 message.*：静态方法读不到 ConfigProvider 的
  // 主题上下文，antd 6 会打 "Static function can not consume context like dynamic theme"。
  // 依赖根部的 <App> 包裹（见 main.tsx）——没有它 useApp() 拿到的是空对象。
  const { message } = App.useApp();
  // 盘级筛选器要绑数据集字段，故这里需要数据集清单（与数据集页共用同一份 store 状态）。
  const datasets = useStore((state) => state.datasets);
  const fetchDatasets = useStore((state) => state.fetchDatasets);

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [doc, setDoc] = useState<DashboardLayoutDocument>(() => migrateDashboardLayout(''));
  const [baseline, setBaseline] = useState<{ name: string; layout: string } | null>(null);
  const [charts, setCharts] = useState<Chart[]>([]);
  const [chartsById, setChartsById] = useState<Record<number, Chart>>({});
  /** 块引用图表的数据集列 ID→列名；ChartView 据此把列 ID 换成展示名。 */
  const [fieldNames, setFieldNames] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, DashboardQueryResult>>({});
  const [pendingData, setPendingData] = useState<Record<string, ChartDataResponse | null>>({});
  const [querying, setQuerying] = useState(false);
  const [saving, setSaving] = useState(false);
  const [draftRestored, setDraftRestored] = useState(false);
  /**
   * 盘级筛选器的**当前取值**（widgetId → 日期筛选值）。刻意不写进 layout：
   * `defaultValue` 是「默认选中值」，取值本身由这里在会话内持有，改完立即重取。
   */
  // 取值按原始类型存：日期族是 DateFilterValue，字符串/数值族是 unknown[]。
  const [filterValues, setFilterValues] = useState<Record<string, unknown>>({});
  const [addFilterOpen, setAddFilterOpen] = useState(false);
  const [configuringFilterId, setConfiguringFilterId] = useState<string | null>(null);
  /** 未持久化块的本地取数在途集合：防止 effect 重跑时对同一块重复发请求。 */
  const inflightRef = useRef<Set<string>>(new Set());
  /**
   * 已落库的 widgetId 集合。
   *
   * 这些块的取数由 `/query` 负责，本地取数必须**完全跳过**：`/query` 是异步的，
   * 在它 settle 之前 React 会先提交一帧 `results = {}`，若本地取数的判定只看 `results`，
   * 每个已落库的块都会抢先多发一次 `GET /api/charts/{id}/data`——这在真实网络下必然发生，
   * 而测试里「即 resolve 的 mock」会把它掩盖掉。
   */
  const persistedIdsRef = useRef<Set<string>>(new Set());

  /**
   * 盘级取数。
   *
   * `filters` 是筛选器的**当前取值**（不是合并结果）：合并由后端单点完成（PRD §6.3），
   * 前端只负责把「哪个筛选器现在选了什么」如实下发，形状规则见 `lib/dashboardFilterValue`。
   */
  const runQuery = useCallback(
    async (dashboardId: string, filters: DashboardQueryFilterValue[]) => {
      setQuerying(true);
      try {
        const response = await dashboardsApi.query(dashboardId, { filters });
        const map: Record<string, DashboardQueryResult> = {};
        for (const result of response.data.data.results ?? []) {
          map[result.widgetId] = result;
        }
        setResults(map);
      } catch (error: any) {
        message.error(error.message || intl.formatMessage({ id: 'common.error' }));
      } finally {
        setQuerying(false);
      }
    },
    // message 实例跨渲染稳定（antd 内部 useMemo([], …)），进依赖数组不会引起循环。
    [intl, message]
  );

  /**
   * 按给定的筛选器状态重取一整盘。
   *
   * 只把**已落库**的筛选器下发出去：后端是按已落库的 layout 逐块取数、并按同一份 layout
   * 建筛选器索引的，未保存的筛选器它根本认不出来 —— 发了也只是白跑一趟。
   * 调用方传显式的 widgets/values 而不是读组件状态：改筛选取值、改粒度这些场景下
   * 新状态还没落进 state，读旧值会算出上一轮的载荷。
   */
  const queryWithFilters = useCallback(
    (dashboardId: string, widgets: readonly DashboardWidget[], values: Record<string, unknown>) => {
      const active = filterWidgetsOf(widgets).filter((widget) =>
        persistedIdsRef.current.has(widget.widgetId)
      );
      return runQuery(dashboardId, dashboardFiltersPayload(active, values));
    },
    [runQuery]
  );

  const load = useCallback(
    async (dashboardId: string) => {
      setLoading(true);
      setLoadError(null);
      try {
        const [detail, chartList] = await Promise.all([
          dashboardsApi.getById(dashboardId),
          chartsApi.getAll(),
        ]);
        const dashboard: Dashboard = detail.data.data;
        const allCharts = chartList.data.data ?? [];
        const byId: Record<number, Chart> = {};
        for (const chart of allCharts) {
          byId[chart.id] = chart;
        }
        setCharts(allCharts);
        setChartsById(byId);
        setResults({});
        setPendingData({});

        // 列 ID→列名映射：ChartView 的 labelOf 靠它把配置里的列 ID 换成展示名。
        // 缺了它，combo 分支的 xAxis.name 会把裸列 ID 画进坐标轴（浏览器验收缺陷 1）。
        // 按块引用的图表去重的数据集拉取，单个数据集失败不阻断盘装载（回落空映射，
        // ChartView 对未命中字段原样输出，行为与修复前一致）。
        const serverDoc = migrateDashboardLayout(dashboard.layout_json);
        const datasetIds = [
          ...new Set(
            serverDoc.widgets.flatMap((w) =>
              w.type === 'chart' ? [byId[w.chartId]?.dataset_id] : []
            )
          ),
        ].filter((id): id is number => typeof id === 'number');
        if (datasetIds.length > 0) {
          void Promise.all(
            datasetIds.map((id) =>
              datasetsApi
                .getColumns(id)
                .then((r) => r.data.data ?? [])
                .catch(() => [])
            )
          ).then((columnGroups) => {
            const names: Record<string, string> = {};
            for (const columns of columnGroups) {
              for (const column of columns) {
                names[column.id] = column.name;
              }
            }
            setFieldNames(names);
          });
        } else {
          setFieldNames({});
        }

        const serverLayout = serializeDashboardLayout(serverDoc);
        persistedIdsRef.current = new Set(serverDoc.widgets.map((widget) => widget.widgetId));

        // 本地草稿只在「真的与后端不一致」时恢复；一致就顺手清掉，
        // 避免一条陈旧草稿长期驻留、下次打开又被判定为「未保存改动」。
        //
        // ⚠️「不一致」的判定必须与写草稿侧的 `dirty` **对称**：`dirty` 同时看 name 与
        // layout，这里若只比 layout，「只改了名字」的草稿会被判成"与后端一致"而走删除
        // 分支 —— 用户的未保存改名静默丢失（浏览器验收 D-1）。
        let nextDoc = serverDoc;
        let nextName = dashboard.name;
        let restored = false;
        const draftRaw = localStorage.getItem(DRAFT_PREFIX + dashboardId);
        if (draftRaw) {
          const draft = readDraft(draftRaw);
          const draftName = draft?.name ?? dashboard.name;
          const keep =
            draft !== null &&
            (draftName !== dashboard.name ||
              serializeDashboardLayout(draft.layout) !== serverLayout);
          if (draft && keep) {
            nextDoc = draft.layout;
            nextName = draftName;
            restored = true;
          } else {
            localStorage.removeItem(DRAFT_PREFIX + dashboardId);
          }
        }

        setDoc(nextDoc);
        setName(nextName);
        setBaseline({ name: dashboard.name, layout: serverLayout });
        setDraftRestored(restored);

        // 筛选器初值取布局里落库的「默认选中值」：控件与首屏取数用同一份，
        // 否则会出现"控件显示最近 7 天、实际查的是全量"这种不一致。
        const seeded = initialFilterWidgetValues(filterWidgetsOf(nextDoc.widgets));
        setFilterValues(seeded);
        await queryWithFilters(dashboardId, nextDoc.widgets, seeded);
      } catch (error: any) {
        setLoadError(error.message || intl.formatMessage({ id: 'dashboard.loadFailed' }));
      } finally {
        setLoading(false);
      }
    },
    [intl, queryWithFilters]
  );

  useEffect(() => {
    if (id) {
      load(id);
    }
  }, [id, load]);

  // 数据集清单只服务于「添加筛选器」的字段选择器；空列表时拉一次即可。
  useEffect(() => {
    if (datasets.length === 0) {
      void fetchDatasets();
    }
  }, [datasets.length, fetchDatasets]);

  const dirty = useMemo(() => {
    if (!baseline) {
      return false;
    }
    return name !== baseline.name || serializeDashboardLayout(doc) !== baseline.layout;
  }, [baseline, doc, name]);

  // 编辑期防丢：只在 dirty 时落草稿；回到基线或保存/丢弃时清掉（真相源恒为后端）。
  useEffect(() => {
    if (!id || !baseline) {
      return;
    }
    const key = DRAFT_PREFIX + id;
    if (!dirty) {
      // 改动被撤销回基线时连同草稿一起清掉，否则下次打开会拿一条看似更旧的内容来比对。
      localStorage.removeItem(key);
      return;
    }
    localStorage.setItem(key, JSON.stringify({ name, layout: serializeDashboardLayout(doc) }));
  }, [id, baseline, dirty, doc, name]);

  /**
   * 未持久化块的本地取数。
   *
   * 盘级 `/query` 是**后端按已落库的 layout 逐块取数**，所以刚拖进来、还没保存的块
   * 拿不到结果。v1 的 `filters` 恒为空数组，此时图表负载与「带盘级筛选」的结果完全等价，
   * 故直接用图表自身取数端点补上，避免"加进来却一片空白、必须保存才看得见"。
   * 保存后 `/query` 的结果优先（见 renderChartBlock 的判定顺序）。
   */
  useEffect(() => {
    if (!id) {
      return;
    }
    for (const widget of doc.widgets) {
      if (widget.type !== 'chart') {
        continue;
      }
      if (persistedIdsRef.current.has(widget.widgetId)) {
        continue;
      }
      if (results[widget.widgetId] || widget.widgetId in pendingData) {
        continue;
      }
      if (inflightRef.current.has(widget.widgetId)) {
        continue;
      }
      inflightRef.current.add(widget.widgetId);
      chartsApi
        .getChartData(widget.chartId)
        .then((response) => {
          setPendingData((prev) => ({ ...prev, [widget.widgetId]: response.data.data }));
        })
        .catch(() => {
          // null = 取数失败（与"键不存在 = 仍在取"区分开）。
          setPendingData((prev) => ({ ...prev, [widget.widgetId]: null }));
        })
        .finally(() => {
          inflightRef.current.delete(widget.widgetId);
        });
    }
  }, [id, doc.widgets, results, pendingData]);

  const layout = useMemo<Layout>(
    () =>
      doc.widgets.map((widget) => ({
        i: widget.widgetId,
        x: widget.x,
        y: widget.y,
        w: widget.w,
        h: widget.h,
        minW: DASHBOARD_MIN_W,
        minH: DASHBOARD_MIN_H,
      })),
    [doc.widgets]
  );

  /**
   * 把拖拽/缩放的结果写回文档。
   *
   * 刻意**不订阅 `onLayoutChange`**：那一个回调在挂载时也会触发，而 verticalCompactor
   * 会把落库布局里不紧凑的块向上吸附 —— 若把这次规范化当成用户改动，打开一个盘就会
   * 立刻显示"有未保存的改动"并写出本地草稿。改为只认拖拽/缩放结束这两次真实手势
   * （两者的首参都是**完整布局**，碰撞下推的邻块也在其中，故一次性同步全部宫格）。
   *
   * 手势结束后仍比较四轴：没变就原样返回旧对象，让 React 跳过这次渲染。
   */
  const handleLayoutCommit = useCallback((nextLayout: Layout) => {
    setDoc((prev) => {
      let changed = false;
      const widgets = prev.widgets.map((widget) => {
        const item = nextLayout.find((entry) => entry.i === widget.widgetId);
        if (!item) {
          return widget;
        }
        const placement = normalizePlacement(item, DASHBOARD_DEFAULT_SIZE[widget.type]);
        if (
          placement.x === widget.x &&
          placement.y === widget.y &&
          placement.w === widget.w &&
          placement.h === widget.h
        ) {
          return widget;
        }
        changed = true;
        return { ...widget, ...placement };
      });
      return changed ? { ...prev, widgets } : prev;
    });
  }, []);

  const handleAddChart = useCallback((chartId: number) => {
    setDoc((prev) => {
      const size = DASHBOARD_DEFAULT_SIZE.chart;
      // 放进首个空位（自上而下、自左而右），而不是一律 `x: 0` 落在最底边 —— 否则 12 列
      // 画布上默认 6 列宽的块会全部堆在左半边、右半边长期空置（浏览器验收 D-3）。
      // 注意：RGL 收到新 layout prop 时会自行 compact（`useGridLayout` 的 prop 同步 effect），
      // 故渲染位置可能比这里存的 y 更靠上——两者在首次拖拽后即收敛（onDragStop 同步整盘），
      // 落库内容始终合法，不需要在这里预压缩。
      const spot = findFreePlacement(prev.widgets, size);
      const widget: DashboardChartWidget = {
        widgetId: createWidgetId(),
        type: 'chart',
        chartId,
        ...normalizePlacement({ x: spot.x, y: spot.y, w: size.w, h: size.h }, size),
      };
      return { ...prev, widgets: [...prev.widgets, widget] };
    });
  }, []);

  const handleRemoveWidget = useCallback((widgetId: string) => {
    setDoc((prev) => ({
      ...prev,
      widgets: prev.widgets.filter((widget) => widget.widgetId !== widgetId),
    }));
  }, []);

  /** 新建筛选器块：与图表块走同一套落位规则（放进首个空位，而不是一律落在最左边）。 */
  const handleAddFilterWidget = useCallback((config: NewFilterWidgetConfig) => {
    setDoc((prev) => {
      const size = DASHBOARD_DEFAULT_SIZE.filter;
      const spot = findFreePlacement(prev.widgets, size);
      const widget: DashboardFilterWidget = {
        widgetId: createWidgetId(),
        type: 'filter',
        binding: config.binding,
        label: config.label,
        dataType: config.dataType,
        // 算子/多选由弹窗按族给默认（见 AddFilterWidgetModal.defaultOperatorFor）。
        operator: config.operator,
        multi: config.multi,
        // 日期族才需要粒度与周计算逻辑；先给默认，用户在块上「配置」里再调。
        ...(filterWidgetFamily({ dataType: config.dataType }) === 'date'
          ? { date: { granularity: 'day' as const, weekStart: 1 as const } }
          : {}),
        ...normalizePlacement({ x: spot.x, y: spot.y, w: size.w, h: size.h }, size),
      };
      return { ...prev, widgets: [...prev.widgets, widget] };
    });
    setAddFilterOpen(false);
  }, []);

  /**
   * 改筛选器状态的**唯一出口**：写 layout（`defaultValue` 就是「该筛选器的默认选中值」，
   * 重开盘时由它做初值）→ 立刻按新取值重取一次。筛选器的意义就是「切一下马上看结果」。
   *
   * 传入显式的 `nextValue`/`nextDate` 而不是读 state：粒度与取值要同一次请求生效，
   * 读上一轮的状态会算出旧粒度的区间。重取只用**已落库**的筛选器（见 queryWithFilters）。
   */
  const applyFilterState = useCallback(
    (
      widgetId: string,
      nextValue: unknown,
      nextDate?: { granularity: DateGranularity; weekStart: WeekStart }
    ) => {
      const values = { ...filterValues, [widgetId]: nextValue };
      const widgets = doc.widgets.map((widget) => {
        if (widget.widgetId !== widgetId || widget.type !== 'filter') {
          return widget;
        }
        return {
          ...widget,
          defaultValue: nextValue,
          // 只有日期族带 date 配置；其余族保持原样（没有该键就是没有）。
          date: nextDate ?? widget.date,
        };
      });
      setFilterValues(values);
      setDoc((prev) => ({ ...prev, widgets }));
      if (!id) {
        return;
      }
      void queryWithFilters(id, widgets, values);
    },
    [doc.widgets, filterValues, id, queryWithFilters]
  );

  /**
   * 换算子（数值族在块上直接改）。只改 layout：取值形状在**下发时**按新算子收形
   * （例如 between ↔ gt 会改变载荷元素个数），所以不用同步改取值。
   */
  const handleFilterOperatorChange = useCallback(
    (widgetId: string, operator: DashboardFilterWidget['operator']) => {
      const widgets = doc.widgets.map((widget) =>
        widget.widgetId === widgetId && widget.type === 'filter' ? { ...widget, operator } : widget
      );
      setDoc((prev) => ({ ...prev, widgets }));
      if (!id) {
        return;
      }
      void queryWithFilters(id, widgets, filterValues);
    },
    [doc.widgets, filterValues, id, queryWithFilters]
  );

  /** 完整日期筛选弹窗确定：取值与粒度/周计算逻辑一起落库并重取。 */
  const handleConfigureFilterOk = useCallback(
    (payload: DateFilterModalPayload) => {
      if (!configuringFilterId) {
        return;
      }
      applyFilterState(configuringFilterId, payload.value, {
        granularity: payload.granularity,
        weekStart: payload.weekStart,
      });
      setConfiguringFilterId(null);
    },
    [applyFilterState, configuringFilterId]
  );

  const handleSave = async () => {
    if (!id) {
      return;
    }
    const trimmed = name.trim();
    if (!trimmed) {
      message.error(intl.formatMessage({ id: 'dashboard.nameRequired' }));
      return;
    }
    setSaving(true);
    try {
      const layoutJson = serializeDashboardLayout(doc);
      // PUT 遵循「未提供则保留」约定，这里显式给出 name 与 layout_json 两项即全量覆写布局。
      await dashboardsApi.update(id, { name: trimmed, layout_json: layoutJson });
      setName(trimmed);
      setBaseline({ name: trimmed, layout: layoutJson });
      // 落库后这些块转为「已落库」，取数交回 /query。
      persistedIdsRef.current = new Set(doc.widgets.map((widget) => widget.widgetId));
      localStorage.removeItem(DRAFT_PREFIX + id);
      setDraftRestored(false);
      message.success(intl.formatMessage({ id: 'common.success' }));
      // 落库后由后端按新布局重新逐块取数，顺手覆盖掉未持久化块的本地结果。
      // persistedIdsRef 刚在上面刷过，所以这次会把（刚保存的）筛选器一并下发。
      await queryWithFilters(id, doc.widgets, filterValues);
    } catch (error: any) {
      message.error(error.message || intl.formatMessage({ id: 'common.error' }));
    } finally {
      setSaving(false);
    }
  };

  const handleDiscardDraft = () => {
    if (!id) {
      return;
    }
    localStorage.removeItem(DRAFT_PREFIX + id);
    setDraftRestored(false);
    load(id);
  };

  /** 正在「配置」的筛选器块；null 时弹窗不渲染内容。 */
  const configuringFilter = configuringFilterId
    ? (filterWidgetsOf(doc.widgets).find((widget) => widget.widgetId === configuringFilterId) ??
      null)
    : null;

  const renderChartBlock = (widget: DashboardChartWidget) => {
    const result = results[widget.widgetId];

    if (result) {
      if (result.status === 'chart_deleted') {
        return (
          <BlockPlaceholder
            status="warning"
            title={intl.formatMessage({ id: 'dashboard.chartDeleted' })}
            description={intl.formatMessage({ id: 'dashboard.chartDeletedDesc' })}
          />
        );
      }
      if (result.status === 'chart_missing') {
        return (
          <BlockPlaceholder
            status="warning"
            title={intl.formatMessage({ id: 'dashboard.chartMissing' })}
            description={intl.formatMessage({ id: 'dashboard.chartMissingDesc' })}
          />
        );
      }
      if (result.status !== 'ok') {
        return (
          <BlockPlaceholder
            status="error"
            title={intl.formatMessage({ id: 'dashboard.blockError' })}
            description={result.message}
          />
        );
      }
      const chart = chartsById[widget.chartId];
      const data = extractChartData(result.data);
      if (!chart || !data) {
        return (
          <BlockPlaceholder
            status="warning"
            title={intl.formatMessage({ id: 'dashboard.chartMissing' })}
            description={intl.formatMessage({ id: 'dashboard.chartMissingDesc' })}
          />
        );
      }
      return (
        <ChartView
          chart={chart}
          data={data}
          fieldNames={fieldNames}
          echartsStyle={{ height: '100%', minHeight: 0 }}
        />
      );
    }

    // 未持久化的块：键不存在 = 仍在取数；值为 null = 取数失败。
    if (!(widget.widgetId in pendingData)) {
      return (
        <div
          style={{
            height: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Spin />
        </div>
      );
    }
    const pending = pendingData[widget.widgetId];
    const chart = chartsById[widget.chartId];
    if (!chart) {
      return (
        <BlockPlaceholder
          status="warning"
          title={intl.formatMessage({ id: 'dashboard.chartMissing' })}
          description={intl.formatMessage({ id: 'dashboard.chartMissingDesc' })}
        />
      );
    }
    if (!pending) {
      return (
        <BlockPlaceholder
          status="error"
          title={intl.formatMessage({ id: 'dashboard.blockError' })}
          description={intl.formatMessage({ id: 'dashboard.blockErrorDesc' })}
        />
      );
    }
    return (
      <ChartView
        chart={chart}
        data={pending}
        fieldNames={fieldNames}
        echartsStyle={{ height: '100%', minHeight: 0 }}
      />
    );
  };

  // ⚠️ 装载态与错误态**不**走提前 return：`<div ref={containerRef}>` 必须在首屏 commit
  // 里就存在。`useContainerWidth` 的「挂载即测量 + 挂 ResizeObserver」那个 effect 只跑
  // 一次（依赖是 measureWidth，而它只依赖 mounted），若那一刻 ref 还是 null，它会直接
  // return —— ResizeObserver 永远挂不上、宽度永远停在 initialWidth(1280)。症状是栅格按
  // 1280 排版：窄屏块溢出容器造成横向滚动、宽屏右侧留白，且此后任何 resize 都不再重测
  //（浏览器验收 D-2）。三种状态都渲染在同一个容器里，测宽口径才一致。
  const chartOptions = charts.map((chart) => ({ value: chart.id, label: chart.name }));

  return (
    <div className="dr-page">
      <PageHeader
        icon={<DashboardOutlined />}
        title={intl.formatMessage({ id: 'dashboard.editor' })}
        description={
          dirty
            ? intl.formatMessage({ id: 'dashboard.unsaved' })
            : name || intl.formatMessage({ id: 'dashboard.untitled' })
        }
        extra={
          <>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')}>
              {intl.formatMessage({ id: 'dashboard.back' })}
            </Button>
            <Button
              icon={<ReloadOutlined />}
              loading={querying}
              onClick={() => {
                // 刷新沿用当前筛选器取值（不是空载荷），否则点一下刷新就"筛了个寂寞"。
                if (id) void queryWithFilters(id, doc.widgets, filterValues);
              }}
            >
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              disabled={!dirty}
              onClick={handleSave}
            >
              {intl.formatMessage({ id: 'common.save' })}
            </Button>
          </>
        }
      />

      {draftRestored && (
        <Alert
          type="warning"
          showIcon
          title={intl.formatMessage({ id: 'dashboard.draftRestored' })}
          action={
            <Button size="small" onClick={handleDiscardDraft}>
              {intl.formatMessage({ id: 'dashboard.discardDraft' })}
            </Button>
          }
        />
      )}

      <Card>
        <div className="dr-card-toolbar">
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={intl.formatMessage({ id: 'dashboard.name' })}
            aria-label={intl.formatMessage({ id: 'dashboard.name' })}
            status={loading || name.trim() ? undefined : 'error'}
            style={{ maxWidth: 320 }}
          />
          <Space>
            <Text type="secondary">{intl.formatMessage({ id: 'dashboard.addChart' })}</Text>
            <Select
              value={null}
              onChange={(value: number) => handleAddChart(value)}
              placeholder={intl.formatMessage({ id: 'dashboard.selectChart' })}
              aria-label={intl.formatMessage({ id: 'dashboard.selectChart' })}
              options={chartOptions}
              notFoundContent={intl.formatMessage({ id: 'dashboard.noCharts' })}
              showSearch
              optionFilterProp="label"
              style={{ minWidth: 220 }}
            />
            <Button
              icon={<PlusOutlined />}
              data-testid="dashboard-add-filter"
              onClick={() => setAddFilterOpen(true)}
            >
              {intl.formatMessage({ id: 'dashboard.addFilter' })}
            </Button>
          </Space>
        </div>

        <div ref={containerRef}>
          {loading ? (
            <div className="dr-state">
              <Spin size="large" />
            </div>
          ) : loadError ? (
            <Result
              status="warning"
              title={intl.formatMessage({ id: 'dashboard.notFound' })}
              subTitle={loadError}
              extra={
                <Button onClick={() => navigate('/')}>
                  {intl.formatMessage({ id: 'dashboard.back' })}
                </Button>
              }
            />
          ) : doc.widgets.length === 0 ? (
            <div className="dr-state">
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={
                  <Space orientation="vertical" size={2}>
                    <Text>{intl.formatMessage({ id: 'dashboard.emptyCanvas' })}</Text>
                    <Text type="secondary">
                      {intl.formatMessage({ id: 'dashboard.emptyCanvasDesc' })}
                    </Text>
                  </Space>
                }
              />
            </div>
          ) : mounted && width > 0 ? (
            <GridLayout
              width={width}
              gridConfig={{
                cols: DASHBOARD_GRID_COLS,
                rowHeight: 40,
                margin: [16, 16],
                containerPadding: [0, 0],
              }}
              compactor={verticalCompactor}
              layout={layout}
              onDragStop={handleLayoutCommit}
              onResizeStop={handleLayoutCommit}
            >
              {doc.widgets.map(
                (widget) =>
                  widget.type === 'chart' ? (
                    <div key={widget.widgetId}>
                      <Card
                        size="small"
                        title={
                          widget.titleOverride ||
                          chartsById[widget.chartId]?.name ||
                          `#${widget.chartId}`
                        }
                        extra={
                          <Button
                            type="text"
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            aria-label={intl.formatMessage({ id: 'dashboard.removeBlock' })}
                            onClick={() => handleRemoveWidget(widget.widgetId)}
                          />
                        }
                        style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
                        styles={{
                          body: { flex: 1, minHeight: 0, padding: 8, overflow: 'hidden' },
                        }}
                      >
                        {renderChartBlock(widget)}
                      </Card>
                    </div>
                  ) : widget.type === 'filter' ? (
                    <div key={widget.widgetId}>
                      <DashboardFilterBlock
                        widget={widget}
                        value={filterValues[widget.widgetId]}
                        // 未落库的筛选器后端读不到（盘级取数按已落库 layout 建索引）。
                        unsaved={!persistedIdsRef.current.has(widget.widgetId)}
                        onChange={(next) => applyFilterState(widget.widgetId, next)}
                        onOperatorChange={(operator) =>
                          handleFilterOperatorChange(widget.widgetId, operator)
                        }
                        onConfigure={() => setConfiguringFilterId(widget.widgetId)}
                        onRemove={() => handleRemoveWidget(widget.widgetId)}
                      />
                    </div>
                  ) : null
                // 说明：text 块保持现状——占用宫格但不画内容。
              )}
            </GridLayout>
          ) : (
            <div className="dr-state">
              <Spin />
            </div>
          )}
        </div>
      </Card>

      <AddFilterWidgetModal
        open={addFilterOpen}
        datasets={datasets}
        onOk={handleAddFilterWidget}
        onCancel={() => setAddFilterOpen(false)}
      />

      <DateFilterModal
        open={configuringFilter !== null}
        fieldName={configuringFilter?.label}
        withTime={configuringFilter ? dateFilterWidgetSettings(configuringFilter).withTime : false}
        initial={
          // 取值 state 是 unknown（三族共用），进日期弹窗前必须过守卫。
          configuringFilterId && isDateFilterValue(filterValues[configuringFilterId])
            ? filterValues[configuringFilterId]
            : undefined
        }
        initialGranularity={configuringFilter?.date?.granularity}
        initialWeekStart={configuringFilter?.date?.weekStart}
        onOk={handleConfigureFilterOk}
        onCancel={() => setConfiguringFilterId(null)}
      />
    </div>
  );
};

export default DashboardEditor;
