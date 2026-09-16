import { CloseOutlined, HolderOutlined, SettingOutlined } from '@ant-design/icons';
import { Dropdown, Menu, Tag } from 'antd';
import React from 'react';
import type { ChartField } from '@/store';

export interface FieldPillProps {
  field: ChartField;
  /** 字段类型：dimension 或 metric */
  fieldType: 'dimension' | 'metric';
  /** 聚合方式（仅指标有效） */
  aggregation?: 'sum' | 'avg' | 'count' | 'max' | 'min' | 'none';
  /** 自定义别名 */
  alias?: string;
  /** 点击设置回调 */
  onSettings?: () => void;
  /** 删除回调 */
  onRemove?: () => void;
  /** 聚合方式变更回调 */
  onAggregationChange?: (aggregation: string) => void;
  /** 可排序模式：传入 sortable 返回的 props */
  sortable?: {
    isDragging?: boolean;
    attributes?: React.HTMLAttributes<HTMLElement>;
    listeners?: { [key: string]: unknown };
    setNodeRef?: (node: HTMLElement | null) => void;
    setActivatorNodeRef?: (node: HTMLElement | null) => void;
    style?: React.CSSProperties;
  };
}

const AGGREGATION_OPTIONS = [
  { label: '求和', value: 'sum' },
  { label: '平均', value: 'avg' },
  { label: '计数', value: 'count' },
  { label: '去重计数', value: 'count_distinct' },
  { label: '最大值', value: 'max' },
  { label: '最小值', value: 'min' },
  { label: '无聚合', value: 'none' },
];

const FieldPill: React.FC<FieldPillProps> = ({
  field,
  fieldType,
  aggregation = 'sum',
  alias,
  onSettings,
  onRemove,
  onAggregationChange,
  sortable,
}) => {
  const getColorByType = () => {
    if (fieldType === 'dimension') {
      if (field.dataType === 'date' || field.dataType === 'timestamp') {
        return 'purple';
      }
      return 'blue';
    }
    return 'green';
  };

  const getDisplayText = () => {
    if (alias) return alias;
    if (fieldType === 'metric' && aggregation !== 'none') {
      const aggLabel =
        AGGREGATION_OPTIONS.find((opt) => opt.value === aggregation)?.label || aggregation;
      return `${aggLabel}(${field.name})`;
    }
    return field.name;
  };

  const handleAggregationMenuClick = (value: string) => {
    onAggregationChange?.(value);
  };

  const aggregationMenu = (
    <Menu
      items={AGGREGATION_OPTIONS.map((opt) => ({
        key: opt.value,
        label: opt.label,
        onClick: () => handleAggregationMenuClick(opt.value),
      }))}
    />
  );

  const content = (
    <Tag
      color={getColorByType()}
      closable={false}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        padding: '4px 8px',
        margin: '2px',
        borderRadius: '12px',
        cursor: 'pointer',
        opacity: sortable?.isDragging ? 0.4 : 1,
        ...sortable?.style,
      }}
      ref={sortable?.setNodeRef}
      {...(sortable?.attributes || {})}
    >
      {sortable && (
        <span ref={sortable.setActivatorNodeRef} {...(sortable.listeners || {})}>
          <HolderOutlined style={{ fontSize: '12px', opacity: 0.5, cursor: 'grab' }} />
        </span>
      )}

      {fieldType === 'metric' ? (
        <Dropdown overlay={aggregationMenu} trigger={['click']}>
          <span style={{ fontWeight: 500 }}>{getDisplayText()}</span>
        </Dropdown>
      ) : (
        <span style={{ fontWeight: 500 }}>{getDisplayText()}</span>
      )}

      {onSettings && (
        <SettingOutlined
          style={{ fontSize: '12px', opacity: 0.6 }}
          onClick={(e) => {
            e.stopPropagation();
            onSettings();
          }}
        />
      )}

      {onRemove && (
        <CloseOutlined
          style={{ fontSize: '12px', opacity: 0.6 }}
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
        />
      )}
    </Tag>
  );

  return content;
};

export default FieldPill;
