import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  CloseOutlined,
  PlusOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { useDraggable, useDroppable } from '@dnd-kit/core';
import { Button, Dropdown, Tag } from 'antd';
import React, { useState } from 'react';
import type { BoundField, ChartField } from '@/store';
import { type DropZoneType, dropZoneId, dropZoneSurfaceStyle } from './dropZoneStyles';

export type { DropZoneType };

/**
 * 拖动查询配置区里的字段标签时挂在 active.data.current 上的载荷。
 * 调用场景：ChartBuilder 的 onDragStart/onDragEnd 据此识别"配置区内部搬字段"。
 */
export interface BindingDragData {
  type: 'binding-source';
  bindingId: string;
  kind: 'dimension' | 'metric';
  groupIndex: number;
  /** 拖拽预览文本（已含聚合/别名），避免预览侧再回查 store。 */
  label: string;
  /** 预览标签颜色（维度蓝 / 日期紫 / 指标绿）。 */
  color: string;
}

/**
 * 字段标签自身作为落点（插入到该标签之前）时挂在 over.data.current 上的载荷。
 * 调用场景：组内换序 / 跨组插到指定位置——只有拿到 index 才能精确落位。
 */
export interface BindingSlotDropData {
  type: 'binding-slot';
  kind: 'dimension' | 'metric';
  groupIndex: number;
  index: number;
  bindingId: string;
}

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
  groupIndex: number;
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

/**
 * 合并 dnd-kit 的 draggable / droppable 两个 ref 到同一个 DOM 节点。
 * 调用场景：字段标签既要能拖起（拖到别的组），又要能作为落点（插到它前面）。
 * 主要逻辑：回调 ref 逐个透传节点；卸载时两个 ref 都收到 null。
 */
const mergeRefs =
  (
    first: (node: HTMLElement | null) => void,
    second: (node: HTMLElement | null) => void
  ): ((node: HTMLElement | null) => void) =>
  (node) => {
    first(node);
    second(node);
  };

const FieldPillInline: React.FC<FieldPillInlineProps> = ({
  bound,
  zoneType,
  groupIndex,
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

  const displayText = getDisplayText();

  // 拖起：整个标签都是拖拽把手。PointerSensor 有 5px 位移阈值，因此标签上的
  // 设置/关闭按钮仍可正常点击，不会误触发拖拽。
  const {
    attributes,
    listeners,
    setNodeRef: setDragNodeRef,
    isDragging,
  } = useDraggable({
    id: `binding-${bindingId}`,
    data: {
      type: 'binding-source',
      bindingId,
      kind: fieldType,
      groupIndex,
      label: displayText,
      color: getColor(),
    } satisfies BindingDragData,
  });

  // 落点：插到本标签之前。与所在组的 drop zone（追加到末尾）同层，依靠 dnd-kit
  // 默认的矩形交集评分（IoU 越大越优先）让内层标签压过外层区域。
  const { setNodeRef: setSlotNodeRef, isOver } = useDroppable({
    id: `binding-slot-${bindingId}`,
    data: {
      type: 'binding-slot',
      kind: fieldType,
      groupIndex,
      index,
      bindingId,
    } satisfies BindingSlotDropData,
  });

  return (
    <span
      ref={mergeRefs(setDragNodeRef, setSlotNodeRef)}
      data-testid={`binding-pill-${bindingId}`}
      {...listeners}
      {...attributes}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 2,
        margin: 0,
        borderRadius: '5px',
        cursor: 'grab',
        opacity: isDragging ? 0.4 : 1,
        outline: isOver ? '2px solid var(--dr-accent)' : 'none',
        outlineOffset: '1px',
      }}
    >
      <Tag
        color={getColor()}
        closable={false}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '4px',
          padding: '0 5px',
          margin: 0,
          borderRadius: '5px',
        }}
      >
        <span style={{ fontWeight: 500 }}>{displayText}</span>
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
    id: dropZoneId(zoneType, groupIndex),
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
            backgroundColor: field.type === 'dimension' ? 'var(--dr-dim)' : 'var(--dr-metric)',
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

  return (
    <div ref={setNodeRef} style={dropZoneSurfaceStyle(zoneType, isOver)}>
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
          <span style={{ color: 'var(--dr-text-3)', fontSize: '13px' }}>
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
              groupIndex={groupIndex}
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
