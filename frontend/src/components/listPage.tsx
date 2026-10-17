import type { TableProps } from 'antd';
import { Input, Space } from 'antd';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import type { ListOrder, ListPage, ListQuery } from '../api';

/**
 * 列表页复用件（设计规范见 docs/developer-guide/list-page-conventions.md）。
 *
 *  - ListSearch：卡片工具栏的关键字搜索框（关键字随查询下发到服务端）；
 *  - standardListPagination：标准分页的外观参数（服务端分页由 useServerList 驱动）；
 *  - useServerList：四类实体列表共用的服务端分页 / 排序 / 关键字状态机。
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

/** 标准列表分页：一页 10 条、可换页大小、显示总数。 */
export const standardListPagination = {
  pageSize: 10,
  showSizeChanger: true,
  showTotal: (total: number) => `Total ${total} items`,
};

/** 关键字输入防抖：避免每敲一个字都打一次服务端查询。 */
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/**
 * 受控列头排序指示：`sortOrder` 必须由查询状态派生，否则分页往返后箭头会停在
 * 上一次点击的位置（antd 的 defaultSortOrder 只对客户端排序有效）。
 */
export const sortOrderOf = (query: ListQuery, field: string): 'ascend' | 'descend' | null =>
  query.sort === field ? (query.order === 'asc' ? 'ascend' : 'descend') : null;

export interface ServerListOptions {
  pageSize?: number;
  /** 默认排序（与后端缺省 ORDER BY 对齐）。 */
  sort?: string;
  order?: ListOrder;
  showTotal?: (total: number) => string;
}

export interface ServerListState<T> {
  items: T[];
  total: number;
  loading: boolean;
  error: string | null;
  query: ListQuery;
  reload: () => void;
  /** 改筛选条件：offset 回到 0（换了筛选还停在第 3 页没有意义）。 */
  setParam: (patch: Partial<ListQuery>) => void;
  onTableChange: NonNullable<TableProps<T>['onChange']>;
  pagination: {
    current: number;
    pageSize: number;
    total: number;
    showSizeChanger: boolean;
    showTotal: (total: number) => string;
  };
}

/**
 * 服务端分页状态机：查询参数一变就重拉，Table 的 onChange 直接接到
 * onTableChange。fetchPage 用 ref 持有，页面传内联箭头函数也不会造成死循环。
 */
export function useServerList<T>(
  fetchPage: (query: ListQuery) => Promise<ListPage<T>>,
  options: ServerListOptions = {}
): ServerListState<T> {
  const defaultPageSize = options.pageSize ?? 10;
  const fetchRef = useRef(fetchPage);
  fetchRef.current = fetchPage;

  const [query, setQuery] = useState<ListQuery>({
    limit: defaultPageSize,
    offset: 0,
    sort: options.sort,
    order: options.order,
  });
  const [items, setItems] = useState<T[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const queryRef = useRef(query);
  queryRef.current = query;
  // 只认最后一次请求的结果：筛选连续变化时，先发的请求不能覆盖后发的。
  const requestId = useRef(0);

  const load = useCallback(async (next: ListQuery) => {
    const id = ++requestId.current;
    setLoading(true);
    try {
      const page = await fetchRef.current(next);
      if (id !== requestId.current) return;
      setItems(page.items ?? []);
      setTotal(page.total ?? 0);
      setError(null);
    } catch (err: any) {
      if (id !== requestId.current) return;
      setError(err?.message || 'Failed to fetch');
    } finally {
      if (id === requestId.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(query);
  }, [load, query]);

  const reload = useCallback(() => {
    void load(queryRef.current);
  }, [load]);

  const setParam = useCallback((patch: Partial<ListQuery>) => {
    setQuery((prev) => ({ ...prev, ...patch, offset: 0 }));
  }, []);

  const onTableChange = useCallback<NonNullable<TableProps<T>['onChange']>>(
    (pagination, _filters, sorter) => {
      const active = Array.isArray(sorter) ? sorter[0] : sorter;
      const limit = pagination.pageSize ?? defaultPageSize;
      const current = pagination.current ?? 1;
      const field = active?.field;
      const direction = active?.order;
      setQuery((prev) => ({
        ...prev,
        limit,
        // 换页大小后停在第 N 页没有可靠语义（旧 offset 可能越界），回到第一页。
        offset: limit !== (prev.limit ?? defaultPageSize) ? 0 : (current - 1) * limit,
        // 第三次点击列头会清空排序，此时回到该实体的默认排序。
        sort: field && direction ? String(field) : options.sort,
        order: field && direction ? (direction === 'ascend' ? 'asc' : 'desc') : options.order,
      }));
    },
    [defaultPageSize, options.sort, options.order]
  );

  const limit = query.limit ?? defaultPageSize;
  const pagination = {
    current: Math.floor((query.offset ?? 0) / limit) + 1,
    pageSize: limit,
    total,
    showSizeChanger: true,
    showTotal: options.showTotal ?? ((n: number) => `Total ${n} items`),
  };

  return {
    items,
    total,
    loading,
    error,
    query,
    reload,
    setParam,
    onTableChange,
    pagination,
  };
}
