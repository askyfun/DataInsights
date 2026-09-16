import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import KpiCard from '@/components/ChartBuilder/KpiCard';

/**
 * KpiCard（R-51 KPI 单值卡）渲染契约测试：AntD Statistic 渲染 value/label/unit，
 * 不走 ECharts。format 字段本任务仅保留接口不消费（见组件 doc 的已知限制）。
 */
describe('KpiCard', () => {
  it('渲染 label 标题与数值（AntD Statistic 默认千分位分组）', () => {
    render(<KpiCard value={95380} label="total_amount" />);

    expect(screen.getByText('total_amount')).toBeInTheDocument();
    expect(screen.getByText('95,380')).toBeInTheDocument();
  });

  it('unit 作为 Statistic suffix 展示', () => {
    const { container } = render(<KpiCard value={1234.5} label="收入" unit="元" />);

    expect(screen.getByText('收入')).toBeInTheDocument();
    // AntD Statistic 把整数部分（"1,234"）与小数部分（".5"）拆成两个 span，
    // 组合文本断言需读 .ant-statistic-content-value 的 textContent。
    expect(container.querySelector('.ant-statistic-content-value')?.textContent).toBe('1,234.5');
    expect(screen.getByText('元')).toBeInTheDocument();
  });

  it('value 为 0（空数据/NULL 聚合回退）时正常渲染而不是空白', () => {
    render(<KpiCard value={0} label="total" />);

    expect(screen.getByText('total')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });

  it('loading 时展示 Card 骨架屏', () => {
    const { container } = render(<KpiCard value={1} label="total" loading />);

    expect(container.querySelector('.ant-card-loading')).not.toBeNull();
    expect(screen.queryByText('total')).not.toBeInTheDocument();
  });
});
