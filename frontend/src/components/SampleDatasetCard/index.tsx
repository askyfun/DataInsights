import { DeleteOutlined, RocketOutlined } from '@ant-design/icons';
import { Alert, Button, Space } from 'antd';
import { useIntl } from 'react-intl';
import type { SampleDatasetStatus } from '../../api';

interface SampleDatasetCardProps {
  status: SampleDatasetStatus | null;
  /** 数据集列表是否为空：引导卡片只在「无数据可分析」时出现。 */
  empty: boolean;
  loading?: boolean;
  onLoad: () => void;
  onDismiss: () => void;
  onRemove: () => void;
}

/**
 * 内置示例数据集（R-79）的引导 / 已加载提示条。
 * 纯展示组件：状态与动作由页面传入，便于单测三态（加载 / 忽略 / 已加载）。
 */
const SampleDatasetCard: React.FC<SampleDatasetCardProps> = ({
  status,
  empty,
  loading,
  onLoad,
  onDismiss,
  onRemove,
}) => {
  const intl = useIntl();
  if (!status) return null;

  if (status.loaded) {
    return (
      <Alert
        data-testid="sample-dataset-loaded"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        title={intl.formatMessage({ id: 'sample.loaded' })}
        action={
          <Button
            size="small"
            danger
            icon={<DeleteOutlined />}
            loading={loading}
            onClick={onRemove}
          >
            {intl.formatMessage({ id: 'sample.remove' })}
          </Button>
        }
      />
    );
  }

  if (!empty || status.dismissed) return null;

  return (
    <Alert
      data-testid="sample-dataset-onboarding"
      type="info"
      showIcon
      style={{ marginBottom: 16 }}
      title={intl.formatMessage({ id: 'sample.title' })}
      description={intl.formatMessage({ id: 'sample.description' })}
      action={
        <Space>
          <Button size="small" disabled={loading} onClick={onDismiss}>
            {intl.formatMessage({ id: 'sample.dismiss' })}
          </Button>
          <Button
            size="small"
            type="primary"
            icon={<RocketOutlined />}
            loading={loading}
            onClick={onLoad}
          >
            {intl.formatMessage({ id: 'sample.load' })}
          </Button>
        </Space>
      }
    />
  );
};

export default SampleDatasetCard;
