import type { TableProps } from 'antd';
import { Empty, Table } from 'antd';
import { useMemo } from 'react';
import { formatMetricValue } from '@/lib/format';
import LoadingPlaceholder from '../LoadingPlaceholder';

interface TableChartProps {
  data: any[];
  loading: boolean;
  columns?: string[];
  columnLabels?: Record<string, string>;
  /** 维度字段名列表，按用户拖入顺序 */
  dimensionNames?: string[];
  /** 指标字段名列表，按用户拖入顺序 */
  metricNames?: string[];
  /** 指标列的「格式」配置（键为输出列名）：如 0,0.00 → 千分位 + 两位小数。 */
  metricFormats?: Record<string, string>;
  rowSize?: 'small' | 'middle' | 'large';
  pagination?: {
    page: number;
    pageSize: number;
    total: number;
  };
  /**
   * 当前生效的排序列（输出列名）与方向，由外部（queryConfig.sort）驱动。
   * 传入即进入受控模式：表头箭头只反映这里的状态，不会自己变——服务端排序必须由
   * 父组件回写状态才算生效，否则箭头会与数据不一致。
   */
  sortField?: string;
  sortOrder?: 'asc' | 'desc';
  onPageChange?: (page: number, pageSize: number) => void;
  /** 传 null 表示用户取消了排序（第三次点击表头）。未传 onSortChange 时不渲染排序箭头。 */
  onSortChange?: (sort: { field: string; order: 'asc' | 'desc' } | null) => void;
}

const TableChart: React.FC<TableChartProps> = ({
  data,
  loading,
  columns: propColumns,
  columnLabels,
  dimensionNames,
  metricNames,
  metricFormats,
  rowSize = 'small',
  pagination,
  sortField,
  sortOrder,
  onPageChange,
  onSortChange,
}) => {
  const columns: TableProps<any>['columns'] = useMemo(() => {
    const keys =
      propColumns && propColumns.length > 0
        ? propColumns
        : data && data.length > 0
          ? Object.keys(data[0])
          : [];

    // 按维度顺序 + 指标顺序排列，剩余字段追加到末尾
    const orderedKeys: string[] = [];
    const keySet = new Set(keys);

    if (dimensionNames?.length || metricNames?.length) {
      // 先按维度顺序
      for (const name of dimensionNames || []) {
        if (keySet.has(name)) {
          orderedKeys.push(name);
          keySet.delete(name);
        }
      }
      // 再按指标顺序
      for (const name of metricNames || []) {
        if (keySet.has(name)) {
          orderedKeys.push(name);
          keySet.delete(name);
        }
      }
      // 剩余字段追加
      for (const key of keys) {
        if (keySet.has(key)) {
          orderedKeys.push(key);
        }
      }
    } else {
      orderedKeys.push(...keys);
    }

    return orderedKeys.map((key) => {
      const format = metricFormats?.[key];
      return {
        title: columnLabels?.[key] || key,
        dataIndex: key,
        key,
        // 没有排序回调时（如分享页只读表格）不挂 sorter：避免渲染一个点了没反应的表头箭头。
        sorter: Boolean(onSortChange),
        // 受控排序：只有当前生效的排序列显示箭头状态，其余列恒为 null。
        sortOrder: onSortChange && key === sortField ? toAntdSortOrder(sortOrder) : null,
        ellipsis: true,
        // 「格式」只作用于指标列的展示层：排序仍按原始行值比较。
        ...(format ? { render: (value: unknown) => formatMetricValue(value, format) } : {}),
      };
    });
  }, [
    columnLabels,
    data,
    propColumns,
    dimensionNames,
    metricNames,
    metricFormats,
    onSortChange,
    sortField,
    sortOrder,
  ]);

  const handleTableChange: TableProps<any>['onChange'] = (
    tablePagination,
    _filters,
    sorter,
    extra
  ) => {
    // 分页：仅当页码/每页条数真的变了才回调。antd 的 onChange 在点击表头排序时也会触发，
    // 以前无条件回调会额外发一次不带 sort 的查询，与排序请求竞态、可能用旧结果覆盖新结果。
    if (onPageChange && pagination) {
      const nextPage = tablePagination?.current || 1;
      const nextPageSize = tablePagination?.pageSize || 10;
      if (nextPage !== pagination.page || nextPageSize !== pagination.pageSize) {
        onPageChange(nextPage, nextPageSize);
      }
    }

    if (onSortChange && sorter && !Array.isArray(sorter)) {
      const clickedField = sorter.field as string | undefined;
      const nextOrder =
        sorter.order === 'ascend' ? 'asc' : sorter.order === 'descend' ? 'desc' : null;

      if (nextOrder === null && extra?.action === 'sort') {
        // 取消排序（第三次点击表头）：antd 走的是 legacy 兼容分支，回传的 sorter
        // 不带 field，无从判断点的是哪一列；但同一时刻只可能有一列处于排序态，
        // 因此"取消"必然作用于当前 sortField。
        if (sortField) {
          onSortChange(null);
        }
        return;
      }

      // 与当前受控状态相同则不下发，避免重复查询。
      const isSameAsCurrent = clickedField === sortField && nextOrder === (sortOrder ?? null);
      if (clickedField && nextOrder && !isSameAsCurrent) {
        onSortChange({ field: clickedField, order: nextOrder });
      }
    }
  };

  if (loading) {
    return <LoadingPlaceholder text="加载数据中..." />;
  }

  if (!data || data.length === 0) {
    return (
      <Empty
        description="暂无数据"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        style={{ padding: '100px 0' }}
      />
    );
  }

  return (
    <Table
      dataSource={data.map((item, index) => ({ ...item, key: index }))}
      columns={columns}
      pagination={
        pagination
          ? {
              current: pagination.page,
              pageSize: pagination.pageSize,
              total: pagination.total,
              showSizeChanger: true,
              pageSizeOptions: ['10', '20', '50', '100'],
              showTotal: (total) => `总计 ${total} 条`,
              showQuickJumper: true,
            }
          : {
              defaultPageSize: 10,
              showSizeChanger: true,
              pageSizeOptions: ['10', '20', '50', '100'],
              showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} items`,
            }
      }
      bordered
      size={rowSize}
      scroll={{ x: 'max-content' }}
      onChange={handleTableChange}
    />
  );
};

/** asc/desc（wire 口径）→ antd 的 ascend/descend；缺省 null 表示无排序。 */
const toAntdSortOrder = (order?: 'asc' | 'desc'): 'ascend' | 'descend' | null => {
  if (order === 'asc') return 'ascend';
  if (order === 'desc') return 'descend';
  return null;
};

export default TableChart;
