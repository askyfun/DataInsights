import {
  ArrowLeftOutlined,
  ClearOutlined,
  DashboardOutlined,
  DeleteOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import {
  Alert,
  App,
  Button,
  Card,
  Dropdown,
  Empty,
  Input,
  Result,
  Select,
  Space,
  Spin,
  Tag,
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
  type DatasetColumn,
  dashboardsApi,
  datasetsApi,
} from '../api';
import ChartLinkageSettings from '../components/ChartLinkage/ChartLinkageSettings';
import ChartView from '../components/ChartView/ChartView';
import AddFilterWidgetModal, {
  type NewFilterWidgetConfig,
} from '../components/DashboardFilterBlock/AddFilterWidgetModal';
import DashboardFilterBlock from '../components/DashboardFilterBlock/DashboardFilterBlock';
import DashboardPageTabs from '../components/DashboardPageTabs';
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
  applicableFilterWidgets,
  createPageId,
  createWidgetId,
  DASHBOARD_DEFAULT_SIZE,
  DASHBOARD_GRID_COLS,
  DASHBOARD_MIN_H,
  DASHBOARD_MIN_W,
  type DashboardChartWidget,
  type DashboardFilterWidget,
  type DashboardLayoutDocument,
  type DashboardLinkageTarget,
  type DashboardPage,
  type DashboardWidget,
  filterWidgetsOf,
  findFreePlacement,
  migrateDashboardLayout,
  normalizePlacement,
  reorderPages,
  serializeDashboardLayout,
  widgetsOfPage,
} from '../lib/dashboardLayoutSchema';
import {
  type ActiveLinkageMap,
  incomingLinkages,
  linkageCandidates,
  linkageKeyColumn,
  linkageQueryPayload,
} from '../lib/dashboardLinkage';
import type { DateGranularity, WeekStart } from '../lib/dateFilter';
import { isDateFilterValue } from '../lib/dateFilter';
import { useStore } from '../store';

