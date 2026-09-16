import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  CloseOutlined,
  HolderOutlined,
  PlusOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { useDroppable } from '@dnd-kit/core';
import { Button, Dropdown, Tag } from 'antd';
import React, { useState } from 'react';
import type { BoundField, ChartField } from '@/store';

export type DropZoneType = 'dimension' | 'metric' | 'filter';

export interface FieldDropZoneProps {
  zoneType: DropZoneType;
  label: string;
  groupIndex?: number;
  fields: BoundField[];
  availableFields?: ChartField[];
  aggregations?: Record<string, string>;
  aliases?: Record<string, string>;
  onRemoveField?: (bindingId: string) => void;
  onAggregationChange?: (bindingId: string, aggregation: string) => void;
  onOpenSettings?: (bound: BoundField) => void;
  onAddField?: (field: ChartField) => void;
  onReorderField?: (oldIndex: number, newIndex: number) => void;
  emptyText?: string;
}

const AGGREGATION_OPTIONS = [
  { label: '求和', value: 'sum' },
  { label: '平均', value: 'avg' },
  { label: '计数', value: 'count' },
  { label: '最大值', value: 'max' },
  { label: '最小值', value: 'min' },
  { label: '无聚合', value: 'none' },
];

interface FieldPillInlineProps {
  bound: BoundField;
  zoneType: DropZoneType;
  aggregations: Record<string, string>;
  aliases: Record<string, string>;
  index: number;
  total: number;
  onRemoveField?: (bindingId: string) => void;
  onAggregationChange?: (bindingId: string, aggregation: string) => void;
  onOpenSettings?: (bound: BoundField) => void;
  onMoveLeft?: () => void;
  onMoveRight?: () => void;
}

const FieldPillInline: React.FC<FieldPillInlineProps> = ({
  bound,
  zoneType,
  aggregations,
  aliases,
  index,
  total,
  onRemoveField,
  onOpenSettings,
  onMoveLeft,
  onMoveRight,
}) => {
  const { field, binding } = bound;
  const bindingId = binding.bindingId;
  const fieldType = zoneType === 'filter' ? 'dimension' : zoneType;
  const agg = (aggregations[bindingId] || 'sum') as string;
  const alias = aliases[bindingId];

  const getColor = () => {
    if (fieldType === 'dimension') {
      return field.dataType === 'date' || field.dataType === 'timestamp' ? 'purple' : 'blue';
    }
    return 'green';
  };

  const getDisplayText = () => {
    if (alias) return alias;
    if (fieldType === 'metric' && agg !== 'none') {
      const label = AGGREGATION_OPTIONS.find((o) => o.value === agg)?.label || agg;
      return `${label}(${field.name})`;
    }
    return field.name;
  };

  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 2, margin: '2px' }}>
      <Tag
        color={getColor()}
        closable={false}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '4px',
          padding: '4px 8px',
          margin: 0,
          borderRadius: '12px',
        }}
      >
        <HolderOutlined style={{ fontSize: '12px', opacity: 0.4 }} />
        <span style={{ fontWeight: 500 }}>{getDisplayText()}</span>
        {onOpenSettings && (
          <SettingOutlined
            style={{ fontSize: '12px', opacity: 0.6, cursor: 'pointer' }}
            onClick={(e) => {
              e.stopPropagation();
              onOpenSettings(bound);
            }}
          />
        )}
        <CloseOutlined
          style={{ fontSize: '12px', opacity: 0.6, cursor: 'pointer' }}
          onClick={(e) => {
            e.stopPropagation();
            onRemoveField?.(bindingId);
          }}
        />
      </Tag>
      {onMoveLeft && (
        <ArrowLeftOutlined
          style={{
            fontSize: '10px',
            opacity: index > 0 ? 0.6 : 0.2,
            cursor: index > 0 ? 'pointer' : 'default',
          }}
          onClick={() => index > 0 && onMoveLeft()}
        />
      )}
      {onMoveRight && (
        <ArrowRightOutlined
          style={{
            fontSize: '10px',
            opacity: index < total - 1 ? 0.6 : 0.2,
            cursor: index < total - 1 ? 'pointer' : 'default',
          }}
          onClick={() => index < total - 1 && onMoveRight()}
        />
      )}
    </span>
  );
};

