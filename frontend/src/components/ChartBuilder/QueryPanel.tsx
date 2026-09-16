import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Card, Select, Space } from 'antd';
import { type ChartField, type QueryConfig, reconcileGroupBindings } from '../../store';

interface QueryPanelProps {
  fields: ChartField[];
  queryConfig: QueryConfig;
  onUpdateConfig: (config: Partial<QueryConfig>) => void;
  onAddDimensionGroup: () => void;
  onRemoveDimensionGroup: (id: string) => void;
  onAddMetricGroup: () => void;
  onRemoveMetricGroup: (id: string) => void;
}

const QueryPanel: React.FC<QueryPanelProps> = ({
  fields,
  queryConfig,
  onUpdateConfig,
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
    const groups = type === 'dimension' ? queryConfig.dimensionGroups : queryConfig.metricGroups;
    // 全局 bindingId 视野：所有维度组 + 指标组的现有 bindings
    const allBindings = [
      ...queryConfig.dimensionGroups.map((g) => g.bindings),
      ...queryConfig.metricGroups.map((g) => g.bindings),
    ];
    const updatedGroups = groups.map((g) =>
      g.id === groupId
        ? { ...g, bindings: reconcileGroupBindings(g.bindings, values, allBindings) }
        : g
    );

    if (type === 'dimension') {
      onUpdateConfig({ dimensionGroups: updatedGroups });
    } else {
      onUpdateConfig({ metricGroups: updatedGroups });
    }
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
          value={queryConfig.dimensionGroups[0]?.bindings.map((b) => b.field) || []}
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
                value={group.bindings.map((b) => b.field)}
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
          value={queryConfig.metricGroups[0]?.bindings.map((b) => b.field) || []}
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
                value={group.bindings.map((b) => b.field)}
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
