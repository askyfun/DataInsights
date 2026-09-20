import { CloseOutlined, PlusOutlined } from '@ant-design/icons';
import { useDroppable } from '@dnd-kit/core';
import { Button, Dropdown, Tag } from 'antd';
import React from 'react';
import type { ChartField, FilterCondition, FilterOperator } from '@/store';
import { fieldTagColor } from './DraggableField';
import { dropZoneId, dropZoneSurfaceStyle } from './dropZoneStyles';

export interface FilterDropZoneProps {
  /** 当前过滤条件；数组顺序即条件在 SQL 中的连接顺序。 */
  filters: FilterCondition[];
  /** 可拖入的字段全集（维度与指标均可参与过滤）。 */
  availableFields: ChartField[];
  /** 拖入 / 下拉选择字段后回调：ChartBuilder 会弹窗收集过滤条件（期间不自动查询）。 */
  onAdd: (field: ChartField) => void;
  /** 点击已存在的过滤条件芯片 → 弹窗编辑。 */
  onEdit: (filter: FilterCondition, field: ChartField | undefined) => void;
  onRemove: (id: string) => void;
  emptyText?: string;
}

/** 操作符中文标签（与过滤配置弹窗共用口径）。 */
export const OPERATOR_LABELS: Partial<Record<FilterOperator, string>> = {
  eq: '等于',
  neq: '不等于',
  gt: '大于',
  gte: '大于等于',
  lt: '小于',
  lte: '小于等于',
  like: '包含',
  in: '属于',
  notIn: '不属于',
  between: '区间',
  isNull: '为空',
  isNotNull: '不为空',
};

/** 条件的单行摘要：芯片右侧的只读说明，点击整行进弹窗改。 */
export function describeFilter(filter: FilterCondition): string {
  const { operator } = filter;
  const label = OPERATOR_LABELS[operator] ?? operator;
  if (operator === 'isNull' || operator === 'isNotNull') {
    return label;
  }
  if (operator === 'in' || operator === 'notIn') {
    const values = Array.isArray(filter.value) ? (filter.value as unknown[]).map(String) : [];
    return `${OPERATOR_LABELS[operator]} ${values.length} 个值`;
  }
  if (operator === 'between') {
    return `区间 ${String(filter.value ?? '')} ~ ${String(filter.valueEnd ?? '')}`;
  }
  return `${label} ${String(filter.value ?? '')}`;
}

/**
 * 过滤字段组：与维度组、指标组同构的可拖入区域。
 * 调用场景：查询配置区第三行。
 * 主要逻辑：
 *   - 从左侧字段列表拖入任意字段（维度或指标）→ ChartBuilder 弹出过滤配置弹窗；
 *   - 已有条件渲染为「字段芯片 + 摘要」，点击进弹窗编辑，行尾 × 删除；
 *   - 多个条件之间恒为「且」：下发请求时 logic 恒为 and
 *     （后端 bun_builder 对非 OR 的 logic 一律按 AND 处理）。
 * 同一字段可重复加入：>= 与 <= 各拖一次即构成区间，不做去重。
 */
const FilterDropZone: React.FC<FilterDropZoneProps> = ({
  filters,
  availableFields,
  onAdd,
  onEdit,
  onRemove,
  emptyText = '拖拽字段到此添加筛选，多个条件为「且」关系',
}) => {
  const { setNodeRef, isOver } = useDroppable({
    id: dropZoneId('filter', 0),
    data: { type: 'filter', groupIndex: 0 },
  });

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
            backgroundColor: field.type === 'dimension' ? 'var(--dr-dim)' : 'var(--dr-metric)',
          }}
        />
        {field.name}
      </span>
    ),
    onClick: () => {
      onAdd(field);
    },
  }));

  return (
    <div
      ref={setNodeRef}
      data-testid="filter-drop-zone"
      style={dropZoneSurfaceStyle('filter', isOver)}
    >
      {/* + 固定在最左（与维度/指标行的行首对齐），提示文案与条件芯片依次排在其右 */}
      {availableFields.length > 0 && (
        <Dropdown menu={{ items: dropdownItems }} trigger={['click']} placement="bottomLeft">
          <Button
            type="dashed"
            size="small"
            icon={<PlusOutlined />}
            onClick={(e) => e.preventDefault()}
          />
        </Dropdown>
      )}

      {filters.length === 0 && (
        <span style={{ color: 'var(--dr-text-3)', fontSize: 13 }}>{emptyText}</span>
      )}

      {filters.map((filter) => {
        const field = fieldByName.get(filter.field);
        return (
          <div
            key={filter.id}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 4,
              flex: '0 1 auto',
              minWidth: 0,
              maxWidth: '100%',
            }}
          >
            {/* 行本体用原生 button：行内嵌了删除按钮，继续用 role="button" 的 div
                会构成交互元素嵌套，键盘语义也不如原生按钮完整。 */}
            <button
              type="button"
              data-testid={`filter-row-${filter.id}`}
              onClick={() => onEdit(filter, field)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 4,
                flex: '0 1 auto',
                cursor: 'pointer',
                minWidth: 0,
                maxWidth: '100%',
                margin: 0,
                padding: 0,
                border: 0,
                background: 'none',
                font: 'inherit',
                color: 'inherit',
                textAlign: 'left',
              }}
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
              <span
                data-testid={`filter-summary-${filter.id}`}
                style={{
                  flex: '0 1 auto',
                  minWidth: 0,
                  maxWidth: 220,
                  overflow: 'hidden',
                  whiteSpace: 'nowrap',
                  textOverflow: 'ellipsis',
                  fontSize: 12,
                  color: 'var(--dr-text-2)',
                }}
              >
                {describeFilter(filter)}
              </span>
            </button>
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
    </div>
  );
};

export default FilterDropZone;