const { Text } = Typography;

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
  /**
   * 当前激活的页面 id。刻意**不写进文档**：它是「正在看哪一页」这个会话态，
   * 不是布局的一部分（落库会让两个用户互相抢视图）。打开盘时取首页。
   */
  const [activePageId, setActivePageId] = useState<string>('');
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
  /**
   * 已激活的图表联动（来源 widgetId → 被点击的维度取值，issue #143）。
   * 与筛选器取值一样是**会话内状态**、不写进 layout：布局里只有「联动配置」（打到哪块图），
   * 点击产生的是取值。
   */
  const [activeLinkages, setActiveLinkages] = useState<ActiveLinkageMap>({});
  /** 正在配置联动设置的图表块；null 时弹窗不渲染内容。 */
  const [linkageSettingsFor, setLinkageSettingsFor] = useState<string | null>(null);
  /** 数据集 id → 列（跨数据集联动要在目标数据集上选关联字段）。 */
  const [columnsByDataset, setColumnsByDataset] = useState<Record<number, DatasetColumn[]>>({});
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
   * `filters` / `linkages` 都是**当前取值**（不是合并结果）：合并由后端单点完成（PRD §6.3），
   * 前端只负责把「哪个筛选器现在选了什么」「哪块图被点了什么值」如实下发，形状规则见
   * `lib/dashboardFilterValue` 与 `lib/dashboardLinkage`。
   * `pageId` 让后端只取当前页的块（多页面盘不必把没在看的页也跑一遍 SQL）。
   */
  const runQuery = useCallback(
    async (
      dashboardId: string,
      pageId: string,
      filters: DashboardQueryFilterValue[],
      linkages: ReturnType<typeof linkageQueryPayload>
    ) => {
      setQuerying(true);
      try {
        const response = await dashboardsApi.query(dashboardId, {
          page_id: pageId,
          filters,
          linkages,
        });
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
   * 按给定的筛选器 / 联动状态重取**当前页**。
   *
   * 只把**已落库**的筛选器 / 联动来源下发出去：后端是按已落库的 layout 逐块取数、并按同一份
   * layout 建筛选器与联动去向索引的，未保存的块它根本认不出来 —— 发了也只是白跑一趟。
   * 下发的筛选器集合是**本页的 + 任意页 `scope: 'all'` 的**（`applicableFilterWidgets`），
   * 与后端 `projectLayout` 的按页收窄是同一口径。
   * 调用方传显式的 widgets/values/pageId/linkages 而不是读组件状态：改筛选取值、改粒度
   * 这些场景下新状态还没落进 state，读旧值会算出上一轮的载荷。
   */
  const queryWithState = useCallback(
    (
      dashboardId: string,
      widgets: readonly DashboardWidget[],
      values: Record<string, unknown>,
      pageId: string,
      linkages: ActiveLinkageMap
    ) => {
      const active = applicableFilterWidgets(widgets, pageId).filter((widget) =>
        persistedIdsRef.current.has(widget.widgetId)
      );
      // 来源也必须已落库：后端从 layout 读「这块图打到哪些目标」，未保存的配置读不到。
      const persistedLinkages = linkageQueryPayload(
        Object.fromEntries(
          Object.entries(linkages).filter(([widgetId]) => persistedIdsRef.current.has(widgetId))
        )
      );
      return runQuery(
        dashboardId,
        pageId,
        dashboardFiltersPayload(active, values),
        persistedLinkages
      );
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
        // 联动是会话内状态：重新装载（含丢弃草稿）一律回到「没有联动」。
        setActiveLinkages({});
        setLinkageSettingsFor(null);

        const serverDoc = migrateDashboardLayout(dashboard.layout_json);

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
        // 打开盘时落回首页：激活页是会话态，不落库（见 activePageId 的注释）。
        const firstPageId = nextDoc.pages[0].id;
        setActivePageId(firstPageId);
        setBaseline({ name: dashboard.name, layout: serverLayout });
        setDraftRestored(restored);

        // 筛选器初值取布局里落库的「默认选中值」：控件与首屏取数用同一份，
        // 否则会出现"控件显示最近 7 天、实际查的是全量"这种不一致。
        // 取全部页面的筛选器（含别的页上的 `scope: all`）：它们在任何页都要有初值。
        const seeded = initialFilterWidgetValues(filterWidgetsOf(nextDoc.widgets));
        setFilterValues(seeded);
        await queryWithState(dashboardId, nextDoc.widgets, seeded, firstPageId, {});
      } catch (error: any) {
        setLoadError(error.message || intl.formatMessage({ id: 'dashboard.loadFailed' }));
      } finally {
        setLoading(false);
      }
    },
    [intl, queryWithState]
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

  const pages = doc.pages;
  /**
   * 当前页的块。用 useMemo 而不是就地 filter：下面几个 effect 拿它当依赖，
   * 每次渲染都新建数组会让它们每渲染一次就跑一遍（虽然体内有幂等守卫，但白跑）。
   */
  const pageWidgets = useMemo(
    () => widgetsOfPage(doc.widgets, activePageId),
    [doc.widgets, activePageId]
  );

  /**
   * 未持久化块的本地取数。
   *
   * 盘级 `/query` 是**后端按已落库的 layout 逐块取数**，所以刚拖进来、还没保存的块
   * 拿不到结果。v1 的 `filters` 恒为空数组，此时图表负载与「带盘级筛选」的结果完全等价，
   * 故直接用图表自身取数端点补上，避免"加进来却一片空白、必须保存才看得见"。
   * 保存后 `/query` 的结果优先（见 renderChartBlock 的判定顺序）。
   *
   * 只跑**当前页**的块：别的页的块此刻不渲染，取回来也没人看（切回去时本 effect 会重跑）。
   */
  useEffect(() => {
    if (!id) {
      return;
    }
    for (const widget of pageWidgets) {
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
  }, [id, pageWidgets, results, pendingData]);

  /**
   * 盘内图表块引用的数据集（去重、升序）。
   *
   * 这些数据集的列清单有两个消费方：ChartView 的列 ID→展示名翻译（缺了它 combo 分支会把
   * 裸列 ID 画进坐标轴），以及跨数据集联动的「关联字段」选择器。
   */
  const referencedDatasetIds = useMemo(() => {
    const ids = new Set<number>();
    for (const widget of doc.widgets) {
      if (widget.type !== 'chart') {
        continue;
      }
      const datasetId = chartsById[widget.chartId]?.dataset_id;
      if (typeof datasetId === 'number') {
        ids.add(datasetId);
      }
    }
    return [...ids].sort((a, b) => a - b);
  }, [doc.widgets, chartsById]);

  /**
   * 按当前引用的数据集**补齐缺失的列清单**（已拉过的不再重复请求）。
   *
   * 放在 effect 而不是装载流程里：会话中新加进来的图表块也要能立刻配置跨数据集联动，
   * 否则用户只能刷新页面才看得到目标字段。单个数据集失败只落空清单、不阻断装载。
   */
  const loadedDatasetIdsRef = useRef<Set<number>>(new Set());
  useEffect(() => {
    const missing = referencedDatasetIds.filter((id) => !loadedDatasetIdsRef.current.has(id));
    if (missing.length === 0) {
      return;
    }
    for (const id of missing) {
      loadedDatasetIdsRef.current.add(id);
    }
    void Promise.all(
      missing.map((id) =>
        datasetsApi
          .getColumns(id)
          .then((response) => ({ id, columns: response.data.data ?? [] }))
          .catch(() => ({ id, columns: [] as DatasetColumn[] }))
      )
    ).then((columnGroups) => {
      setColumnsByDataset((prev) => {
        const next = { ...prev };
        for (const group of columnGroups) {
          next[group.id] = group.columns;
        }
        return next;
      });
      setFieldNames((prev) => {
        const next = { ...prev };
        for (const group of columnGroups) {
          for (const column of group.columns) {
            next[column.id] = column.name;
          }
        }
        return next;
      });
    });
  }, [referencedDatasetIds]);

  const layout = useMemo<Layout>(
    () =>
      pageWidgets.map((widget) => ({
        i: widget.widgetId,
        x: widget.x,
        y: widget.y,
        w: widget.w,
        h: widget.h,
        minW: DASHBOARD_MIN_W,
        minH: DASHBOARD_MIN_H,
      })),
    [pageWidgets]
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

  const handleAddChart = useCallback(
    (chartId: number) => {
      setDoc((prev) => {
        const size = DASHBOARD_DEFAULT_SIZE.chart;
        // 放进**当前页**的首个空位（自上而下、自左而右），而不是一律 `x: 0` 落在最底边
        // —— 否则 12 列画布上默认 6 列宽的块会全部堆在左半边、右半边长期空置（浏览器验收 D-3）。
        // 空位只在当前页里找：跨页算空位会让新块落到别的页已有的位置下。
        // 注意：RGL 收到新 layout prop 时会自行 compact（`useGridLayout` 的 prop 同步 effect），
        // 故渲染位置可能比这里存的 y 更靠上——两者在首次拖拽后即收敛（onDragStop 同步整页），
        // 落库内容始终合法，不需要在这里预压缩。
        const spot = findFreePlacement(widgetsOfPage(prev.widgets, activePageId), size);
        const widget: DashboardChartWidget = {
          widgetId: createWidgetId(),
          pageId: activePageId,
          type: 'chart',
          chartId,
          ...normalizePlacement({ x: spot.x, y: spot.y, w: size.w, h: size.h }, size),
        };
        return { ...prev, widgets: [...prev.widgets, widget] };
      });
    },
    [activePageId]
  );

  const handleRemoveWidget = useCallback((widgetId: string) => {
    setDoc((prev) => ({
      ...prev,
      widgets: prev.widgets.filter((widget) => widget.widgetId !== widgetId),
    }));
    // 会话内的联动取值随块一起消失（布局里残留的「去向」由后端忽略、弹窗确定时自愈）。
    setActiveLinkages((prev) => {
      if (!(widgetId in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[widgetId];
      return next;
    });
    setLinkageSettingsFor((prev) => (prev === widgetId ? null : prev));
  }, []);

  /**
   * 切页：激活 + 按新页重取。
   *
   * 重取是必要的，而不是「反正结果按 widgetId 归位」：`/query` 一次只取一页，
   * 没查过的页在 `results` 里是空的，不重取就会整页停在转圈上。
   */
  const handleActivatePage = useCallback(
    (pageId: string) => {
      setActivePageId(pageId);
      if (id) {
        void queryWithState(id, doc.widgets, filterValues, pageId, activeLinkages);
      }
    },
    [activeLinkages, doc.widgets, filterValues, id, queryWithState]
  );

  const handleAddPage = useCallback(() => {
    const page: DashboardPage = {
      id: createPageId(),
      name: intl.formatMessage({ id: 'dashboard.pageNewName' }, { index: doc.pages.length + 1 }),
    };
    setDoc((prev) => ({ ...prev, pages: [...prev.pages, page] }));
    // 新页是空的，没有块可取数，故不触发查询。
    setActivePageId(page.id);
  }, [doc.pages.length, intl]);

  const handleRenamePage = useCallback((pageId: string, name: string) => {
    setDoc((prev) => ({
      ...prev,
      pages: prev.pages.map((page) => (page.id === pageId ? { ...page, name } : page)),
    }));
  }, []);

  /** 已落库的图表块（只有它们的联动配置后端读得到）。 */
  const isPersisted = useCallback((widgetId: string) => persistedIdsRef.current.has(widgetId), []);

  /**
   * 块上「联动来源的键列」：只有恰好一个维度、且图型支持点击的块才可作来源。
   * 盘内逐块算一次（图表配置在编辑期不变）。
   */
  const linkageKeyColumns = useMemo(() => {
    const map: Record<string, string | null> = {};
    for (const widget of doc.widgets) {
      if (widget.type !== 'chart') {
        continue;
      }
      const chart = chartsById[widget.chartId];
      map[widget.widgetId] = chart ? linkageKeyColumn(chart.config, chart.chart_type) : null;
    }
    return map;
  }, [doc.widgets, chartsById]);

  /**
   * 块可点击的联动键列。**只有已落库的块**才给：联动去向存在布局里，未保存的配置后端
   * 读不到，让用户点一个不生效的图比不让点更糟（弹窗里有同样的提示）。
   */
  const linkageFieldIdOf = (widget: DashboardChartWidget): string | null =>
    isPersisted(widget.widgetId) ? (linkageKeyColumns[widget.widgetId] ?? null) : null;

  /** 已落库的基线布局文档（判断「这块图的联动配置是否还没保存」）。 */
  const baselineDoc = useMemo(
    () => (baseline ? migrateDashboardLayout(baseline.layout) : null),
    [baseline]
  );

  /**
   * 该块的联动配置是否「还没生效」：块本身未落库，或它的 linkage 与基线不同。
   * 只看配置不看整盘 dirty —— 改个名字就弹「保存后生效」会变成噪声。
   */
  const linkageConfigPending = (widget: DashboardChartWidget): boolean => {
    if (!isPersisted(widget.widgetId)) {
      return true;
    }
    const baseWidget = baselineDoc?.widgets.find((item) => item.widgetId === widget.widgetId);
    const baseLinkage = baseWidget?.type === 'chart' ? baseWidget.linkage : undefined;
    return JSON.stringify(widget.linkage ?? null) !== JSON.stringify(baseLinkage ?? null);
  };

  /**
   * 数据项被点击：记下取值并按新状态重取。
   *
   * 联动取值是**会话内**状态（不进 layout）：布局里只存「打到哪块图」，点击产生的是值。
   * 同一来源再点一次直接覆盖（符合「点柱子即筛选」的心智模型）。
   */
  const handleDataPointClick = useCallback(
    (widgetId: string, column: string, value: unknown) => {
      if (!id) {
        return;
      }
      const next: ActiveLinkageMap = { ...activeLinkages, [widgetId]: { column, value } };
      setActiveLinkages(next);
      void queryWithState(id, doc.widgets, filterValues, activePageId, next);
    },
    [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]
  );

  /** 撤掉某个来源的联动（块上的「×」与 ⋮ 菜单里的「清除联动」共用这一个出口）。 */
  const clearLinkage = useCallback(
    (sourceWidgetId: string) => {
      if (!id || !(sourceWidgetId in activeLinkages)) {
        return;
      }
      const next = { ...activeLinkages };
      delete next[sourceWidgetId];
      setActiveLinkages(next);
      void queryWithState(id, doc.widgets, filterValues, activePageId, next);
    },
    [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]
  );

  /** 一键清除所有联动（PRD 要求「清除所有联动」）。 */
  const clearAllLinkages = useCallback(() => {
    if (!id || Object.keys(activeLinkages).length === 0) {
      return;
    }
    setActiveLinkages({});
    void queryWithState(id, doc.widgets, filterValues, activePageId, {});
  }, [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]);

  /**
   * 联动设置确定：把「打到哪些块、哪一列」写回来源块的布局配置，然后立刻重取一次。
   *
   * 配置在保存前不生效（后端按已落库 layout 读取去向），此时点击来源块仍会亮出联动提示，
   * 但数据要保存后才跟着变——弹窗里对此有「保存后生效」的提示。
   */
  const handleLinkageSettingsOk = useCallback(
    (targets: DashboardLinkageTarget[]) => {
      const sourceWidgetId = linkageSettingsFor;
      setLinkageSettingsFor(null);
      if (!sourceWidgetId) {
        return;
      }
      const widgets = doc.widgets.map((widget) => {
        if (widget.widgetId !== sourceWidgetId || widget.type !== 'chart') {
          return widget;
        }
        if (targets.length === 0) {
          // 全部取消勾选 = 没有联动配置：不留 `linkage: {targets: []}` 这种空壳。
          const cleared: DashboardChartWidget = { ...widget };
          delete cleared.linkage;
          return cleared;
        }
        return { ...widget, linkage: { targets } };
      });
      setDoc((prev) => ({ ...prev, widgets }));
      if (!id) {
        return;
      }
      void queryWithState(id, widgets, filterValues, activePageId, activeLinkages);
    },
    [
      activeLinkages,
      activePageId,
      doc.widgets,
      filterValues,
      id,
      linkageSettingsFor,
      queryWithState,
    ]
  );

  /**
   * 复制页面：页面本身 + 该页全部块。
   *
   * 块的 `widgetId` 必须重新生成——它的唯一域是**整盘**，沿用会导致两份块在同一盘里
   * 撞 id（渲染按 widgetId 归位、取数也按它建索引）。位置原样保留，所以副本与源页
   * 的排版一致。
   */
  const handleDuplicatePage = useCallback(
    (pageId: string) => {
      const copyId = createPageId();
      setDoc((prev) => {
        const index = prev.pages.findIndex((page) => page.id === pageId);
        if (index < 0) {
          return prev;
        }
        const copy: DashboardPage = {
          id: copyId,
          name: `${prev.pages[index].name}${intl.formatMessage({ id: 'dashboard.pageCopySuffix' })}`,
        };
        const cloned = widgetsOfPage(prev.widgets, pageId).map((widget) => ({
          ...widget,
          widgetId: createWidgetId(),
          pageId: copyId,
        }));
        const pages = [...prev.pages];
        pages.splice(index + 1, 0, copy);
        return { ...prev, pages, widgets: [...prev.widgets, ...cloned] };
      });
      setActivePageId(copyId);
    },
    [intl]
  );

  /**
   * 删除页面：页 + 该页全部块。副本不做级联外的任何清洗（同一盘只有一个引用点）。
   *
   * 盘**必须至少留一页**：全删掉就没有「往哪加图表」的落点了。守卫在 UI 层（删到最后一页
   * 时菜单项禁用），这里再兜一层，防止快捷键/程序调用绕过。
   */
  const handleRemovePage = useCallback(
    (pageId: string) => {
      if (doc.pages.length <= 1) {
        return;
      }
      // 文档改动在 updater 里从 prev 现算，而不是用闭包里那份 doc：本页与上一次
      // setDoc（例如刚复制出来的副本）挨得极近时，闭包里的 doc 可能落后一轮，
      // 用它算「剩下的页」会把上一轮的结果整份覆盖掉。
      setDoc((prev) => {
        if (prev.pages.length <= 1) {
          return prev;
        }
        return {
          ...prev,
          pages: prev.pages.filter((page) => page.id !== pageId),
          widgets: prev.widgets.filter((widget) => widget.pageId !== pageId),
        };
      });
      if (pageId !== activePageId) {
        return;
      }
      // 删掉的是正在看的页：落到原位置的右邻，没有右邻就取左邻（保证仍有激活页）。
      const index = doc.pages.findIndex((page) => page.id === pageId);
      const next = doc.pages[index + 1] ?? doc.pages[index - 1];
      if (next) {
        setActivePageId(next.id);
        if (id) {
          void queryWithState(
            id,
            doc.widgets.filter((widget) => widget.pageId !== pageId),
            filterValues,
            next.id,
            activeLinkages
          );
        }
      }
    },
    [activeLinkages, activePageId, doc.pages, doc.widgets, filterValues, id, queryWithState]
  );

  const handleReorderPages = useCallback((fromId: string, toId: string) => {
    setDoc((prev) => ({ ...prev, pages: reorderPages(prev.pages, fromId, toId) }));
  }, []);

  /** 改筛选器作用范围（本页 / 全部页面）。范围变了要立刻重取：作用面已经不同。 */
  const handleFilterScopeChange = useCallback(
    (widgetId: string, scope: DashboardFilterWidget['scope']) => {
      const widgets = doc.widgets.map((widget) =>
        widget.widgetId === widgetId && widget.type === 'filter' ? { ...widget, scope } : widget
      );
      setDoc((prev) => ({ ...prev, widgets }));
      if (!id) {
        return;
      }
      void queryWithState(id, widgets, filterValues, activePageId, activeLinkages);
    },
    [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]
  );

  /** 新建筛选器块：与图表块走同一套落位规则（放进当前页首个空位，而不是一律落在最左边）。 */
  const handleAddFilterWidget = useCallback(
    (config: NewFilterWidgetConfig) => {
      setDoc((prev) => {
        const size = DASHBOARD_DEFAULT_SIZE.filter;
        const spot = findFreePlacement(widgetsOfPage(prev.widgets, activePageId), size);
        const widget: DashboardFilterWidget = {
          widgetId: createWidgetId(),
          pageId: activePageId,
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
    },
    [activePageId]
  );

  /**
   * 改筛选器状态的**唯一出口**：写 layout（`defaultValue` 就是「该筛选器的默认选中值」，
   * 重开盘时由它做初值）→ 立刻按新取值重取一次。筛选器的意义就是「切一下马上看结果」。
   *
   * 传入显式的 `nextValue`/`nextDate` 而不是读 state：粒度与取值要同一次请求生效，
   * 读上一轮的状态会算出旧粒度的区间。重取只用**已落库**的筛选器（见 queryWithState）。
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
      void queryWithState(id, widgets, values, activePageId, activeLinkages);
    },
    [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]
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
      void queryWithState(id, widgets, filterValues, activePageId, activeLinkages);
    },
    [activeLinkages, activePageId, doc.widgets, filterValues, id, queryWithState]
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
      // persistedIdsRef 刚在上面刷过，所以这次会把（刚保存的）筛选器与联动一并下发。
      await queryWithState(id, doc.widgets, filterValues, activePageId, activeLinkages);
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

  const blockTitle = (widget: DashboardChartWidget) =>
    widget.titleOverride || chartsById[widget.chartId]?.name || `#${widget.chartId}`;

  /** 正在配置联动的来源块；null 时弹窗不渲染内容。 */
  const linkageSource =
    linkageSettingsFor !== null
      ? ((doc.widgets.find(
          (widget): widget is DashboardChartWidget =>
            widget.type === 'chart' && widget.widgetId === linkageSettingsFor
        ) ?? null) as DashboardChartWidget | null)
      : null;

  // `initialTargets` 必须引用稳定（弹窗打开时按它重置勾选），故按来源块派生。
  const linkageInitialTargets = useMemo(
    () => linkageSource?.linkage?.targets ?? [],
    [linkageSource]
  );

  /**
   * 块顶部的联动状态标签：
   *   - 来源侧：这块图正在驱动联动（自己的点击取值）；
   *   - 目标侧：被哪些来源筛着，逐条可 ×（清除该来源的联动）。
   */
  const linkageTags = (widget: DashboardChartWidget) => {
    const tags: React.ReactNode[] = [];
    const own = activeLinkages[widget.widgetId];
    if (own) {
      tags.push(
        <Tag key="own" color="processing" data-testid={`linkage-source-${widget.widgetId}`}>
          {intl.formatMessage({ id: 'dashboard.linkageActive' })}: {String(own.value)}
        </Tag>
      );
    }
    for (const incoming of incomingLinkages(doc.widgets, activeLinkages, widget.widgetId)) {
      const source = doc.widgets.find(
        (item): item is DashboardChartWidget =>
          item.type === 'chart' && item.widgetId === incoming.sourceWidgetId
      );
      tags.push(
        <Tag
          key={incoming.sourceWidgetId}
          color="blue"
          closable
          data-testid={`linkage-in-${widget.widgetId}`}
          onClose={(event) => {
            event.preventDefault();
            clearLinkage(incoming.sourceWidgetId);
          }}
        >
          {source ? blockTitle(source) : ''}: {String(incoming.value)}
        </Tag>
      );
    }
    return tags;
  };

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
          linkageFieldId={linkageFieldIdOf(widget)}
          onDataPointClick={
            linkageFieldIdOf(widget)
              ? (column, value) => handleDataPointClick(widget.widgetId, column, value)
              : undefined
          }
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
        linkageFieldId={linkageFieldIdOf(widget)}
        onDataPointClick={
          linkageFieldIdOf(widget)
            ? (column, value) => handleDataPointClick(widget.widgetId, column, value)
            : undefined
        }
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
            {Object.keys(activeLinkages).length > 0 && (
              <Button
                icon={<ClearOutlined />}
                data-testid="linkage-clear-all"
                onClick={clearAllLinkages}
              >
                {intl.formatMessage({ id: 'dashboard.linkageClearAll' })}
              </Button>
            )}
            <Button
              icon={<ReloadOutlined />}
              loading={querying}
              onClick={() => {
                // 刷新沿用当前筛选器与联动取值（不是空载荷），否则点一下刷新就"筛了个寂寞"。
                if (id)
                  void queryWithState(id, doc.widgets, filterValues, activePageId, activeLinkages);
              }}
            >
              {intl.formatMessage({ id: 'common.refresh' })}
            </Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              disabled={!dirty}
              data-testid="dashboard-save"
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
          ) : pageWidgets.length === 0 ? (
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
              {pageWidgets.map(
                (widget) =>
                  widget.type === 'chart' ? (
                    <div key={widget.widgetId}>
                      <Card
                        size="small"
                        title={
                          <Space size={4}>
                            <span>{blockTitle(widget)}</span>
                            {linkageTags(widget)}
                          </Space>
                        }
                        extra={
                          <Space size={0}>
                            <Dropdown
                              trigger={['click']}
                              menu={{
                                items: [
                                  {
                                    key: 'linkage-settings',
                                    label: intl.formatMessage({
                                      id: 'dashboard.linkageSettings',
                                    }),
                                  },
                                  ...(activeLinkages[widget.widgetId]
                                    ? [
                                        {
                                          key: 'linkage-clear',
                                          label: intl.formatMessage({
                                            id: 'dashboard.linkageClearOne',
                                          }),
                                        },
                                      ]
                                    : []),
                                ],
                                onClick: ({ key }) => {
                                  if (key === 'linkage-settings') {
                                    setLinkageSettingsFor(widget.widgetId);
                                    return;
                                  }
                                  clearLinkage(widget.widgetId);
                                },
                              }}
                            >
                              <Button
                                type="text"
                                size="small"
                                icon={<MoreOutlined />}
                                aria-label={intl.formatMessage({ id: 'dashboard.blockMenu' })}
                                data-testid={`block-menu-${widget.widgetId}`}
                              />
                            </Dropdown>
                            <Button
                              type="text"
                              size="small"
                              danger
                              icon={<DeleteOutlined />}
                              aria-label={intl.formatMessage({ id: 'dashboard.removeBlock' })}
                              onClick={() => handleRemoveWidget(widget.widgetId)}
                            />
                          </Space>
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
                        onScopeChange={(scope) => handleFilterScopeChange(widget.widgetId, scope)}
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

        {/* 页面标签条：编辑区在底部（对齐火山引擎盘底部多页面编辑区）。 */}
        <DashboardPageTabs
          pages={pages}
          activePageId={activePageId}
          canRemove={pages.length > 1}
          onActivate={handleActivatePage}
          onAdd={handleAddPage}
          onRename={handleRenamePage}
          onDuplicate={handleDuplicatePage}
          onRemove={handleRemovePage}
          onReorder={handleReorderPages}
        />
      </Card>

      <AddFilterWidgetModal
        open={addFilterOpen}
        datasets={datasets}
        onOk={handleAddFilterWidget}
        onCancel={() => setAddFilterOpen(false)}
      />

      <ChartLinkageSettings
        open={linkageSource !== null}
        source={
          linkageSource
            ? {
                datasetId: chartsById[linkageSource.chartId]?.dataset_id,
                keyColumn: linkageKeyColumns[linkageSource.widgetId] ?? null,
                unsaved: linkageConfigPending(linkageSource),
              }
            : null
        }
        candidates={linkageCandidates(doc.widgets, linkageSettingsFor ?? '').map((widget) => ({
          widgetId: widget.widgetId,
          label: blockTitle(widget),
          datasetId: chartsById[widget.chartId]?.dataset_id,
        }))}
        columnsByDataset={columnsByDataset}
        initialTargets={linkageInitialTargets}
        onOk={handleLinkageSettingsOk}
        onCancel={() => setLinkageSettingsFor(null)}
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