const FieldDropZone: React.FC<FieldDropZoneProps> = ({
  zoneType,
  groupIndex = 0,
  fields,
  availableFields = [],
  aggregations = {},
  aliases = {},
  onRemoveField,
  onAggregationChange: _onAggregationChange,
  onOpenSettings,
  onAddField,
  onReorderField,
  emptyText,
}) => {
  const { setNodeRef, isOver } = useDroppable({
    id: `dropzone-${zoneType}-${groupIndex}`,
    data: { type: zoneType, groupIndex },
  });

  const [dropdownOpen, setDropdownOpen] = useState(false);

  const defaultEmptyText = {
    dimension: '拖拽维度字段到此，或点击+添加',
    metric: '拖拽指标字段到此，或点击+添加',
    filter: '拖拽字段添加筛选',
  };

  const filteredFields = availableFields
    .filter((f) => {
      if (zoneType === 'dimension') return f.type === 'dimension';
      if (zoneType === 'metric') return f.type === 'metric';
      return true;
    })
    .filter((f) => !fields.some((added) => added.field.id === f.id));

  const dropdownItems = filteredFields.map((field) => ({
    key: field.id,
    label: (
      <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: field.type === 'dimension' ? '#1890ff' : '#52c41a',
          }}
        />
        {field.name}
      </span>
    ),
    onClick: () => {
      onAddField?.(field);
      setDropdownOpen(false);
    },
  }));

  const getZoneColor = () => {
    switch (zoneType) {
      case 'dimension':
        return isOver ? '#e6f4ff' : '#fafafa';
      case 'metric':
        return isOver ? '#f6ffed' : '#fafafa';
      case 'filter':
        return isOver ? '#fff7e6' : '#fafafa';
      default:
        return '#fafafa';
    }
  };

  const getBorderColor = () => {
    if (!isOver) return '#d9d9d9';
    switch (zoneType) {
      case 'dimension':
        return '#1890ff';
      case 'metric':
        return '#52c41a';
      case 'filter':
        return '#fa8c16';
      default:
        return '#d9d9d9';
    }
  };

  return (
    <div
      ref={setNodeRef}
      style={{
        minHeight: '40px',
        padding: '6px 10px',
        backgroundColor: getZoneColor(),
        border: `1px dashed ${getBorderColor()}`,
        borderRadius: '6px',
        transition: 'all 0.2s ease',
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        flexWrap: 'wrap',
      }}
    >
      {fields.length === 0 ? (
        filteredFields.length > 0 ? (
          <Dropdown
            menu={{ items: dropdownItems }}
            trigger={['click']}
            open={dropdownOpen}
            onOpenChange={setDropdownOpen}
            placement="bottomLeft"
          >
            <Button
              type="dashed"
              size="small"
              icon={<PlusOutlined />}
              onClick={(e) => e.preventDefault()}
              style={{ border: 'none', padding: '4px 8px', height: 'auto' }}
            />
          </Dropdown>
        ) : (
          <span style={{ color: '#999', fontSize: '13px' }}>
            {emptyText || defaultEmptyText[zoneType]}
          </span>
        )
      ) : (
        <>
          {fields.map((bound, index) => (
            <FieldPillInline
              key={bound.binding.bindingId}
              bound={bound}
              zoneType={zoneType}
              aggregations={aggregations}
              aliases={aliases}
              index={index}
              total={fields.length}
              onRemoveField={onRemoveField}
              onAggregationChange={_onAggregationChange}
              onOpenSettings={onOpenSettings}
              onMoveLeft={onReorderField ? () => onReorderField(index, index - 1) : undefined}
              onMoveRight={onReorderField ? () => onReorderField(index, index + 1) : undefined}
            />
          ))}
          {filteredFields.length > 0 && (
            <Dropdown menu={{ items: dropdownItems }} trigger={['click']} placement="bottomLeft">
              <Button
                type="dashed"
                size="small"
                icon={<PlusOutlined />}
                onClick={(e) => e.preventDefault()}
                style={{ border: 'none', padding: '4px 8px', height: 'auto', minWidth: 24 }}
              />
            </Dropdown>
          )}
        </>
      )}
    </div>
  );
};

export default FieldDropZone;
