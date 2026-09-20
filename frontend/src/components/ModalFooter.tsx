import { Space } from 'antd';
import React from 'react';

export interface ModalFooterProps {
  children: React.ReactNode;
}

/** 弹窗/表单底部按钮行：撑满宽度、右对齐 */
const ModalFooter: React.FC<ModalFooterProps> = ({ children }) => (
  <Space style={{ width: '100%', justifyContent: 'flex-end' }}>{children}</Space>
);

export default ModalFooter;
