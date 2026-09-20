import { Input, Modal, Space } from 'antd';
import React, { useEffect, useState } from 'react';
import type { ChartField } from '@/store';

export interface FieldSettingsValue {
  /** 显示名称：维度即显示名，指标即别名（影响请求聚合别名与列头）。 */
  alias: string;
  /** 单位（仅指标）：如 元 / %，展示为「列名 (单位)」。 */
  unit: string;
  /** 格式（仅指标）：如 0,0.00 → 千分位 + 两位小数。 */
  format: string;
}

export interface FieldSettingsModalProps {
  open: boolean;
  /** 维度只有显示名称；指标额外有单位与格式。 */
  kind: 'dimension' | 'metric';
  field: ChartField | null;
  initial: FieldSettingsValue;
  onOk: (value: FieldSettingsValue) => void;
  onCancel: () => void;
}

/**
 * 字段属性弹窗：查询配置芯片上的扳手按钮点击弹出，
 * 收拢原右下角「维度属性 / 指标属性」面板的显示名、单位、格式三项配置。
 * 修改实时生效（确定即写回 store 并触发自动查询），留空 = 清除该项配置。
 */
const FieldSettingsModal: React.FC<FieldSettingsModalProps> = ({
  open,
  kind,
  field,
  initial,
  onOk,
  onCancel,
}) => {
  const [alias, setAlias] = useState('');
  const [unit, setUnit] = useState('');
  const [format, setFormat] = useState('');

  // 依赖必须拆成原始值：调用方每次都传内联对象字面量，直接依赖 initial
  // 会让弹窗打开期间的任何父级重渲染（例如查询结果回流）把用户正在输入的
  // 内容重置回 store 里的旧值。
  const { alias: initialAlias, unit: initialUnit, format: initialFormat } = initial;
  useEffect(() => {
    if (!open) return;
    setAlias(initialAlias ?? '');
    setUnit(initialUnit ?? '');
    setFormat(initialFormat ?? '');
  }, [open, initialAlias, initialUnit, initialFormat]);

  const isMetric = kind === 'metric';

  return (
    <Modal
      open={open}
      title={field ? `字段属性 · ${field.name}` : '字段属性'}
      okText="确定"
      cancelText="取消"
      onOk={() => onOk({ alias: alias.trim(), unit: unit.trim(), format: format.trim() })}
      onCancel={onCancel}
      width={420}
      destroyOnHidden
      // 配置弹窗追求"即点即开"：关闭默认的 zoom+fade 过渡（约 300ms）。
      transitionName=""
      maskTransitionName=""
    >
      <Space direction="vertical" style={{ width: '100%' }} size={12}>
        {field?.comment && (
          <div style={{ fontSize: 12, color: 'var(--dr-text-2)' }}>{field.comment}</div>
        )}
        <div>
          <div style={{ fontSize: 12, color: 'var(--dr-text-2)', marginBottom: 4 }}>显示名称</div>
          <Input
            placeholder={field?.name}
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            data-testid="field-settings-alias"
          />
        </div>
        {isMetric && (
          <>
            <div>
              <div style={{ fontSize: 12, color: 'var(--dr-text-2)', marginBottom: 4 }}>单位</div>
              <Input
                placeholder="例如 元 / %"
                value={unit}
                onChange={(e) => setUnit(e.target.value)}
                data-testid="field-settings-unit"
              />
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--dr-text-2)', marginBottom: 4 }}>格式</div>
              <Input
                placeholder="例如 0,0.00"
                value={format}
                onChange={(e) => setFormat(e.target.value)}
                data-testid="field-settings-format"
              />
            </div>
          </>
        )}
      </Space>
    </Modal>
  );
};

export default FieldSettingsModal;
