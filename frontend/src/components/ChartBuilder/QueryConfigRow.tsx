import React from 'react';
import type { BoundField, ChartField } from '@/store';
import FieldDropZone, { DropZoneType } from './FieldDropZone';

export interface QueryConfigRowProps {
  /** 行类型 */
  rowType: DropZoneType;
  /** 当前字段组索引 */
  groupIndex?: number;
  /** 自定义标签 */
  label?: string;
  /** 自定义空态文案 */
  emptyText?: string;
  /** 当前字段列表（绑定实例 + 字段对象） */
  fields: BoundField[];
  /** 所有可用字段列表 */
  availableFields?: ChartField[];
  /** 指标聚合方式映射（键为 bindingId） */
  aggregations?: Record<string, string>;
  /** 字段别名映射（键为 bindingId） */
  aliases?: Record<string, string>;
  /** 删除字段回调（按 bindingId） */
  onRemoveField?: (bindingId: string) => void;
  /** 聚合方式变更回调（按 bindingId） */
  onAggregationChange?: (bindingId: string, aggregation: string) => void;
  /** 打开设置回调 */
  onOpenSettings?: (bound: BoundField) => void;
  /** 添加字段回调 */
  onAddField?: (field: ChartField) => void;
  /** 重排序回调 */
  onReorderField?: (oldIndex: number, newIndex: number) => void;
}

const ROW_CONFIG: Record<DropZoneType, { label: string; color: string }> = {
  dimension: { label: '维度', color: '#1890ff' },
  metric: { label: '指标', color: '#52c41a' },
  filter: { label: '筛选', color: '#fa8c16' },
};

const QueryConfigRow: React.FC<QueryConfigRowProps> = ({
  rowType,
  groupIndex = 0,
  label,
  emptyText,
  fields,
  availableFields,
  aggregations,
  aliases,
  onRemoveField,
  onAggregationChange,
  onOpenSettings,
  onAddField,
  onReorderField,
}) => {
  const config = ROW_CONFIG[rowType];
  const displayLabel = label || config.label;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        marginBottom: '8px',
      }}
    >
      <div
        style={{
          width: '48px',
          minWidth: '48px',
          fontWeight: 500,
          color: config.color,
          paddingTop: '10px',
          fontSize: '13px',
        }}
      >
        {displayLabel}
      </div>
      <div style={{ flex: 1 }}>
        <FieldDropZone
          zoneType={rowType}
          label={displayLabel}
          groupIndex={groupIndex}
          fields={fields}
          availableFields={availableFields}
          aggregations={aggregations}
          aliases={aliases}
          onRemoveField={onRemoveField}
          onAggregationChange={onAggregationChange}
          onOpenSettings={onOpenSettings}
          onAddField={onAddField}
          onReorderField={onReorderField}
          emptyText={emptyText}
        />
      </div>
    </div>
  );
};

export default QueryConfigRow;
