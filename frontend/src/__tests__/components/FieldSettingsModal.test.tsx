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

/**
 * 占比开关（issue #132）。
 *
 * 占比不是第二份配置，而是「格式」串的一个 `%` 后缀：弹窗里拆成输入框 + 开关两个
 * 视图，提交时折回一个字段。若两处各自可写，就会出现"输入框里没有 % 但开关是开的"
 * 这种无法自洽的状态。
 */
describe('FieldSettingsModal 占比开关', () => {
  // antd Switch 的可点击体是外层 button（testid 挂在内层 span 上）；确定按钮在
  // Modal footer 里，getByRole 在 jsdom 下拿不到它的可及名，两处都走 DOM 查询。
  const clickPercentSwitch = () => {
    const inner = screen.getByTestId('field-settings-percent');
    const button = inner.closest('button');
    if (!button) {
      throw new Error('占比开关的 button 容器未找到');
    }
    fireEvent.click(button);
  };
  const clickModalOk = () => {
    const ok = document.querySelector<HTMLElement>('.ant-modal-footer .ant-btn-primary');
    if (!ok) {
      throw new Error('弹窗确定按钮未找到');
    }
    fireEvent.click(ok);
  };

  it('格式带 % 时：输入框只显示不含后缀的部分，开关为开', () => {
    renderModal({ initial: { alias: '', unit: '', format: '0,0.00%' } });
    expect(screen.getByTestId('field-settings-format')).toHaveValue('0,0.00');
    expect(screen.getByTestId('field-settings-percent')).toBeChecked();
  });

  it('打开开关后确定：格式补上 % 后缀', () => {
    const { onOk } = renderModal({ initial: { alias: '', unit: '', format: '0,0.00' } });
    clickPercentSwitch();
    clickModalOk();
    expect(onOk).toHaveBeenCalledWith({ alias: '', unit: '', format: '0,0.00%' });
  });

  it('关闭开关后确定：剥掉 % 后缀，其余格式保留', () => {
    const { onOk } = renderModal({ initial: { alias: '', unit: '', format: '0,0.00%' } });
    clickPercentSwitch();
    clickModalOk();
    expect(onOk).toHaveBeenCalledWith({ alias: '', unit: '', format: '0,0.00' });
  });

  it('格式为空时打开开关：给一个两位小数的默认格式，而不是孤零零一个 %', () => {
    const { onOk } = renderModal({ initial: { alias: '', unit: '', format: '' } });
    clickPercentSwitch();
    clickModalOk();
    expect(onOk).toHaveBeenCalledWith({ alias: '', unit: '', format: '0,0.00%' });
  });
});
