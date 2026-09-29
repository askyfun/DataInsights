import { Alert, Checkbox, Modal, Select, Space, Typography } from 'antd';
import React, { useEffect, useMemo, useState } from 'react';
import { useIntl } from 'react-intl';
import type { DashboardLinkageTarget } from '@/lib/dashboardLayoutSchema';
import { linkageDefaultColumn } from '@/lib/dashboardLinkage';

const { Text } = Typography;

/** 目标数据集的列（id + 展示名），供跨数据集联动选关联字段。 */
export interface LinkageColumnOption {
  id: string;
  name: string;
}

/** 一个可勾选的目标图表块。 */
export interface LinkageCandidate {
  /** 目标图表块的 widgetId。 */
  widgetId: string;
  /** 展示名（块标题或图表名）。 */
  label: string;
  /** 目标图表所属数据集（判断同/跨数据集 → 默认落点）。 */
  datasetId?: number;
}

export interface ChartLinkageSettingsProps {
  open: boolean;
  /** 联动来源：盘内的哪个 chart 块、它的联动键列、以及块是否已落库。 */
  source: {
    datasetId?: number;
    /** 来源的联动键列；null = 该图不能作联动来源（多维度 / 图型不支持点击）。 */
    keyColumn: string | null;
    /** 未落库的块后端读不到它的联动配置（盘级取数按已落库 layout 建索引）。 */
    unsaved: boolean;
  } | null;
  /** 盘内其它图表块（调用方已排除来源自己）。 */
  candidates: readonly LinkageCandidate[];
  /** 目标数据集 id → 可选列。缺该数据集的列时（加载失败）跨数据集目标无法配置。 */
  columnsByDataset: Readonly<Record<number, readonly LinkageColumnOption[]>>;
  /** 打开时的初值（已保存的联动配置）。 */
  initialTargets: readonly DashboardLinkageTarget[];
  onOk: (targets: DashboardLinkageTarget[]) => void;
  onCancel: () => void;
}

/**
 * 图表联动设置弹窗（issue #143）。
 *
 * 只收集「这块图被点击时，要把值下发到哪些图表块的哪一列上」——**取值**不在这里配，
 * 由用户在盘上点击数据项产生。同数据集的候选默认沿用来源的联动键列（最常见的
 * 「省份点省份」），跨数据集的必须显式选一列（列名撞名不是关联依据）。
 *
 * 落点列写进布局文档（`linkage.targets`），请求里只带「点了什么值」——与后端
 * `buildLinkageOverrides` 的信任边界一致。
 */
const ChartLinkageSettings: React.FC<ChartLinkageSettingsProps> = ({
  open,
  source,
  candidates,
  columnsByDataset,
  initialTargets,
  onOk,
  onCancel,
}) => {
  const intl = useIntl();
  /** 勾选的目标 → 落点列 ID（空串 = 还没选定，不能确定）。 */
  const [selected, setSelected] = useState<Record<string, string>>({});

  // 打开时按已保存配置重置：沿用上次残留的选择会让「确定」看起来可用，
  // 实际配置的是别的目标。
  //
  // ⚠️ `initialTargets` 必须由调用方保持引用稳定（useMemo 自来源块派生），否则这个
  // effect 每渲染都会重跑、覆盖用户刚做的勾选。
  useEffect(() => {
    if (!open) {
      return;
    }
    const seeded: Record<string, string> = {};
    for (const target of initialTargets) {
      seeded[target.widgetId] = target.column;
    }
    setSelected(seeded);
  }, [open, initialTargets]);

  const missingColumn = useMemo(
    () => Object.values(selected).some((column) => column === ''),
    [selected]
  );

  const toggle = (candidate: LinkageCandidate, checked: boolean) => {
    setSelected((prev) => {
      if (!checked) {
        const next = { ...prev };
        delete next[candidate.widgetId];
        return next;
      }
      return {
        ...prev,
        [candidate.widgetId]:
          prev[candidate.widgetId] ??
          linkageDefaultColumn(source?.keyColumn ?? null, source?.datasetId, candidate.datasetId),
      };
    });
  };

  const canOk = source !== null && source.keyColumn !== null && !missingColumn;

  return (
    <Modal
      open={open}
      title={intl.formatMessage({ id: 'dashboard.linkageSettings' })}
      width={560}
      okText={intl.formatMessage({ id: 'common.confirm' })}
      cancelText={intl.formatMessage({ id: 'common.cancel' })}
      okButtonProps={{ disabled: !canOk, 'data-testid': 'linkage-ok' }}
      onOk={() => {
        if (!canOk) {
          return;
        }
        // 只提交仍在盘内的目标：块被移除后残留的悬空引用在这里被顺手清掉
        // （它们没有可勾选的候选行，用户无从取消）。
        const candidateSet = new Set(candidates.map((item) => item.widgetId));
        onOk(
          Object.entries(selected)
            .filter(([widgetId]) => candidateSet.has(widgetId))
            .map(([widgetId, column]) => ({ widgetId, column }))
        );
      }}
      onCancel={onCancel}
      destroyOnHidden
    >
      <Space orientation="vertical" size={12} style={{ width: '100%' }}>
        <Text type="secondary">{intl.formatMessage({ id: 'dashboard.linkageSettingsDesc' })}</Text>

        {source && source.keyColumn === null && (
          <Alert
            type="warning"
            showIcon
            title={intl.formatMessage({ id: 'dashboard.linkageNoKeyColumn' })}
          />
        )}

        {source?.unsaved && (
          <Alert
            type="info"
            showIcon
            title={intl.formatMessage({ id: 'dashboard.linkageNeedSave' })}
          />
        )}

        {source && source.keyColumn !== null && candidates.length === 0 && (
          <Alert
            type="info"
            showIcon
            title={intl.formatMessage({ id: 'dashboard.linkageNoCandidate' })}
          />
        )}

        {source &&
          source.keyColumn !== null &&
          candidates.map((candidate) => {
            const checked = candidate.widgetId in selected;
            const crossDataset = candidate.datasetId !== source?.datasetId;
            const options = (candidate.datasetId && columnsByDataset[candidate.datasetId]) || [];
            return (
              <div key={candidate.widgetId}>
                <Space align="center" size={8} style={{ width: '100%' }}>
                  <Checkbox
                    checked={checked}
                    data-testid={`linkage-target-${candidate.widgetId}`}
                    onChange={(event) => toggle(candidate, event.target.checked)}
                  >
                    {candidate.label}
                  </Checkbox>
                  {checked && crossDataset && (
                    <Select
                      size="small"
                      style={{ minWidth: 200 }}
                      value={selected[candidate.widgetId] || null}
                      placeholder={intl.formatMessage({ id: 'dashboard.linkageColumnPlaceholder' })}
                      aria-label={intl.formatMessage({ id: 'dashboard.linkageColumn' })}
                      options={options.map((column) => ({ value: column.id, label: column.name }))}
                      notFoundContent={intl.formatMessage({ id: 'dashboard.linkageNoColumns' })}
                      onChange={(value: string) =>
                        setSelected((prev) => ({ ...prev, [candidate.widgetId]: value }))
                      }
                    />
                  )}
                </Space>
                {checked && crossDataset && (
                  <Text type="secondary" style={{ display: 'block', marginLeft: 24, fontSize: 12 }}>
                    {intl.formatMessage({ id: 'dashboard.linkageCrossDatasetHint' })}
                  </Text>
                )}
              </div>
            );
          })}
      </Space>
    </Modal>
  );
};

export default ChartLinkageSettings;
