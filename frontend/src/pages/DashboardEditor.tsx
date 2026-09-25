import {
  ArrowLeftOutlined,
  DashboardOutlined,
  DeleteOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Empty,
  Input,
  message,
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
} from '../api';
import ChartView from '../components/ChartView/ChartView';
import PageHeader from '../components/PageHeader';
import {
  createWidgetId,
  DASHBOARD_DEFAULT_SIZE,
  DASHBOARD_GRID_COLS,
  DASHBOARD_MIN_H,
  DASHBOARD_MIN_W,
  type DashboardChartWidget,
  type DashboardLayoutDocument,
  migrateDashboardLayout,
  normalizePlacement,
  serializeDashboardLayout,
} from '../lib/dashboardLayoutSchema';

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

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [doc, setDoc] = useState<DashboardLayoutDocument>(() => migrateDashboardLayout(''));
  const [baseline, setBaseline] = useState<{ name: string; layout: string } | null>(null);
  const [charts, setCharts] = useState<Chart[]>([]);
  const [chartsById, setChartsById] = useState<Record<number, Chart>>({});
  const [results, setResults] = useState<Record<string, DashboardQueryResult>>({});
  const [pendingData, setPendingData] = useState<Record<string, ChartDataResponse | null>>({});
  const [querying, setQuerying] = useState(false);
  const [saving, setSaving] = useState(false);
  const [draftRestored, setDraftRestored] = useState(false);
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

  /** 盘级取数。v1 不提供筛选器入口，故此处的 `filters` 恒为空数组。 */
  const runQuery = useCallback(
    async (dashboardId: string) => {
      setQuerying(true);
      try {
        const response = await dashboardsApi.query(dashboardId, { filters: [] });
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
    [intl]
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

        const serverDoc = migrateDashboardLayout(dashboard.layout_json);
        const serverLayout = serializeDashboardLayout(serverDoc);
        persistedIdsRef.current = new Set(serverDoc.widgets.map((widget) => widget.widgetId));

        // 本地草稿只在「真的与后端不一致」时恢复；一致就顺手清掉，
        // 避免一条陈旧草稿长期驻留、下次打开又被判定为「未保存改动」。
        let nextDoc = serverDoc;
        let nextName = dashboard.name;
        let restored = false;
        const draftRaw = localStorage.getItem(DRAFT_PREFIX + dashboardId);
        if (draftRaw) {
          const draft = readDraft(draftRaw);
          if (draft && serializeDashboardLayout(draft.layout) !== serverLayout) {
            nextDoc = draft.layout;
            nextName = draft.name ?? dashboard.name;
            restored = true;
          } else {
            localStorage.removeItem(DRAFT_PREFIX + dashboardId);
          }
        }

        setDoc(nextDoc);
        setName(nextName);
        setBaseline({ name: dashboard.name, layout: serverLayout });
        setDraftRestored(restored);

        await runQuery(dashboardId);
      } catch (error: any) {
        setLoadError(error.message || intl.formatMessage({ id: 'dashboard.loadFailed' }));
      } finally {
        setLoading(false);
      }
    },
    [intl, runQuery]
  );

  useEffect(() => {
    if (id) {
      load(id);
    }
  }, [id, load]);

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
      // 落在当前最底边之下，避免与既有块重叠。
      // 注意：RGL 收到新 layout prop 时会自行 compact（`useGridLayout` 的 prop 同步 effect），
      // 故渲染位置可能比这里存的 y 更靠上——两者在首次拖拽后即收敛（onDragStop 同步整盘），
      // 落库内容始终合法，不需要在这里预压缩。
      const bottom = prev.widgets.reduce((max, widget) => Math.max(max, widget.y + widget.h), 0);
      const widget: DashboardChartWidget = {
        widgetId: createWidgetId(),
        type: 'chart',
        chartId,
        ...normalizePlacement({ x: 0, y: bottom, w: size.w, h: size.h }, size),
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
      await runQuery(id);
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
        <ChartView chart={chart} data={data} echartsStyle={{ height: '100%', minHeight: 0 }} />
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
      <ChartView chart={chart} data={pending} echartsStyle={{ height: '100%', minHeight: 0 }} />
    );
  };

  if (loading) {
    return (
      <div className="dr-page dr-page--center">
        <Spin size="large" />
      </div>
    );
  }

  if (loadError) {
    return (
      <div className="dr-page dr-page--center">
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
      </div>
    );
  }

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
                if (id) runQuery(id);
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
            status={name.trim() ? undefined : 'error'}
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
          </Space>
        </div>

        <div ref={containerRef}>
          {doc.widgets.length === 0 ? (
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
              {doc.widgets
                .filter((widget): widget is DashboardChartWidget => widget.type === 'chart')
                .map((widget) => (
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
                ))}
            </GridLayout>
          ) : (
            <div className="dr-state">
              <Spin />
            </div>
          )}
        </div>
      </Card>
    </div>
  );
};

export default DashboardEditor;
