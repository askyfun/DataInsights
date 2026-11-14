import { ArrowLeftOutlined, ClockCircleOutlined } from '@ant-design/icons';
import { Button, Result, Space, Spin, Typography } from 'antd';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { GridLayout, type Layout, verticalCompactor } from 'react-grid-layout';
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
import {
  DASHBOARD_GRID_COLS,
  DASHBOARD_MIN_H,
  DASHBOARD_MIN_W,
  type DashboardChartWidget,
  type DashboardLayoutDocument,
  migrateDashboardLayout,
  widgetsOfPage,
} from '../lib/dashboardLayoutSchema';
import {
  fitScreenScale,
  parsePlaybackConfig,
  SCREEN_DESIGN_HEIGHT,
  SCREEN_DESIGN_WIDTH,
  type ScreenPlaybackConfig,
} from '../lib/screenMode';
import { forceTheme, releaseTheme } from '../lib/theme';

const { Text, Title } = Typography;

/**
 * 仪表盘「数字大屏」播放器（issue #159）。
 *
 * 独立路由 `/dashboards/:id/screen`：只读、无编辑操作栏、深色、等比缩放、可选
 * 按页轮播与数据自动刷新。全部走既有取数链路（`POST /api/dashboards/{id}/query`），
 * 后端零改动——这是「仪表盘投大屏」最小形态（Non-goal：独立大屏编辑器 / 数字孪生）。
 *
 * 播放配置经 URL 参数传入（`?carousel=<秒>&refresh=<秒>`，0=关），这样：
 *   - 可直接被 iframe 嵌入（#160 Token iframe 的 Phase D 联动，AC-4）；
 *   - 轮播/刷新是**展示层**参数，不写进仪表盘布局文档（不污染数据模型）。
 *
 * 缩放心智：设计画布恒为 1920×1080（16:9），GridLayout 内部始终按 1920 宽排布，
 * 外层 div 用 `transform: scale()` 缩放到实际视口（fit，含 letterbox 黑边），
 * 不裁切不拉伸（AC-1）。这样图表块内的相对布局在任意分辨率下都一致。
 *
 * 深色：挂载时 `forceTheme('dark')`、卸载时 `releaseTheme()`——不改用户主题偏好
 * （AC-3：大屏默认深色，与 #146 主题联动）。
 */

/** 从盘级取数结果的 `data` 里取出图表负载（同 DashboardEditor 的口径）。 */
function extractChartData(raw: unknown): ChartDataResponse | null {
  if (raw && typeof raw === 'object' && 'data' in raw) {
    return (raw as { data: ChartDataResponse }).data;
  }
  return null;
}

/** 收集盘内所有图表块引用的数据集 id（用于拉列名映射）。 */
function referencedDatasetIds(
  widgets: readonly DashboardChartWidget[],
  chartsById: Record<number, Chart>
): number[] {
  const ids = new Set<number>();
  for (const widget of widgets) {
    const chart = chartsById[widget.chartId];
    if (chart) {
      ids.add(chart.dataset_id);
    }
  }
  return [...ids];
}

/** 单页大屏内容：只读渲染该页的图表块（栅格布局 + ChartView）。 */
const ScreenPage: React.FC<{
  doc: DashboardLayoutDocument;
  pageId: string;
  chartsById: Record<number, Chart>;
  fieldNames: Record<string, string>;
  results: Record<string, DashboardQueryResult>;
}> = ({ doc, pageId, chartsById, fieldNames, results }) => {
  const intl = useIntl();
  const pageWidgets = useMemo(() => widgetsOfPage(doc.widgets, pageId), [doc.widgets, pageId]);
  const gridWidgets = useMemo(
    () => pageWidgets.filter((widget) => widget.type === 'chart'),
    [pageWidgets]
  );

  const layout = useMemo<Layout>(
    () =>
      gridWidgets.map((widget) => ({
        i: widget.widgetId,
        x: widget.x,
        y: widget.y,
        w: widget.w,
        h: widget.h,
        minW: DASHBOARD_MIN_W,
        minH: DASHBOARD_MIN_H,
      })),
    [gridWidgets]
  );

  return (
    <GridLayout
      width={SCREEN_DESIGN_WIDTH}
      gridConfig={{
        cols: DASHBOARD_GRID_COLS,
        rowHeight: 40,
        margin: [16, 16],
        containerPadding: [0, 0],
      }}
      compactor={verticalCompactor}
      layout={layout}
    >
      {gridWidgets.map((widget) => {
        const result = results[widget.widgetId];
        const chart = chartsById[widget.chartId];
        const data = result ? extractChartData(result.data) : null;
        const status = result?.status ?? 'loading';
        return (
          <div key={widget.widgetId} data-testid={`screen-widget-${widget.widgetId}`}>
            {status === 'loading' ? (
              <div className="screen-widget-state">
                <Spin size="small" />
              </div>
            ) : status === 'ok' && chart && data ? (
              <ChartView
                chart={chart}
                data={data}
                fieldNames={fieldNames}
                echartsStyle={{ height: '100%', minHeight: 0 }}
              />
            ) : (
              <div className="screen-widget-state">
                <Text type="secondary">
                  {intl.formatMessage({
                    id:
                      status === 'chart_deleted'
                        ? 'dashboard.chartDeleted'
                        : status === 'chart_missing'
                          ? 'dashboard.chartMissing'
                          : 'dashboard.blockError',
                  })}
                </Text>
              </div>
            )}
          </div>
        );
      })}
    </GridLayout>
  );
};

