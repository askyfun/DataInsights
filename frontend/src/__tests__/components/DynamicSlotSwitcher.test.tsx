import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import DynamicSlotSwitcher from '@/components/ChartBuilder/DynamicSlotSwitcher';
import type { DynamicSlot } from '@/lib/dynamicFieldSlots';

const slots: DynamicSlot[] = [
  {
    bindingId: 'b-0',
    label: '地区',
    defaultFieldId: 'd-1',
    options: [
      { id: 'd-1', name: 'region' },
      { id: 'd-2', name: 'city' },
    ],
  },
];

describe('DynamicSlotSwitcher', () => {
  it('没有动态槽位时不渲染', () => {
    const { container } = render(
      <DynamicSlotSwitcher slots={[]} selections={{}} onChange={() => {}} />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('默认显示槽位默认列，切换后上报 bindingId + 列 id', async () => {
    const onChange = vi.fn();
    render(
      <DynamicSlotSwitcher slots={slots} selections={{}} onChange={onChange} testIdPrefix="t" />
    );

    const select = screen.getByTestId('t-b-0');
    expect(screen.getByTestId('t-switcher').textContent).toContain('地区');
    expect(select.textContent).toContain('region');

    fireEvent.mouseDown(select);
    fireEvent.click(
      await screen.findByText('city', { selector: '.ant-select-item-option-content' })
    );

    await waitFor(() => expect(onChange).toHaveBeenCalledWith('b-0', 'd-2'));
  });

  it('会话里已有有效选择时显示该选择（而不是默认列）', () => {
    render(<DynamicSlotSwitcher slots={slots} selections={{ 'b-0': 'd-2' }} onChange={() => {}} />);
    expect(screen.getByTestId('dynamic-slot-b-0').textContent).toContain('city');
  });
});
