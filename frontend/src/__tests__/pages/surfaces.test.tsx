// 表面系统（Surface System）的结构契约回归测试。
//
// 为什么需要它：设计规则里最容易悄悄失效的一条是「页头坐在画布上、不在卡片里」。
// 只要有人把标题塞回 `<Card>`（哪怕只是为了让某个页面的间距好看一点），页面就会
// 退回"白底白卡"—— 卡内同时存在背景白、卡头白、标题区白三层同色。这种退化在
// 代码评审里极难看出来（JSX 差异只有几行缩进），但 `closest('.ant-card')` 一眼可判。
//
// 断言范围刻意只覆盖骨架与从属关系，不涉及任何颜色值 —— 色值归 styles/index.css
// 的 token 表管理，测试里复制一份色值只会制造第二份事实源。
import { render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { messages } from '../../i18n/useLocale';
import ChartsPage from '../../pages/Charts';
import DatasetDetailPage from '../../pages/DatasetDetail';
import DatasourcePage from '../../pages/Datasource';
import SharePage from '../../pages/Share';

const { instance } = vi.hoisted(() => {
  const inst = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  };
  return { instance: inst };
});

vi.mock('axios', () => ({ default: { create: () => instance } }));

class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver);

const envelope = (data: unknown) => ({ data: { code: 20000, msg: 'success', trace: '', data } });

beforeEach(() => {
  vi.clearAllMocks();
  instance.get.mockImplementation((url: string) => {
    if (url === '/api/datasources') return Promise.resolve(envelope([{ id: 2, name: 'pg' }]));
    if (url.endsWith('/columns')) return Promise.resolve(envelope([]));
    if (url.endsWith('/preview')) return Promise.resolve(envelope({ columns: [], data: [] }));
    if (url.startsWith('/api/datasources/'))
      return Promise.resolve(envelope({ id: 2, name: 'pg' }));
    if (url === '/api/datasets') return Promise.resolve(envelope([]));
    if (url.startsWith('/api/datasets/'))
      return Promise.resolve(
        envelope({
          id: 4,
          name: 'sales',
          datasource_id: 2,
          table_name: 'orders',
          query_type: 'table',
          columns: '[]',
        })
      );
    return Promise.resolve(envelope([]));
  });
});

const wrap = (ui: React.ReactElement, path = '/') =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter>
    </IntlProvider>
  );

/** 结构契约：存在页面骨架，页头存在，且页头**不在**任何卡片内部。 */
const assertSurface = (expectedTitle: string) => {
  const header = document.querySelector('.dr-page-header');
  expect(header).not.toBeNull();
  expect(header?.closest('.ant-card')).toBeNull();
  expect(header?.querySelector('h1')?.textContent).toBe(expectedTitle);
  expect(document.querySelector('.dr-page')?.contains(header as Node)).toBe(true);
};

describe('表面系统 · 页面骨架', () => {
  it('图表列表页：页头在画布上、不在卡里', async () => {
    wrap(<ChartsPage />);
    await waitFor(() => expect(document.querySelector('.dr-page')).not.toBeNull());
    assertSurface('图表');
  });

  it('数据源列表页：页头在画布上、不在卡里', async () => {
    wrap(<DatasourcePage />);
    await waitFor(() => expect(document.querySelector('.dr-page')).not.toBeNull());
    assertSurface('数据源');
  });

  it('分享列表页：页头在画布上、不在卡里', async () => {
    wrap(<SharePage />);
    await waitFor(() => expect(document.querySelector('.dr-page')).not.toBeNull());
    assertSurface('分享管理');
  });

  it('数据集详情页：面包屑与页头同处画布层', async () => {
    wrap(
      <Routes>
        <Route path="/datasets/:id" element={<DatasetDetailPage />} />
      </Routes>,
      '/datasets/4'
    );
    await waitFor(() => expect(document.querySelector('.dr-page-header')).not.toBeNull());
    const header = document.querySelector('.dr-page-header');
    expect(header?.closest('.ant-card')).toBeNull();
    expect(header?.querySelector('.dr-page-header__crumb')).not.toBeNull();
    // 数据集名同时出现在面包屑末项与页头标题，两处都应存在
    expect(header?.querySelector('h1')?.textContent).toBe('sales');
    expect(screen.getAllByText('sales').length).toBeGreaterThanOrEqual(2);
  });

  it('页面根节点就是 .dr-page（中间不再套一层 padding:24 的白底 div）', async () => {
    const { container } = wrap(<ChartsPage />);
    await waitFor(() => expect(container.querySelector('.dr-page')).not.toBeNull());
    expect((container.firstChild as HTMLElement).className).toBe('dr-page');
  });
});
