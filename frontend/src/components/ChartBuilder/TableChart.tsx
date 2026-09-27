import type { TableProps } from 'antd';
import { Empty, Table } from 'antd';
import type { ReactNode } from 'react';
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
  /** 是否在最左侧插入序号列（跨分页连续编号）。 */
  showIndex?: boolean;
  /** 单元格自动换行：true 时取消 ellipsis 截断，长文本折行显示。 */
  wordWrap?: boolean;
  /** 空值显示：把 NULL/空字符串统一渲染为占位符；'raw'（缺省）保持原样。 */
  nullDisplay?: 'raw' | 'dash' | 'blank' | 'zero';
  /** 冻结维度列：横向滚动时把维度列（含序号列）固定在左侧。 */
  freezeDimensions?: boolean;
  /**
   * 合计行（issue #131）：键为指标列的输出列名，值由后端在过滤后的**完整数据集**上
   * 重算。传入即在表尾渲染一行合计；缺省不渲染。
   * ⚠️ 不在前端拿当前页明细行相加——服务端分页下那只是某一页的和，
   * AVG / COUNT(DISTINCT) 更是会算成「平均数的平均数」。
   */
  totalRow?: Record<string, unknown>;
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

/** 序号列的常量 dataIndex（不参与取数，仅占位）；合计行据此把它与真实数据列区分开。 */
const INDEX_COLUMN_DATA_INDEX = '__row_index__';

