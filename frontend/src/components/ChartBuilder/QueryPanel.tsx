import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Card, Select, Space } from 'antd';
import { type ChartField, type QueryConfig } from '../../store';

interface QueryPanelProps {
  fields: ChartField[];
  queryConfig: QueryConfig;
  onReconcileGroupFields: (
    groupType: 'dimension' | 'metric',
    groupId: string,
    selectedFields: string[]
  ) => void;
  onAddDimensionGroup: () => void;
  onRemoveDimensionGroup: (id: string) => void;
  onAddMetricGroup: () => void;
  onRemoveMetricGroup: (id: string) => void;
}

const QueryPanel: React.FC<QueryPanelProps> = ({
  fields,
  queryConfig,
  onReconcileGroupFields,
  onAddDimensionGroup,
  onRemoveDimensionGroup,
  onAddMetricGroup,
  onRemoveMetricGroup,
}) => {
  const dimensionFields = fields.filter((f) => f.type === 'dimension');
  const metricFields = fields.filter((f) => f.type === 'metric');

  const getFieldOptions = (type: 'dimension' | 'metric') => {
    const filteredFields = type === 'dimension' ? dimensionFields : metricFields;
    return filteredFields.map((f) => ({
      value: f.name,
      label: f.name,
    }));
  };

  const handleGroupFieldsChange = (
    groupId: string,
    type: 'dimension' | 'metric',
    values: string[]
  ) => {
    // 交给 store action 原子完成：按 values reconcile 目标组的 bindings，并在同一次更新里
    // 清理被取消选中 bindingId 的五个元数据 Record（防止 nextBindingId(max+1) 复用号跨列污染）。
    onReconcileGroupFields(type, groupId, values);
  };

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Card
        title="维度"
        size="small"
        extra={
          queryConfig.dimensionGroups.length > 0 && (
            <Button
              type="text"
              danger
              icon={<DeleteOutlined />}
              onClick={() => onRemoveDimensionGroup(queryConfig.dimensionGroups[0].id)}
              size="small"
            />
          )
        }
      >
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder="选择维度字段（支持多选）"
          value={queryConfig.dimensionGroups[0]?.bindings.map((b) => b.fieldId) || []}
          onChange={(values) =>
            handleGroupFieldsChange(
              queryConfig.dimensionGroups[0]?.id || 'default-dim',
              'dimension',
              values
            )
          }
          options={getFieldOptions('dimension')}
        />
        {queryConfig.dimensionGroups.length === 0 && (
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={onAddDimensionGroup}
            block
            style={{ marginTop: 8 }}
          >
            添加维度组（透视表）
          </Button>
        )}
        {queryConfig.dimensionGroups.length > 1 && (
          <Space style={{ marginTop: 8 }}>
            {queryConfig.dimensionGroups.slice(1).map((group, index) => (
              <Select
                key={group.id}
                mode="multiple"
                style={{ minWidth: 150 }}
                placeholder={`维度组 ${index + 2}`}
                value={group.bindings.map((b) => b.fieldId)}
                onChange={(values) => handleGroupFieldsChange(group.id, 'dimension', values)}
                options={getFieldOptions('dimension')}
                suffixIcon={
                  <DeleteOutlined
                    style={{ color: '#ff4d4f', cursor: 'pointer' }}
                    onClick={() => onRemoveDimensionGroup(group.id)}
                  />
                }
              />
            ))}
            <Button
              type="dashed"
              size="small"
              icon={<PlusOutlined />}
              onClick={onAddDimensionGroup}
            >
              添加
            </Button>
          </Space>
        )}
      </Card>

      <Card
        title="指标"
        size="small"
        extra={
          queryConfig.metricGroups.length > 0 && (
            <Button
              type="text"
              danger
              icon={<DeleteOutlined />}
              onClick={() => onRemoveMetricGroup(queryConfig.metricGroups[0].id)}
              size="small"
            />
          )
        }
      >
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder="选择指标字段（支持多选）"
          value={queryConfig.metricGroups[0]?.bindings.map((b) => b.fieldId) || []}
          onChange={(values) =>
            handleGroupFieldsChange(
              queryConfig.metricGroups[0]?.id || 'default-metric',
              'metric',
              values
            )
          }
          options={getFieldOptions('metric')}
        />
        {queryConfig.metricGroups.length === 0 && (
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={onAddMetricGroup}
            block
            style={{ marginTop: 8 }}
          >
            添加指标组（双轴图）
          </Button>
        )}
        {queryConfig.metricGroups.length > 1 && (
          <Space style={{ marginTop: 8 }}>
            {queryConfig.metricGroups.slice(1).map((group, index) => (
              <Select
                key={group.id}
                mode="multiple"
                style={{ minWidth: 150 }}
                placeholder={`指标组 ${index + 2}`}
                value={group.bindings.map((b) => b.fieldId)}
                onChange={(values) => handleGroupFieldsChange(group.id, 'metric', values)}
                options={getFieldOptions('metric')}
                suffixIcon={
                  <DeleteOutlined
                    style={{ color: '#ff4d4f', cursor: 'pointer' }}
                    onClick={() => onRemoveMetricGroup(group.id)}
                  />
                }
              />
            ))}
            <Button type="dashed" size="small" icon={<PlusOutlined />} onClick={onAddMetricGroup}>
              添加
            </Button>
          </Space>
        )}
      </Card>
    </div>
  );
};

export default QueryPanel;
