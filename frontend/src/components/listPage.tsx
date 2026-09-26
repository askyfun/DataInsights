import { Input, Space } from 'antd';
import React from 'react';

/**
 * 列表页复用件（设计规范见 docs/developer-guide/list-page-conventions.md）。
 *
 * 收拢三件全站列表页都一样、却各自手写导致漂移的东西：
 *  - ListSearch：卡片工具栏的关键字搜索框（客户端过滤）；
 *  - makeTimeSorter：时间列排序比较器（空值/非法日期沉底 + 同键按 id 兜底防抖动）；
 *  - standardListPagination：标准分页配置。
 */

export const ListSearch: React.FC<{
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  extra?: React.ReactNode;
}> = ({ placeholder, value, onChange, extra }) => (
  <Space wrap>
    <Input.Search
      placeholder={placeholder}
      allowClear
      style={{ width: 300 }}
      onChange={(e) => onChange(e.target.value)}
      value={value}
    />
    {extra}
  </Space>
);

/**
 * 时间列排序比较器工厂：`sorter: makeTimeSorter<Dataset>((r) => r.created_at)`。
 * 空值/非法日期沉底；同键值时按 id 降序兜底——实测存在大量 created_at 完全相同的行
 * （批量导入），只按时间排会同键抖动、看起来像排序坏了。
 */
export const makeTimeSorter =
  <T extends { id: number }>(getTime: (row: T) => string | undefined | null) =>
  (a: T, b: T): number => {
    const ta = Date.parse(getTime(a) ?? '');
    const tb = Date.parse(getTime(b) ?? '');
    const va = Number.isNaN(ta) ? Number.NEGATIVE_INFINITY : ta;
    const vb = Number.isNaN(tb) ? Number.NEGATIVE_INFINITY : tb;
    if (va !== vb) return va < vb ? -1 : 1;
    return b.id - a.id;
  };

/** 标准列表分页：一页 10 条、可换页大小、显示总数。 */
export const standardListPagination = {
  pageSize: 10,
  showSizeChanger: true,
  showTotal: (total: number) => `Total ${total} items`,
};
