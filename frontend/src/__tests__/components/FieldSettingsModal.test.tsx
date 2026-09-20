import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import FieldSettingsModal from '../../components/ChartBuilder/FieldSettingsModal';
import type { ChartField } from '../../store';

/**
 * 字段属性弹窗：芯片扳手按钮打开，收拢原右下角维度/指标属性面板的配置。
 * 维度只有显示名称；指标额外有单位与格式；确定上抛 trim 后的值。
 */

const FIELD: ChartField = {
  id: 'retail_sales',
  name: 'retail_sales',
  type: 'metric',
  dataType: 'double',
};

const DIM_FIELD: ChartField = {
  id: 'brand_name',
  name: 'brand_name',
  type: 'dimension',
  dataType: 'character varying',
};

type Props = Parameters<typeof FieldSettingsModal>[0];

const renderModal = (props: Partial<Props> = {}) => {
  const onOk = vi.fn();
  const onCancel = vi.fn();
  render(
    <FieldSettingsModal
      open
      kind="metric"
      field={FIELD}
      initial={{ alias: '', unit: '', format: '' }}
      onOk={onOk}
      onCancel={onCancel}
      {...props}
    />
  );
  return { onOk, onCancel };
};

describe('FieldSettingsModal', () => {
  it('指标字段：显示名称/单位/格式三项，确定上抛 trim 后的值', () => {
    const onOk = vi.fn();
    renderModal({ onOk });

    fireEvent.change(screen.getByTestId('field-settings-alias'), {
      target: { value: '销售额 ' },
    });
    fireEvent.change(screen.getByTestId('field-settings-unit'), { target: { value: '元' } });
    fireEvent.change(screen.getByTestId('field-settings-format'), {
      target: { value: '0,0.00' },
    });
    fireEvent.click(screen.getByRole('button', { name: '确 定' }));

    expect(onOk).toHaveBeenCalledWith({ alias: '销售额', unit: '元', format: '0,0.00' });
  });

  it('维度字段：只有显示名称，单位/格式不渲染', () => {
    renderModal({ kind: 'dimension', field: DIM_FIELD });

    expect(screen.getByTestId('field-settings-alias')).toBeInTheDocument();
    expect(screen.queryByTestId('field-settings-unit')).not.toBeInTheDocument();
    expect(screen.queryByTestId('field-settings-format')).not.toBeInTheDocument();
  });

  it('留空 = 清除：确定上抛空字符串', () => {
    const onOk = vi.fn();
    renderModal({ onOk, initial: { alias: '销售额', unit: '元', format: '0,0.00' } });

    fireEvent.click(screen.getByRole('button', { name: '确 定' }));
    expect(onOk).toHaveBeenCalledWith({ alias: '销售额', unit: '元', format: '0,0.00' });
  });

  it('取消上抛 onCancel', () => {
    const { onCancel } = renderModal();
    fireEvent.click(screen.getByRole('button', { name: '取 消' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('父级重渲染（initial 每次都是新对象字面量）不重置正在输入的内容', () => {
    const onOk = vi.fn();
    const onCancel = vi.fn();
    const props = { open: true, kind: 'metric', field: FIELD, onOk, onCancel } as const;

    const { rerender } = render(
      <FieldSettingsModal {...props} initial={{ alias: '', unit: '', format: '' }} />
    );

    fireEvent.change(screen.getByTestId('field-settings-alias'), {
      target: { value: '销售额' },
    });

    // 模拟父级重渲染：store 值未变，但 initial 是新对象（引用已变）。
    // 若 effect 直接依赖 initial，这里会把用户输入重置回空串。
    rerender(<FieldSettingsModal {...props} initial={{ alias: '', unit: '', format: '' }} />);

    expect(screen.getByTestId('field-settings-alias')).toHaveValue('销售额');
  });
});
