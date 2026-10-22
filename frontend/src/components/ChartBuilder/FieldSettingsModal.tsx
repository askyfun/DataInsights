import { Input, Modal, Select, Space, Switch } from 'antd';
import React, { useEffect, useState } from 'react';
import { applyPercentFormat, splitPercentFormat } from '@/lib/format';
import type { ChartField } from '@/store';

export interface FieldSettingsValue {
  /** 显示名称：维度即显示名，指标即别名（影响请求聚合别名与列头）。 */
  alias: string;
  /** 单位（仅指标）：如 元 / %，展示为「列名 (单位)」。 */
  unit: string;
  /** 格式（仅指标）：如 0,0.00 → 千分位 + 两位小数；以 `%` 结尾表示按占比显示。 */
  format: string;
  /** 动态维度/指标的候选列（issue #134，不含当前列）；缺省/空数组 = 取消动态槽位。 */
  candidates?: string[];
}

export interface FieldSettingsModalProps {
  open: boolean;
  /** 维度只有显示名称；指标额外有单位与格式。 */
  kind: 'dimension' | 'metric';
  field: ChartField | null;
  initial: FieldSettingsValue;
  /** 可选的候选列（与当前字段同 kind，已排除当前列）。 */
  candidateOptions?: Array<{ id: string; name: string }>;
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
  candidateOptions,
  onOk,
  onCancel,
}) => {
  const [alias, setAlias] = useState('');
  const [unit, setUnit] = useState('');
  const [percentOfTotal, setPercentOfTotal] = useState(false);
  // 「格式」输入框承载不含 `%` 的部分；占比开关只负责最后补/剥那个后缀。
  const [format, setFormat] = useState('');
  /** 动态维度/指标候选（issue #134）：不含当前列的附加可选列。 */
  const [candidates, setCandidates] = useState<string[]>([]);

  // 依赖必须拆成原始值：调用方每次都传内联对象字面量，直接依赖 initial
  // 会让弹窗打开期间的任何父级重渲染（例如查询结果回流）把用户正在输入的
  // 内容重置回 store 里的旧值。
  const { alias: initialAlias, unit: initialUnit, format: initialFormat } = initial;
  const initialCandidatesKey = (initial.candidates ?? []).join('\u0000');
  useEffect(() => {
    if (!open) return;
    const parsed = splitPercentFormat(initialFormat ?? '');
    setAlias(initialAlias ?? '');
    setUnit(initialUnit ?? '');
    setFormat(parsed.base);
    setPercentOfTotal(parsed.percent);
    setCandidates(initialCandidatesKey === '' ? [] : initialCandidatesKey.split('\u0000'));
  }, [open, initialAlias, initialUnit, initialFormat, initialCandidatesKey]);

  const isMetric = kind === 'metric';

  return (
    <Modal
      open={open}
      title={field ? `字段属性 · ${field.name}` : '字段属性'}
      okText="确定"
      cancelText="取消"
      onOk={() =>
        onOk({
          alias: alias.trim(),
          unit: unit.trim(),
          format: applyPercentFormat(format, percentOfTotal),
          candidates: candidates.filter((id) => id !== field?.id),
        })
      }
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
            <div>
              <div style={{ fontSize: 12, color: 'var(--dr-text-2)', marginBottom: 4 }}>
                显示占比
              </div>
              <Switch
                checked={percentOfTotal}
                onChange={(checked) => setPercentOfTotal(checked)}
                data-testid="field-settings-percent"
              />
              {/* 占比不是格式串能自解释的东西：分母来自后端的完整数据集合计，
                  拿不到分母（图型不支持、或该表无维度）时这一列会留空。 */}
              <div style={{ fontSize: 12, color: 'var(--dr-text-3)', marginTop: 4 }}>
                按占全集比例显示（分母 = 过滤后全部数据的合计，不是当前页相加）；目前仅表格支持。
              </div>
            </div>
          </>
        )}
        {(candidateOptions?.length ?? 0) > 0 && (
          <div>
            <div style={{ fontSize: 12, color: 'var(--dr-text-2)', marginBottom: 4 }}>
              候选字段（动态{isMetric ? '指标' : '维度'}）
            </div>
            <Select
              mode="multiple"
              allowClear
              value={candidates}
              onChange={(values: string[]) => setCandidates(values)}
              placeholder="选择可在图表上切换的其他列"
              options={(candidateOptions ?? []).map((option) => ({
                value: option.id,
                label: option.name,
              }))}
              style={{ width: '100%' }}
              data-testid="field-settings-candidates"
            />
            <div style={{ fontSize: 12, color: 'var(--dr-text-3)', marginTop: 4 }}>
              观看者在图表上方切换候选列即可换视角；切换不改变当前列（默认选中项）。
            </div>
          </div>
        )}
      </Space>
    </Modal>
  );
};

export default FieldSettingsModal;
