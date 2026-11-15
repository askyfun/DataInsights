import { Select } from 'antd';
import React from 'react';

/**
 * 分页「每页条数」下拉（#268）。
 *
 * antd v6 Pagination 内置的 SizeChanger 本质是 `showSearch: true` 的 Select → 内部是
 * **可键入的文本输入框**，会被 1Password / Bitwarden 等识别成用户名/密码框而弹出自动
 * 填充。这里换成 `showSearch={false}` 的 Select：输入框为 readonly、只能从固定选项里选，
 * 既不触发搜索，也不会被当成可输入文本框。
 *
 * 通过 Pagination 的 `components={{ sizeChanger }}` 注入（antd v6 官方扩展点）。
 */
export interface PageSizeChangerProps {
  value?: number;
  onChange?: (size: number) => void;
  disabled?: boolean;
  className?: string;
}

/** 生成一个绑定固定候选值列表的 sizeChanger 组件。 */
function makePageSizeChanger(options: number[]) {
  const Comp: React.FC<PageSizeChangerProps> = ({ value, onChange, disabled, className }) => (
    <Select
      showSearch={false}
      size="small"
      className={className}
      disabled={disabled}
      value={value}
      onChange={(v: number) => onChange?.(Number(v))}
      options={options.map((n) => ({ value: n, label: `${n} 条/页` }))}
    />
  );
  Comp.displayName = `PageSizeChanger(${options.join('/')})`;
  return Comp;
}

/** 默认候选（表格图 / 数据集 / 数据源预览）。 */
export const PageSizeChanger = makePageSizeChanger([10, 20, 50, 100]);
/** 透视表候选（与旧 pageSizeOptions ['50','100','200'] 保持一致）。 */
export const PageSizeChangerWide = makePageSizeChanger([50, 100, 200]);

export default PageSizeChanger;
