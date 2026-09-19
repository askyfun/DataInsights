import { useDraggable } from '@dnd-kit/core';
import { Tag, Tooltip } from 'antd';
import React from 'react';
import type { ChartField } from '@/store';

export interface DraggableFieldProps {
  field: ChartField;
}

export interface FieldDragPreviewProps {
  /** 预览标签文本：侧边栏字段直接用字段名，配置区字段标签用带聚合/别名的显示文本。 */
  label: string;
  /** 预览标签颜色（维度蓝 / 日期紫 / 指标绿），由调用方按字段类型判定。 */
  color: string;
}

/**
 * 根据字段角色和数据类型返回统一标签颜色（antd 预设色名）。
 * 调用场景：侧边栏字段标签、配置区字段标签与拖拽 overlay 预览需要保持一致的视觉语义。
 * 主要逻辑：日期维度使用紫色，普通维度使用蓝色，指标使用绿色。
 */
export const fieldTagColor = (field: ChartField): 'blue' | 'purple' | 'green' => {
  if (field.type === 'dimension') {
    if (field.dataType === 'date' || field.dataType === 'timestamp') {
      return 'purple';
    }
    return 'blue';
  }
  return 'green';
};

/**
 * 语义色的变量形态。antd 的 Tag 只吃预设色名，而圆点 / 色条需要具体色值；
 * 两者同源登记在这里，避免同一个"维度=蓝"的规则散成两份色谱。
 * 变量定义在 styles/index.css 的 :root 上，ChartBuilder 与 ShareView 都取得到。
 */
const FIELD_ACCENT: Record<'blue' | 'purple' | 'green', string> = {
  blue: 'var(--dr-dim)',
  purple: 'var(--dr-date)',
  green: 'var(--dr-metric)',
};

/**
 * 渲染拖拽中的标签预览（侧边栏字段与配置区字段标签共用）。
 * 调用场景：ChartBuilder 的 DragOverlay 需要一个跟随鼠标移动的轻量视觉副本。
 * 主要逻辑：按下发的颜色渲染标签，并关闭指针事件以免遮挡 drop zone 命中。
 */
export const FieldDragPreview: React.FC<FieldDragPreviewProps> = ({ label, color }) => {
  return (
    <Tag
      data-testid="drag-overlay-field"
      color={color}
      style={{
        cursor: 'grabbing',
        marginBottom: '4px',
        padding: '0 5px',
        pointerEvents: 'none',
        boxShadow: '0 8px 24px rgba(0, 0, 0, 0.18)',
        borderRadius: '5px',
      }}
    >
      {label}
    </Tag>
  );
};

/**
 * 渲染侧边栏可拖拽字段行。
 * 调用场景：图表查询界面的字段列表。
 * 主要逻辑：通过 dnd-kit 注册 draggable 节点；每行左侧为字段类型圆点、中间为字段名，
 * 整行作为 Tooltip 触发区，悬停时展示字段注释（无注释则不弹出）；拖拽时降低整行透明度。
 * 视觉约定：圆点色 = 字段类型语义色（维度蓝 / 日期紫 / 指标绿），与字段标签同源；
 * 悬停用品牌蓝浅底而非黑色蒙层——黑色蒙层在灰画布上会发浑，也读不出"可拖"。
 */
const DraggableField: React.FC<DraggableFieldProps> = ({ field }) => {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `field-${field.id}`,
    data: {
      type: 'field',
      field,
      fieldType: field.type,
    },
  });
  const [hovered, setHovered] = React.useState(false);

  return (
    <Tooltip title={field.comment} placement="right" mouseEnterDelay={0} mouseLeaveDelay={0}>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: 纯 hover 展示 Tooltip 的字段行，无键盘交互语义 */}
      <div
        ref={setNodeRef}
        data-testid={`field-row-${field.name}`}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '3px 6px',
          marginBottom: '1px',
          borderRadius: '6px',
          cursor: 'grab',
          opacity: isDragging ? 0.5 : 1,
          backgroundColor: hovered ? 'var(--dr-accent-soft)' : 'transparent',
          transition: 'background-color 0.2s ease',
        }}
        {...listeners}
        {...attributes}
      >
        <span
          aria-hidden
          style={{
            width: 5,
            height: 5,
            borderRadius: '50%',
            flexShrink: 0,
            backgroundColor: FIELD_ACCENT[fieldTagColor(field)],
          }}
        />
        <span
          style={{
            flex: 1,
            minWidth: 0,
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
            fontSize: '13px',
            color: 'var(--dr-text-2)',
          }}
        >
          {field.name}
        </span>
      </div>
    </Tooltip>
  );
};

export default DraggableField;
