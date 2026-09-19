import type { TableProps } from 'antd';
import { Empty, Spin, Table, Typography } from 'antd';
import { useMemo } from 'react';
import type { PivotResponseV2, PivotRow } from '../../api';

const { Text } = Typography;

// 镜像后端 query.PivotSubtotalColKey / pivotValueKeySep / PivotValueKey（Go 常量无法
// import，前后端必须逐字一致，否则单元格取值 undefined → 透视表静默全空）。
const PIVOT_SUBTOTAL_COL_KEY = '__subtotal__';
const PIVOT_VALUE_KEY_SEP = '|';
const pivotValueKey = (colHeader: string, metricAlias: string) =>
  colHeader + PIVOT_VALUE_KEY_SEP + metricAlias;

const GRAND_TOTAL_LABEL = '合计';
const SUBTOTAL_COLUMN_TITLE = '小计';

/** antd Table 数据行：dataIndex → 显示值；rowKind 区分明细/小计/合计行（供 rowClassName）。 */
type PivotTableRowRecord = Record<string, string | number>;

interface PivotTableProps {
  data: PivotResponseV2;
  loading?: boolean;
  /** 行维度列名的美化显示名（缺省用 row_headers 原值）。 */
  columnLabels?: Record<string, string>;
}

// 取不到值（undefined）时渲染空字符串，不让 undefined/NaN 字样漏进表格。
const formatCell = (value: number | undefined): string | number =>
  value === undefined ? '' : value;

/**
 * 透视表 v2 交叉表渲染器（R-53）：消费 PivotResponseV2（cells + col_headers +
 * row_headers + metric_names + grand_total），左侧行维度列 + 交叉分组表头 +
 * 最右小计列；小计/合计行经 rowClassName 加类名区分样式。
 */
const PivotTable: React.FC<PivotTableProps> = ({ data, loading, columnLabels }) => {
  const columns = useMemo<TableProps<PivotTableRowRecord>['columns']>(() => {
    // 左侧：每个行维度一列，单元格值取该行 row_key[r]。
    const rowHeaderColumns = data.row_headers.map((header, r) => ({
      title: columnLabels?.[header] || header,
      dataIndex: `rowHeader-${r}`,
      key: `rowHeader-${r}`,
    }));

    // 交叉体：每个 col_header 一个分组列（children = 每 metric 一叶子，antd 嵌套列
    // 自动生成 colSpan 合并的分组表头）。colHeader 逐字用响应 col_headers 原值——
    // 多列维度时后端已用 " - " 拼好，前端不再拼。
    const crossColumns = data.col_headers.map((colHeader, c) => ({
      title: colHeader,
      key: `col-${c}`,
      children: data.metric_names.map((metric, m) => ({
        title: metric,
        dataIndex: `cell-${c}-${m}`,
        key: `cell-${c}-${m}`,
      })),
    }));

    // 最右：小计分组列，值取哨兵键 __subtotal__|<metric>。
    const subtotalColumn = {
      title: SUBTOTAL_COLUMN_TITLE,
      key: 'subtotal',
      children: data.metric_names.map((metric, m) => ({
        title: metric,
        dataIndex: `subtotal-${m}`,
        key: `subtotal-${m}`,
      })),
    };

    return [...rowHeaderColumns, ...crossColumns, subtotalColumn];
  }, [data, columnLabels]);

  const dataSource = useMemo<PivotTableRowRecord[]>(() => {
    const toRecord = (
      row: PivotRow,
      key: string,
      rowKind: 'detail' | 'subtotal' | 'grandTotal'
    ): PivotTableRowRecord => {
      const record: PivotTableRowRecord = { key, rowKind };
      data.row_headers.forEach((_, r) => {
        // grand_total 的 row_key 是 []，左侧第一列显示"合计"标签。
        record[`rowHeader-${r}`] =
          rowKind === 'grandTotal' && r === 0 ? GRAND_TOTAL_LABEL : (row.row_key[r] ?? '');
      });
      data.metric_names.forEach((metric, m) => {
        if (row.is_subtotal) {
          // 小计/合计行只有哨兵键，交叉体列留空。
          record[`subtotal-${m}`] = formatCell(
            row.values[pivotValueKey(PIVOT_SUBTOTAL_COL_KEY, metric)]
          );
        } else {
          // 明细行只有 colHeader|metric 键，小计列留空。
          data.col_headers.forEach((colHeader, c) => {
            record[`cell-${c}-${m}`] = formatCell(row.values[pivotValueKey(colHeader, metric)]);
          });
          record[`subtotal-${m}`] = '';
        }
      });
      return record;
    };

    const records = data.cells.map((row, index) =>
      toRecord(row, `cell-row-${index}`, row.is_subtotal ? 'subtotal' : 'detail')
    );
    if (data.grand_total) {
      records.push(toRecord(data.grand_total, 'grand-total', 'grandTotal'));
    }
    return records;
  }, [data]);

  const rowClassName = (record: PivotTableRowRecord): string => {
    if (record.rowKind === 'grandTotal') return 'pivot-grand-total-row';
    if (record.rowKind === 'subtotal') return 'pivot-subtotal-row';
    return '';
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">加载数据中...</Text>
        </div>
      </div>
    );
  }

  if (data.cells.length === 0) {
    return (
      <Empty
        description="暂无数据"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        style={{ padding: '100px 0' }}
      />
    );
  }

  return (
    <div className="pivot-table-v2">
      <style>{`
        /* 冷调灰阶：与全局斑马纹（--dr-sunken）同族。原为 #fafafa / #f0f0f0（暖调），
           和表体一起会显出轻微的"发黄"，与页面其余灰面不同源。
           token 挂在 :root 上，故本组件在 ShareView 下渲染时同样取得到值。 */
        .pivot-table-v2 .pivot-subtotal-row > td { font-weight: 600; background: var(--dr-sunken); }
        .pivot-table-v2 .pivot-grand-total-row > td { font-weight: 700; background: var(--dr-canvas); }
      `}</style>
      <Table
        dataSource={dataSource}
        columns={columns}
        rowClassName={rowClassName}
        pagination={false}
        bordered
        size="small"
        scroll={{ x: 'max-content' }}
      />
    </div>
  );
};

export default PivotTable;
