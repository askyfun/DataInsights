import { CloseOutlined, PlusOutlined } from '@ant-design/icons';
import { useDroppable } from '@dnd-kit/core';
import { Button, Dropdown, Input, Select, Tag } from 'antd';
import React, { useState } from 'react';
import type { ChartField, FilterCondition, FilterOperator } from '@/store';
import { fieldTagColor } from './DraggableField';
import { dropZoneId, dropZoneSurfaceStyle } from './dropZoneStyles';

export interface FilterDropZoneProps {
  /** 当前过滤条件；数组顺序即条件在 SQL 中的连接顺序。 */
  filters: FilterCondition[];
  /** 可拖入的字段全集（维度与指标均可参与过滤）。 */
  availableFields: ChartField[];
  onAdd: (field: ChartField) => void;
  onRemove: (id: string) => void;
  onUpdate: (id: string, patch: Partial<FilterCondition>) => void;
  emptyText?: string;
}

/**
 * 过滤操作符选项。
 * value 必须是后端 entity.Filter 认得的字面量：gte/lte/neq 不可写成 ge/le/ne，
 * 后端对未知操作符会静默退化。标签用 ASCII 比较符 + 中文动词，下拉宽度可控。
 */
const OPERATOR_OPTIONS: { value: FilterOperator; label: string }[] = [
  { value: 'eq', label: '=' },
  { value: 'neq', label: '!=' },
  { value: 'gt', label: '>' },
  { value: 'gte', label: '>=' },
  { value: 'lt', label: '<' },
  { value: 'lte', label: '<=' },
  { value: 'like', label: '包含' },
  { value: 'in', label: '属于' },
  { value: 'between', label: '区间' },
  { value: 'isNull', label: '为空' },
  { value: 'isNotNull', label: '不为空' },
];

const needsValue = (operator: FilterOperator): boolean =>
  operator !== 'isNull' && operator !== 'isNotNull';

const needsTwoValues = (operator: FilterOperator): boolean => operator === 'between';

const needsMultiValues = (operator: FilterOperator): boolean => operator === 'in';

/**
 * 过滤字段组：与维度组、指标组同构的可拖入区域。
 * 调用场景：查询配置区第三行，替代旧的手动「Add Filter」按钮式交互。
 * 主要逻辑：
 *   - 从左侧字段列表拖入任意字段（维度或指标）→ 落到本区即在 store 追加一条过滤条件；
 *   - 每条条件就地选择操作符并填值，删除用行尾 ×；
 *   - 多个条件之间恒为「且」：不再提供 AND/OR 切换，下发请求时 logic 恒为 and
 *     （后端 bun_builder 对非 OR 的 logic 一律按 AND 处理）。
 * 同一字段可重复加入：>= 与 <= 各拖一次即构成区间，不做去重。
 */
const FilterDropZone: React.FC<FilterDropZoneProps> = ({
  filters,
  availableFields,
  onAdd,
  onRemove,
  onUpdate,
  emptyText = '拖拽字段到此添加筛选，多个条件为「且」关系',
}) => {
  const { setNodeRef, isOver } = useDroppable({
    id: dropZoneId('filter', 0),
    data: { type: 'filter', groupIndex: 0 },
  });
  const [dropdownOpen, setDropdownOpen] = useState(false);

  // 过滤条件只存列名，回查字段对象是为了拿到类型/日期信息以决定标签颜色。
  const fieldByName = new Map(availableFields.map((field) => [field.name, field]));

  const dropdownItems = availableFields.map((field) => ({
    key: field.name,
    label: (
      <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: field.type === 'dimension' ? '#1677ff' : '#52c41a',
          }}
        />
        {field.name}
      </span>
    ),
    onClick: () => {
      onAdd(field);
      setDropdownOpen(false);
    },
  }));

  const renderValueInput = (filter: FilterCondition) => {
    if (!needsValue(filter.operator)) {
      return null;
    }

    if (needsTwoValues(filter.operator)) {
      return (
        <>
          <Input
            size="small"
            style={{ width: 76 }}
            placeholder="最小值"
            value={filter.value ?? ''}
            onChange={(e) => onUpdate(filter.id, { value: e.target.value })}
            data-testid={`filter-value-${filter.id}`}
          />
          <span style={{ color: 'var(--dr-text-3)' }}>-</span>
          <Input
            size="small"
            style={{ width: 76 }}
            placeholder="最大值"
            value={filter.valueEnd ?? ''}
            onChange={(e) => onUpdate(filter.id, { valueEnd: e.target.value })}
            data-testid={`filter-value-end-${filter.id}`}
          />
        </>
      );
    }

    return (
      <Input
        size="small"
        style={{ flex: 1, minWidth: 90 }}
        placeholder={needsMultiValues(filter.operator) ? '值1, 值2, 值3' : '值'}
        value={filter.value ?? ''}
        onChange={(e) => onUpdate(filter.id, { value: e.target.value })}
        data-testid={`filter-value-${filter.id}`}
      />
    );
  };

  return (
    <div
      ref={setNodeRef}
      data-testid="filter-drop-zone"
      style={dropZoneSurfaceStyle('filter', isOver)}
    >
      {filters.length === 0 && (
        <span style={{ color: 'var(--dr-text-3)', fontSize: 13 }}>{emptyText}</span>
      )}

      {filters.map((filter) => {
        const field = fieldByName.get(filter.field);
        return (
          <div
            key={filter.id}
            data-testid={`filter-row-${filter.id}`}
            style={{ display: 'flex', alignItems: 'center', gap: 4, flex: '1 1 100%' }}
          >
            <Tag
              color={field ? fieldTagColor(field) : 'default'}
              style={{
                margin: 0,
                padding: '0 5px',
                borderRadius: '5px',
                maxWidth: 160,
                overflow: 'hidden',
                whiteSpace: 'nowrap',
                textOverflow: 'ellipsis',
              }}
            >
              {filter.field || '未选择字段'}
            </Tag>
            <Select
              size="small"
              style={{ width: 84 }}
              value={filter.operator}
              onChange={(operator) =>
                onUpdate(filter.id, { operator, value: '', valueEnd: undefined })
              }
              options={OPERATOR_OPTIONS}
              data-testid={`filter-operator-${filter.id}`}
            />
            {renderValueInput(filter)}
            <Button
              type="text"
              size="small"
              danger
              icon={<CloseOutlined />}
              onClick={() => onRemove(filter.id)}
              data-testid={`filter-remove-${filter.id}`}
            />
          </div>
        );
      })}

      {availableFields.length > 0 && (
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
          />
        </Dropdown>
      )}
    </div>
  );
};

export default FilterDropZone;
