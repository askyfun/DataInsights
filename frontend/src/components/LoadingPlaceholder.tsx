import { Spin, Typography } from 'antd';
import React from 'react';

export interface LoadingPlaceholderProps {
  /** 占位下方的说明文字 */
  text: string;
}

/** 居中的加载占位：大号 Spin + 次要说明文字 */
const LoadingPlaceholder: React.FC<LoadingPlaceholderProps> = ({ text }) => (
  <div style={{ textAlign: 'center', padding: '100px 0' }}>
    <Spin size="large" />
    <div style={{ marginTop: 16 }}>
      <Typography.Text type="secondary">{text}</Typography.Text>
    </div>
  </div>
);

export default LoadingPlaceholder;