const TableChart: React.FC<TableChartProps> = ({
  data,
  loading,
  columns: propColumns,
  columnLabels,
  dimensionNames,
  metricNames,
  metricFormats,
  showIndex,
  wordWrap,
  nullDisplay,
  freezeDimensions,
  totalRow,
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

    const nullText = nullPlaceholder(nullDisplay);
    const isNullish = (value: unknown) => value === null || value === undefined || value === '';

    const dimensionSet = new Set(dimensionNames || []);
    const freeze = Boolean(freezeDimensions) && dimensionSet.size > 0;

    const dataColumns = orderedKeys.map((key) => {
      const format = metricFormats?.[key];
      // 只在「真的有事要做」时覆盖 render（配了空值占位、或指标列有格式）；
      // 否则保持 antd 默认渲染——为普通列无条件挂 render 会干扰受控排序箭头的重渲染。
      const nullAware = nullText !== null;
      const needsRender = nullAware || Boolean(format);
      return {
        title: columnLabels?.[key] || key,
        dataIndex: key,
        key,
        // 没有排序回调时（如分享页只读表格）不挂 sorter：避免渲染一个点了没反应的表头箭头。
        sorter: Boolean(onSortChange),
        // 受控排序：只有当前生效的排序列显示箭头状态，其余列恒为 null。
        sortOrder: onSortChange && key === sortField ? toAntdSortOrder(sortOrder) : null,
        // 自动换行开启时取消 ellipsis；否则沿用截断。
        ellipsis: !wordWrap,
        // 冻结时把维度列固定在左侧（指标列不固定，避免全表锁死无法横向看指标）。
        ...(freeze && dimensionSet.has(key) ? { fixed: 'left' as const } : {}),
        // 展示层渲染：空值按 nullDisplay 占位，指标列套格式（'raw' 且无格式时不进入这里）。
        ...(needsRender
          ? {
              render: (value: unknown) => {
                if (nullAware && isNullish(value)) {
                  return nullText;
                }
                if (format) {
                  return formatMetricValue(value, format);
                }
                return value === null || value === undefined ? '' : String(value);
              },
            }
          : {}),
      };
    });

    if (!showIndex) {
      return dataColumns;
    }
    // 序号列：跨服务端分页连续编号（当前页行下标 + 页偏移），冻结时随维度列固定在左。
    const pageIndex = pagination ? (pagination.page - 1) * pagination.pageSize : 0;
    const indexColumn = {
      title: '#',
      dataIndex: INDEX_COLUMN_DATA_INDEX,
      key: '__row_index__',
      width: 56,
      align: 'center' as const,
      ellipsis: false,
      ...(freeze ? { fixed: 'left' as const } : {}),
      render: (_value: unknown, _record: unknown, index: number) => pageIndex + index + 1,
    };
    return [indexColumn, ...dataColumns];
  }, [
    columnLabels,
    data,
    propColumns,
    dimensionNames,
    metricNames,
    metricFormats,
    showIndex,
    wordWrap,
    nullDisplay,
    freezeDimensions,
    pagination,
    onSortChange,
    sortField,
    sortOrder,
  ]);

  // 合计行（issue #131）：指标列显示后端在完整数据集上重算的值，「合计」标签落在
  // 第一个非指标列（维度列；没有维度列时落到序号列）。直接复用 columns 的渲染器，
  // 于是「格式」「空值显示」与明细行同口径，序号列/冻结列的位置也不会数错。
  const summary = useMemo<(() => ReactNode) | null>(() => {
    if (!totalRow || !columns) {
      return null;
    }
    const metricSet = new Set(metricNames || []);
    // 标签候选 = 第一个「真实数据列且不是指标列」。序号列（常量 dataIndex）被排除：
    // 它没有 dataIndex，任何情况下都不该拿 totalRow 的值或当标签位。
    const isIndexColumn = (col: object) =>
      'dataIndex' in col && col.dataIndex === INDEX_COLUMN_DATA_INDEX;
    let labelCell = columns.findIndex(
      (col) => 'dataIndex' in col && !isIndexColumn(col) && !metricSet.has(col.dataIndex as string)
    );
    // 没有维度列时把标签退到序号列；只有一列时不放——那一列要留给合计数值本身，
    // 放标签会把值挤掉。
    if (labelCell === -1 && columns.length > 1) {
      labelCell = 0;
    }
    return () => (
      <Table.Summary fixed>
        <Table.Summary.Row>
          {columns.map((col, i) => {
            const key = 'key' in col && typeof col.key === 'string' ? col.key : String(i);
            if (i === labelCell) {
              return (
                <Table.Summary.Cell key={key} index={i}>
                  合计
                </Table.Summary.Cell>
              );
            }
            if (isIndexColumn(col)) {
              return (
                <Table.Summary.Cell key={key} index={i}>
                  {''}
                </Table.Summary.Cell>
              );
            }
            // 复用列自己的 render：合计值与明细值走同一条展示层（「格式」「空值显示」
            // 口径一致），不会出现同一列两种小数位数。
            const dataIndex = 'dataIndex' in col ? String(col.dataIndex) : '';
            const render = (col as { render?: (value: unknown) => ReactNode }).render;
            const value = totalRow[dataIndex];
            return (
              <Table.Summary.Cell key={key} index={i}>
                {render ? render(value) : value == null ? '' : String(value)}
              </Table.Summary.Cell>
            );
          })}
        </Table.Summary.Row>
      </Table.Summary>
    );
  }, [totalRow, columns, metricNames]);

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

  // 明细为空但有合计时不显示空态：过滤后确实没有明细行，全集聚合仍可能给出数值。
  if ((!data || data.length === 0) && !totalRow) {
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
      summary={summary ?? (() => null)}
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
              // 非受控分页（透视表 v1 回退等）：与 store 默认一致，一页 100 条。
              defaultPageSize: 100,
              showSizeChanger: true,
              pageSizeOptions: ['10', '20', '50', '100'],
              showTotal: (total) => `总计 ${total} 条`,
            }
      }
      bordered
      size={rowSize}
      scroll={{ x: 'max-content' }}
      onChange={handleTableChange}
    />
  );
};

/**
 * 空值占位符：把渲染层的 NULL/空字符串替换为可读标记。
 * 'raw'（或缺省）返回 null，表示不拦截、沿用既有"原样输出空单元格"行为。
 */
const nullPlaceholder = (mode?: 'raw' | 'dash' | 'blank' | 'zero'): string | null => {
  if (mode === 'dash') return '--';
  if (mode === 'blank') return '';
  if (mode === 'zero') return '0';
  return null;
};

/** asc/desc（wire 口径）→ antd 的 ascend/descend；缺省 null 表示无排序。 */
const toAntdSortOrder = (order?: 'asc' | 'desc'): 'ascend' | 'descend' | null => {
  if (order === 'asc') return 'ascend';
  if (order === 'desc') return 'descend';
  return null;
};

export default TableChart;
