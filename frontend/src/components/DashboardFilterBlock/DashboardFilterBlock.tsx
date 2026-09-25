import { DeleteOutlined, SettingOutlined } from '@ant-design/icons';
import { Button, Card, InputNumber, Select, Space, Typography } from 'antd';
import React, { useEffect, useState } from 'react';
import { datasetsApi } from '@/api';
// 算子中文名复用图表侧的导出（`FilterConfigModal` 里还有一份重复的表，属既有重复，未一并合并）。
import { OPERATOR_LABELS } from '@/components/ChartBuilder/FilterDropZone';
import DateFilterControl from '@/components/DateFilter/DateFilterControl';
import {
  dateFilterWidgetSettings,
  FAMILY_OPERATORS,
  filterWidgetFamily,
} from '@/lib/dashboardFilterValue';
import type { DashboardFilterWidget } from '@/lib/dashboardLayoutSchema';
import { type DateFilterValue, isDateFilterValue } from '@/lib/dateFilter';
import type { FilterOperator } from '@/store';

const { Text } = Typography;

/**
 * 从枚举查询结果里取值。
 *
 * 请求只带一个维度，所以每行恰好一个键；但**键名是列名**，而块里只有列 ID
 * （列名可能已被改过、也可能取自另一个同名上游），按键名索引不可靠 —— 直接取该行的值。
 */
function extractCandidates(rows: readonly Record<string, unknown>[]): string[] {
  const values = new Set<string>();
  for (const row of rows) {
    const cells = Object.values(row ?? {});
    const raw = cells.find((cell) => cell !== null && cell !== undefined);
    if (raw === undefined) continue;
    const text = String(raw);
    if (text !== '') values.add(text);
  }
  return [...values].sort((a, b) => a.localeCompare(b, 'zh'));
}

export interface DashboardFilterBlockProps {
  widget: DashboardFilterWidget;
  /** 原始取值：日期族是 `DateFilterValue`，字符串/数值族是 `unknown[]`。 */
  value: unknown;
  /**
   * 该块尚未落库。盘级取数是**后端按已落库的 layout 逐块取数**，所以未保存前这个筛选器
   * 还作用不到任何图表——不提示的话看起来就像「筛选坏了」。
   */
  unsaved?: boolean;
  onChange: (next: unknown) => void;
  /** 换算子（数值族在块上直接改）。 */
  onOperatorChange: (operator: FilterOperator) => void;
  /** 打开完整日期筛选弹窗（粒度 / 快捷选项 / 高级起止 / 特殊值）——仅日期族。 */
  onConfigure: () => void;
  onRemove: () => void;
}

/**
 * 仪表盘上的一枚盘级筛选器块，按字段族给不同控件：
 *   - `date`：行内日期控件（高频切换）+「配置」进完整弹窗（一次性设置）；
 *   - `string`：枚举多/单选，候选值实查该列（与图表侧筛选用同一套取值口径）；
 *   - `number`：算子下拉 + 数值输入（`between` 给两个输入）。
 *
 * 三族共用同一份取值下发契约（见 `lib/dashboardFilterValue.ts`），所以块只负责「收值」。
 */
