// 底部页面标签条：readOnly（移动端阅览模式）只保留「点击切换」，编辑手势全部收敛。
import { fireEvent, render, screen } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { describe, expect, it, vi } from 'vitest';
import DashboardPageTabs from '../../components/DashboardPageTabs';
import { messages } from '../../i18n/useLocale';
import type { DashboardPage } from '../../lib/dashboardLayoutSchema';

const pages: DashboardPage[] = [
  { id: 'p-1', name: '页面 1' },
  { id: 'p-2', name: '页面 2' },
];

const renderTabs = (readOnly?: boolean) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      <DashboardPageTabs
        pages={pages}
        activePageId="p-1"
        canRemove={pages.length > 1}
        readOnly={readOnly}
        onActivate={vi.fn()}
        onAdd={vi.fn()}
        onRename={vi.fn()}
        onDuplicate={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
      />
    </IntlProvider>
  );

describe('DashboardPageTabs readOnly', () => {
  it('编辑态：新增 / ⋮ 菜单 / 拖拽把手都在', () => {
    renderTabs();
    expect(screen.getByTestId('dashboard-page-add')).toBeInTheDocument();
    expect(screen.getAllByTestId('dashboard-page-actions')).toHaveLength(2);
    expect(screen.getAllByTestId('dashboard-page-drag')).toHaveLength(2);
  });

  it('只读态：新增 / ⋮ 菜单 / 拖拽把手全部隐藏，但切换仍可用', () => {
    const onActivate = vi.fn();
    render(
      <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
        <DashboardPageTabs
          pages={pages}
          activePageId="p-1"
          canRemove={pages.length > 1}
          readOnly
          onActivate={onActivate}
          onAdd={vi.fn()}
          onRename={vi.fn()}
          onDuplicate={vi.fn()}
          onRemove={vi.fn()}
          onReorder={vi.fn()}
        />
      </IntlProvider>
    );
    expect(screen.queryByTestId('dashboard-page-add')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-page-actions')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-page-drag')).not.toBeInTheDocument();

    fireEvent.click(screen.getAllByTestId('dashboard-page-label')[1]);
    expect(onActivate).toHaveBeenCalledWith('p-2');
  });
});
