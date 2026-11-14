import { Select } from 'antd';
import React from 'react';
import type { DynamicFieldSelections, DynamicSlot } from '../../lib/dynamicFieldSlots';

export interface DynamicSlotSwitcherProps {
  /** 当前图型的动态槽位（候选数 ≥1；由 dynamicSlotsOf 投影）。空数组时不渲染。 */
  slots: readonly DynamicSlot[];
  /** 会话选择（bindingId → 列 id）；缺省项用槽位默认列。 */
  selections: DynamicFieldSelections;
  /** 切换回调：只更新会话态并重新取数，不写回图表配置。 */
  onChange: (bindingId: string, fieldId: string) => void;
  /** 测试/自动化定位前缀（可选）。 */
  testIdPrefix?: string;
}

/**
 * 动态维度/指标切换器（issue #134）：图表上方的行内控件组。
 *
 * 只负责展示与上报选择——「选择算不算数、要不要重查」由宿主（构建页 / 仪表盘）决定。
 * 值必须收敛在槽位 options 内：选择失效时 dynamicSlotsOf/pruneSelections 已把它回落默认列。
 */
const DynamicSlotSwitcher: React.FC<DynamicSlotSwitcherProps> = ({
  slots,
  selections,
  onChange,
  testIdPrefix = 'dynamic-slot',
}) => {
  if (slots.length === 0) {
    return null;
  }
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: 8,
        marginBottom: 8,
      }}
      data-testid={`${testIdPrefix}-switcher`}
    >
      {slots.map((slot) => (
        <div key={slot.bindingId} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <span style={{ fontSize: 12, color: 'var(--dr-text-2)' }}>{slot.label}</span>
          <Select
            size="small"
            style={{ minWidth: 120 }}
            value={selections[slot.bindingId] ?? slot.defaultFieldId}
            onChange={(fieldId: string) => onChange(slot.bindingId, fieldId)}
            options={slot.options.map((option) => ({ value: option.id, label: option.name }))}
            data-testid={`${testIdPrefix}-${slot.bindingId}`}
          />
        </div>
      ))}
    </div>
  );
};

export default DynamicSlotSwitcher;