const DashboardScreen: React.FC = () => {
  const intl = useIntl();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [doc, setDoc] = useState<DashboardLayoutDocument>(() => migrateDashboardLayout(''));
  const [chartsById, setChartsById] = useState<Record<number, Chart>>({});
  const [fieldNames, setFieldNames] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, DashboardQueryResult>>({});
  const [querying, setQuerying] = useState(false);
  const [activePageId, setActivePageId] = useState<string>('');

  // 视口尺寸（用于 fit 等比缩放）：监听 window resize。
  const [viewport, setViewport] = useState({ w: window.innerWidth, h: window.innerHeight });
  useEffect(() => {
    const onResize = () => setViewport({ w: window.innerWidth, h: window.innerHeight });
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  // 播放配置：从 URL 解析（?carousel=<秒>&refresh=<秒>）。
  const playback = useMemo<ScreenPlaybackConfig>(
    () => parsePlaybackConfig(window.location.search),
    []
  );

  // 强制深色：大屏默认深色（AC-3）。挂载即强制，卸载释放（回到用户偏好）。
  useEffect(() => {
    forceTheme('dark');
    return () => releaseTheme();
  }, []);

  // 页面引用（轮播/刷新都用）。用 state 而非 ref：轮播 effect 必须依赖它，
  // 才能在 load 完成（页面列表就绪）后**重挂** interval——若用 ref，effect 挂载时
  // 页面列表还是空，guard 直接 return，之后 load 完成但 effect 永不重跑，轮播就死了。
  const [pageIds, setPageIds] = useState<string[]>([]);
  const activePageIdRef = useRef(activePageId);
  useEffect(() => {
    activePageIdRef.current = activePageId;
  }, [activePageId]);

  /** 取当前页数据（按页收窄，与编辑器一致）。 */
  const runQuery = useCallback(async (dashboardId: string, pageId: string) => {
    setQuerying(true);
    try {
      const response = await dashboardsApi.query(dashboardId, { page_id: pageId });
      const map: Record<string, DashboardQueryResult> = {};
      for (const result of response.data.data.results ?? []) {
        map[result.widgetId] = result;
      }
      setResults(map);
    } catch {
      // 大屏只读：取数失败静默保留上一帧（轮播不中断），不弹错误条。
    } finally {
      setQuerying(false);
    }
  }, []);

  const load = useCallback(
    async (dashboardId: string) => {
      setLoading(true);
      setLoadError(null);
      try {
        const [detail, chartList] = await Promise.all([
          dashboardsApi.getById(dashboardId),
          chartsApi.getAll({ limit: 1000 }),
        ]);
        const dashboard: Dashboard = detail.data.data;
        const allCharts = chartList.data.data.items ?? [];
        const byId: Record<number, Chart> = {};
        for (const chart of allCharts) {
          byId[chart.id] = chart;
        }
        setChartsById(byId);
        setResults({});

        const serverDoc = migrateDashboardLayout(dashboard.layout_json);
        setDoc(serverDoc);
        setName(dashboard.name);

        // 打开时落回首页。
        const firstPageId = serverDoc.pages[0].id;
        setActivePageId(firstPageId);
        setPageIds(serverDoc.pages.map((page) => page.id));

        // 构建列名映射（binding.field 是列 id，ChartView 需要翻译成列名）。
        const chartWidgets = serverDoc.widgets.filter(
          (w): w is DashboardChartWidget => w.type === 'chart'
        );
        const datasetIds = referencedDatasetIds(chartWidgets, byId);
        const groups = await Promise.all(
          datasetIds.map((datasetId) =>
            datasetsApi
              .getColumns(datasetId)
              .then((resp) => ({ id: datasetId, columns: resp.data.data ?? [] }))
              .catch(() => ({ id: datasetId, columns: [] }))
          )
        );
        const names: Record<string, string> = {};
        for (const group of groups) {
          for (const column of group.columns) {
            names[column.id] = column.name;
          }
        }
        setFieldNames(names);

        await runQuery(dashboardId, firstPageId);
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
      void load(id);
    }
  }, [id, load]);

  // 轮播：多页面盘按页轮播（AC-2）。interval 变化时重挂。
  const carouselMs = playback.carouselIntervalMs;
  useEffect(() => {
    if (carouselMs <= 0 || pageIds.length <= 1 || !id) {
      return;
    }
    const timer = setInterval(() => {
      const ids = pageIds;
      if (ids.length <= 1) {
        return;
      }
      const currentIdx = ids.indexOf(activePageIdRef.current);
      const nextIdx = (currentIdx + 1) % ids.length;
      const nextPageId = ids[nextIdx];
      setActivePageId(nextPageId);
      void runQuery(id, nextPageId);
    }, carouselMs);
    return () => clearInterval(timer);
  }, [carouselMs, pageIds, id, runQuery]);

  // 数据自动刷新：重取当前页（AC-2），走既有取数链路。
  const refreshMs = playback.refreshIntervalMs;
  useEffect(() => {
    if (refreshMs <= 0 || !id) {
      return;
    }
    const timer = setInterval(() => {
      void runQuery(id, activePageIdRef.current);
    }, refreshMs);
    return () => clearInterval(timer);
  }, [refreshMs, id, runQuery]);

  // 等比缩放（AC-1）：fit 到视口。
  const scale = useMemo(() => fitScreenScale(viewport.w, viewport.h), [viewport.w, viewport.h]);

  const hasCarousel = carouselMs > 0;
  const hasRefresh = refreshMs > 0;
  const pageIndex = doc.pages.findIndex((page) => page.id === activePageId);
  const pageLabel =
    pageIndex >= 0
      ? `${intl.formatMessage({ id: 'screen.pageIndicator' })} ${pageIndex + 1}/${doc.pages.length}`
      : '';

  return (
    <div className="screen-root" data-testid="dashboard-screen">
      {/* 顶栏：最小化操作栏——仅返回 + 页面指示。大屏隐藏编辑操作（AC-1）。 */}
      <div className="screen-toolbar">
        <Space>
          <Button
            type="text"
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate(`/dashboards/${id}`)}
            data-testid="screen-back"
          >
            {intl.formatMessage({ id: 'dashboard.back' })}
          </Button>
          <Title level={5} style={{ margin: 0, color: 'var(--dr-text-1)' }}>
            {name}
          </Title>
        </Space>
        <Space size={16}>
          {hasCarousel && (
            <span className="screen-status">
              <ClockCircleOutlined /> {intl.formatMessage({ id: 'screen.carousel' })}{' '}
              {carouselMs / 1000}s
            </span>
          )}
          {hasRefresh && (
            <span className="screen-status">
              <ClockCircleOutlined /> {intl.formatMessage({ id: 'screen.refresh' })}{' '}
              {refreshMs / 1000}s
            </span>
          )}
          {doc.pages.length > 1 && <span className="screen-status">{pageLabel}</span>}
        </Space>
      </div>

      {/* 播放画布：等比缩放容器。缩放系数为 0（首帧未测量）时不渲染。 */}
      <div className="screen-viewport">
        {loading ? (
          <div className="screen-center">
            <Spin size="large" />
          </div>
        ) : loadError ? (
          <div className="screen-center">
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
        ) : scale.scale > 0 ? (
          <div
            className="screen-canvas"
            style={{
              width: scale.width,
              height: scale.height,
              transform: `translate(${scale.offsetX}px, ${scale.offsetY}px) scale(${scale.scale})`,
              transformOrigin: 'top left',
            }}
            data-testid="screen-canvas"
          >
            <div style={{ width: SCREEN_DESIGN_WIDTH, height: SCREEN_DESIGN_HEIGHT }}>
              <ScreenPage
                doc={doc}
                pageId={activePageId}
                chartsById={chartsById}
                fieldNames={fieldNames}
                results={results}
              />
            </div>
          </div>
        ) : null}
        {querying && (
          <div className="screen-center screen-querying">
            <Spin size="small" />
          </div>
        )}
      </div>
    </div>
  );
};

export default DashboardScreen;
