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
  /** 当前字段列表（绑定实例 + 字段对象）；用 children 自定义内容时可省略 */
  fields?: BoundField[];
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
  /**
   * 自定义字段区内容。提供时替代默认的 FieldDropZone。
   * 调用场景：过滤字段组——条件不是「绑定」（每条自带操作符与值），无法复用 FieldDropZone，
   * 但标签列宽与配色必须与维度/指标组同源，故复用本行的外壳。
   */
  children?: React.ReactNode;
}

/**
 * 槽位语义色。走全站语义色变量（:root 的 --dr-dim / --dr-metric / --dr-filter），
 * 与 dropZoneStyles 的 ZONE_PALETTE 同源：色条、标签、落点高亮指的其实是同一个"维度"，
 * 色值分叉会让同一概念出现两种蓝。
 */
const ROW_CONFIG: Record<DropZoneType, { label: string; color: string }> = {
  dimension: { label: '维度', color: 'var(--dr-dim)' },
  metric: { label: '指标', color: 'var(--dr-metric)' },
  filter: { label: '过滤', color: 'var(--dr-filter)' },
};

const QueryConfigRow: React.FC<QueryConfigRowProps> = ({
  rowType,
  groupIndex = 0,
  label,
  emptyText,
  fields = [],
  availableFields,
  aggregations,
  aliases,
  onRemoveField,
  onAggregationChange,
  onOpenSettings,
  onAddField,
  onReorderField,
  children,
}) => {
  const config = ROW_CONFIG[rowType];
  const displayLabel = label || config.label;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        marginBottom: '4px',
      }}
    >
      {/* 槽位标签 = 3px 语义色条 + 深色文字。
          颜色从"文字色"改由色条承载：12~13px 的彩色文字对比度偏低（尤其橙 #fa8c16），
          而色条既能保住颜色出现的位置，又和左栏字段分组头使用同一套视觉语法
          （色条=分组 / 圆点=字段），读者不必学两遍。
          60px 是实测宽度：最长的「X 轴指标」在 13px 下约 51px，加色条与间隙 ≈59px，
          收紧到 48px 会让它折行、行高与拖放区错位。 */}
      <div
        style={{
          width: '60px',
          minWidth: '60px',
          height: 26,
          display: 'flex',
          alignItems: 'center',
          gap: 5,
          fontSize: '13px',
          fontWeight: 600,
        }}
      >
        <span
          aria-hidden
          style={{
            width: 3,
            height: 11,
            borderRadius: 2,
            flexShrink: 0,
            background: config.color,
          }}
        />
        <span style={{ color: 'var(--dr-text-2)' }}>{displayLabel}</span>
      </div>
      <div style={{ flex: 1 }}>
        {children ?? (
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
        )}
      </div>
    </div>
  );
};

export default QueryConfigRow;