const DashboardFilterBlock: React.FC<DashboardFilterBlockProps> = ({
  widget,
  value,
  unsaved,
  onChange,
  onOperatorChange,
  onConfigure,
  onRemove,
}) => {
  const family = filterWidgetFamily(widget);
  const { datasetId, column } = widget.binding;
  const [candidates, setCandidates] = useState<string[]>([]);
  const [candidatesLoading, setCandidatesLoading] = useState(false);

  // 枚举候选值：只有字符串族需要，且只在绑定列变化时重查（cancelled 挡乱序响应）。
  useEffect(() => {
    if (family !== 'string') {
      return;
    }
    let cancelled = false;
    setCandidatesLoading(true);
    datasetsApi
      .queryDistinct(datasetId, column)
      .then((response) => {
        if (cancelled) return;
        setCandidates(extractCandidates(response.data.data ?? []));
      })
      .catch(() => {
        // 候选值取不到不该让筛选器不可用：留空列表，用户仍然可以改算子/去保存。
        if (!cancelled) setCandidates([]);
      })
      .finally(() => {
        if (!cancelled) setCandidatesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [family, datasetId, column]);

  const renderBody = () => {
    if (family === 'date') {
      const dateValue: DateFilterValue = isDateFilterValue(value) ? value : { kind: 'dynamic' };
      return (
        <DateFilterControl
          label={widget.label}
          value={dateValue}
          settings={dateFilterWidgetSettings(widget)}
          withTime={dateFilterWidgetSettings(widget).withTime}
          allowModeSwitch
          allowClear
          onChange={(next) => onChange(next)}
          onClear={() => onChange({ kind: 'dynamic' })}
        />
      );
    }

    if (family === 'string') {
      const selected: unknown[] = Array.isArray(value) ? value : [];
      return (
        <Select
          style={{ width: '100%' }}
          mode={widget.multi ? 'multiple' : undefined}
          value={widget.multi ? selected : (selected[0] ?? undefined)}
          placeholder="选择值"
          data-testid="dashboard-filter-values"
          showSearch
          allowClear
          loading={candidatesLoading}
          options={candidates.map((item) => ({ value: item, label: item }))}
          onChange={(next) => {
            if (Array.isArray(next)) {
              onChange(next);
              return;
            }
            onChange(next === undefined || next === null ? [] : [next]);
          }}
        />
      );
    }

    // number：算子 +（区间给两个输入，单侧给一个）
    const numbers: unknown[] = Array.isArray(value) ? value : [];
    const toNumber = (input: unknown): number | null => {
      if (input === null || input === undefined || input === '') return null;
      const parsed = Number(input);
      return Number.isFinite(parsed) ? parsed : null;
    };
    const isRange = widget.operator === 'between';
    const first = toNumber(numbers[0]);
    const second = toNumber(numbers[1]);

    return (
      <Space orientation="vertical" size={8} style={{ width: '100%' }}>
        <Select
          size="small"
          style={{ width: '100%' }}
          value={widget.operator}
          data-testid="dashboard-filter-operator"
          options={FAMILY_OPERATORS.number.map((operator) => ({
            value: operator,
            label: OPERATOR_LABELS[operator as keyof typeof OPERATOR_LABELS] ?? operator,
          }))}
          // 算子词表在这里是普通字符串（FAMILY_OPERATORS），到边界收成 FilterOperator。
          onChange={(next) => onOperatorChange(next as FilterOperator)}
        />
        {isRange ? (
          <Space>
            <InputNumber
              style={{ width: 110 }}
              placeholder="下界"
              value={first}
              data-testid="dashboard-filter-min"
              onChange={(next) => onChange([next, second])}
            />
            <Text type="secondary">~</Text>
            <InputNumber
              style={{ width: 110 }}
              placeholder="上界"
              value={second}
              data-testid="dashboard-filter-max"
              onChange={(next) => onChange([first, next])}
            />
          </Space>
        ) : (
          <InputNumber
            style={{ width: '100%' }}
            placeholder="数值"
            value={first}
            data-testid="dashboard-filter-number"
            onChange={(next) => onChange(next === null ? [] : [next])}
          />
        )}
      </Space>
    );
  };

  return (
    <Card
      size="small"
      title={widget.label}
      extra={
        <>
          {family === 'date' && (
            <Button
              type="text"
              size="small"
              icon={<SettingOutlined />}
              aria-label="配置筛选器"
              data-testid="dashboard-filter-configure"
              onClick={onConfigure}
            />
          )}
          <Button
            type="text"
            size="small"
            danger
            icon={<DeleteOutlined />}
            aria-label="移除筛选器"
            data-testid="dashboard-filter-remove"
            onClick={onRemove}
          />
        </>
      }
      style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
      styles={{ body: { flex: 1, minHeight: 0, padding: 12, overflow: 'auto' } }}
    >
      {renderBody()}
      {unsaved && (
        <Text type="secondary" style={{ display: 'block', marginTop: 8, fontSize: 12 }}>
          保存仪表盘后，该筛选条件才会作用到图表。
        </Text>
      )}
    </Card>
  );
};

export default DashboardFilterBlock;
