// 盘级筛选器块：编辑类回调（scope/配置/移除）改为可选后，缺省即不渲染对应控件
// ——移动端阅览模式靠这条收敛编辑入口；取值与算子（查询语义）保持可用。
import { render, screen } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { describe, expect, it, vi } from 'vitest';
import DashboardFilterBlock from '../../components/DashboardFilterBlock/DashboardFilterBlock';
import { messages } from '../../i18n/useLocale';
import type { DashboardFilterWidget } from '../../lib/dashboardLayoutSchema';

vi.mock('../../api', () => ({
  datasetsApi: {
    queryDistinct: vi
      .fn()
      .mockResolvedValue({ data: { code: 20000, msg: 'success', trace: '', data: [] } }),
  },
}));

const widget: DashboardFilterWidget = {
  type: 'filter',
  widgetId: 'w-f',
  pageId: 'p-1',
  binding: { datasetId: 1, column: 'city' },
  label: '城市',
  dataType: 'string',
  operator: 'in',
  multi: true,
  x: 0,
  y: 0,
  w: 3,
  h: 3,
};

const renderBlock = (props: Partial<Parameters<typeof DashboardFilterBlock>[0]> = {}) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <DashboardFilterBlock
        widget={widget}
        value={[]}
        onChange={vi.fn()}
        onOperatorChange={vi.fn()}
        {...props}
      />
    </IntlProvider>
  );

describe('DashboardFilterBlock 可选编辑回调', () => {
  it('回调齐全（桌面编辑态）：scope / 移除控件都渲染', () => {
    renderBlock({ onScopeChange: vi.fn(), onRemove: vi.fn() });
    expect(screen.getByTestId('dashboard-filter-scope')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-filter-remove')).toBeInTheDocument();
  });

  it('缺省（移动端阅览态）：scope / 配置 / 移除都不渲染，取值控件仍在', () => {
    renderBlock();
    expect(screen.queryByTestId('dashboard-filter-scope')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-filter-configure')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-filter-remove')).not.toBeInTheDocument();
    // 字符串族的主取值控件（枚举选择）仍可交互：块标题（取值区的 label）仍在。
    expect(screen.getAllByText('城市').length).toBeGreaterThan(0);
  });
});
