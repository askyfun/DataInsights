// 首启向导的组件契约（#226 回归现场）：
//   - 建好管理员后必须把「向导进行中」写进会话并回调 onAdminCreated——否则 Guard 换判据
//     的瞬间就把这个组件卸载，②载示例 / ③出第一张图 永远到不了；
//   - 载入示例时必须顺手配好一张图，第③步给的按钮直接指向这条记录（真能看到图），
//     配不出来也不能卡住流程（降级成空画布）。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import FirstRunGuide from '../../components/FirstRunGuide';
import { messages } from '../../i18n/useLocale';
import { SETUP_IN_PROGRESS_KEY } from '../../lib/firstRun';

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

vi.mock('axios', () => ({
  default: { create: () => instance },
}));

const envelope = (data: unknown) => ({ data: { code: 20000, msg: 'success', trace: '', data } });

const datasetRow = {
  id: 7,
  name: '示例：电商订单',
  datasource_id: 3,
  table_name: 'di_sample_orders',
  query_sql: null,
  query_type: 'table',
  mode: 'direct',
  description: '内置示例数据，可一键移除',
  tags: '["sample"]',
  columns: '[]',
  quality_rules: '[]',
  shard_enabled: false,
  shard_keys: '[]',
  created_at: '2026-11-16T00:00:00Z',
  updated_at: '2026-11-16T00:00:00Z',
  owner_name: null,
};

const columnRow = (over: Record<string, unknown>) => ({
  expr: over.name,
  comment: '',
  role: 'dimension',
  ...over,
});

const renderGuide = (onAdminCreated = vi.fn(), onDone = vi.fn()) => {
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN'] as Record<string, string>}>
      <MemoryRouter initialEntries={['/']}>
        <FirstRunGuide onDone={onDone} onAdminCreated={onAdminCreated} />
      </MemoryRouter>
    </IntlProvider>
  );
  return { onAdminCreated, onDone };
};

const fillAndCreateAdmin = async () => {
  fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'admin' } });
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'Passw0rd!' } });
  fireEvent.change(screen.getByLabelText('Bootstrap 令牌'), { target: { value: 'boot-token' } });
  fireEvent.click(screen.getByRole('button', { name: '创建管理员并继续' }));
};

describe('FirstRunGuide（首启向导）', () => {
  beforeEach(() => {
    sessionStorage.clear();
    instance.get.mockReset();
    instance.post.mockReset();
    instance.get.mockResolvedValue(envelope({ enforce_mode: 'shadow', needs_bootstrap: false }));
  });

  it('建好管理员后写「向导进行中」并回调 onAdminCreated（否则向导自我卸载）', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/auth/register') {
        return Promise.resolve(
          envelope({
            token: 'tok',
            user: {
              id: 1,
              username: 'admin',
              display_name: 'admin',
              role: 'admin',
              disabled: false,
              created_at: '2026-11-16T00:00:00Z',
            },
          })
        );
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });

    const { onAdminCreated } = renderGuide();
    await fillAndCreateAdmin();

    await waitFor(() => expect(onAdminCreated).toHaveBeenCalledTimes(1));
    expect(sessionStorage.getItem(SETUP_IN_PROGRESS_KEY)).toBe('1');
    // 第②步的入口必须出现：证明组件没有在「建号成功」这一刻被卸载/重置。
    expect(await screen.findByRole('button', { name: '载入示例数据' })).toBeInTheDocument();
  });

  it('载入示例后直接给出可打开的示例图（真能看到渲染结果，而不是空画布）', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/auth/register') {
        return Promise.resolve(
          envelope({
            token: 'tok',
            user: {
              id: 1,
              username: 'admin',
              display_name: 'admin',
              role: 'admin',
              disabled: false,
              created_at: '2026-11-16T00:00:00Z',
            },
          })
        );
      }
      if (url === '/api/sample-dataset/load') {
        return Promise.resolve(envelope({ loaded: true, dismissed: false }));
      }
      if (url === '/api/queries') {
        return Promise.resolve(
          envelope({
            query_id: 'CSatF9qXyRoQXFtAA3iZS',
            created_at: '2026-11-16T00:00:00Z',
            expires_at: null,
          })
        );
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });
    instance.get.mockImplementation((url: string) => {
      if (url === '/api/datasets') {
        return Promise.resolve(envelope({ items: [datasetRow], total: 1 }));
      }
      if (url === '/api/datasets/7/columns') {
        return Promise.resolve(
          envelope([
            columnRow({ id: 'c-date', name: 'order_date', type: 'date' }),
            columnRow({ id: 'c-amount', name: 'amount', type: 'float', role: 'metric' }),
          ])
        );
      }
      return Promise.resolve(envelope({ enforce_mode: 'shadow', needs_bootstrap: false }));
    });

    renderGuide();
    await fillAndCreateAdmin();
    fireEvent.click(await screen.findByRole('button', { name: '载入示例数据' }));

    const openChart = await screen.findByRole('button', { name: '看看这张图' });
    expect(openChart).toBeInTheDocument();
    // 文案里点明配的是什么，用户才知道这张图不是随机来的。
    expect(screen.getByText(/order_date/)).toBeInTheDocument();
    // 落库请求带的是示例数据集与一份 v1 信封（后端按它校验）。
    const saveCall = instance.post.mock.calls.find(([url]) => url === '/api/queries');
    expect(saveCall?.[1]?.dataset_id).toBe(7);
    expect(saveCall?.[1]?.spec?.v).toBe(1);
  });

  it('配示例图失败时降级成空画布，不把用户堵在第③步', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/auth/register') {
        return Promise.resolve(
          envelope({
            token: 'tok',
            user: {
              id: 1,
              username: 'admin',
              display_name: 'admin',
              role: 'admin',
              disabled: false,
              created_at: '2026-11-16T00:00:00Z',
            },
          })
        );
      }
      if (url === '/api/sample-dataset/load') {
        return Promise.resolve(envelope({ loaded: true, dismissed: false }));
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });
    instance.get.mockImplementation((url: string) => {
      if (url === '/api/datasets') {
        return Promise.reject(new Error('boom'));
      }
      return Promise.resolve(envelope({ enforce_mode: 'shadow', needs_bootstrap: false }));
    });

    renderGuide();
    await fillAndCreateAdmin();
    fireEvent.click(await screen.findByRole('button', { name: '载入示例数据' }));

    // 没有示例图时退回「去建第一张图」，第③步仍然可用。
    expect(await screen.findByRole('button', { name: '去建第一张图' })).toBeInTheDocument();
  });
});
