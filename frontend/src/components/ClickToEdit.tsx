import { Input, Typography } from 'antd';
import React, { useEffect, useRef, useState } from 'react';

const { Text } = Typography;

export interface ClickToEditProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  /** 只读态文本颜色（如维度蓝 / 指标绿）；缺省用正文色。 */
  color?: string;
  style?: React.CSSProperties;
  /** 测试定位钩子。 */
  testId?: string;
  ariaLabel?: string;
}

/**
 * Excel 式「点开即改」单元格：默认渲染为一段普通只读文本（无边框、无输入框痕迹，
 * 悬停时出现浅色底提示可点）；点击后原地变成无边框输入框并聚焦——光标可见、可直接
 * 打字；失焦或回车收起并提交，Esc 还原。阅读态零干扰、编辑态一步直达。
 *
 * 调用场景：数据集详情页字段表格的「字段名」「描述」两列（两列此前交互不一致：
 * 一个常显边框、一个点了不见光标）。整站凡「表格里看起来是文本、实际可改」的
 * 单元格都应复用本组件，保持同一套交互（见 docs/developer-guide/list-page-conventions.md）。
 *
 * 实现：只读态用 div（不是 disabled Input，避免灰字与不可选中）；编辑态用
 * variant="borderless" 的 Input + autoFocus，保证点击即出现光标。
 */
const ClickToEdit: React.FC<ClickToEditProps> = ({
  value,
  onChange,
  placeholder,
  color,
  style,
  testId,
  ariaLabel,
}) => {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const inputInstanceRef = useRef<HTMLInputElement | null>(null);

  // 从只读态进入编辑态时，草稿以最新 value 为起点。
  useEffect(() => {
    if (editing) {
      setDraft(value);
    }
  }, [editing, value]);

  useEffect(() => {
    if (editing) {
      // antd Input 是受控组件，聚焦真实 DOM 元素即可出现光标。
      inputInstanceRef.current?.focus();
      inputInstanceRef.current?.select();
    }
  }, [editing]);

  const commit = () => {
    setEditing(false);
    if (draft !== value) {
      onChange(draft);
    }
  };

  if (editing) {
    return (
      <Input
        ref={(node: unknown) => {
          // antd v5 Input 的 ref 是组件实例，真实 input 挂在 input 属性上。
          inputInstanceRef.current =
            (node as { input?: HTMLInputElement | null } | null)?.input ?? null;
        }}
        value={draft}
        variant="borderless"
        size="small"
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onPressEnter={commit}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setDraft(value);
            setEditing(false);
          }
        }}
        aria-label={ariaLabel}
        data-testid={testId ? `${testId}-input` : undefined}
        style={{
          paddingInline: 4,
          ...style,
        }}
      />
    );
  }

  return (
    // 原生 button 承担点击语义（与 FilterDropZone 的条件行同一做法）：
    // a11y 完整，且不会构成交互元素嵌套。
    <button
      type="button"
      data-testid={testId}
      aria-label={ariaLabel}
      onClick={() => setEditing(true)}
      style={{
        display: 'block',
        width: '100%',
        cursor: 'text',
        minHeight: 22,
        lineHeight: '22px',
        padding: '0 4px',
        borderRadius: 4,
        border: 0,
        background: 'none',
        font: 'inherit',
        color: 'inherit',
        textAlign: 'left',
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        textOverflow: 'ellipsis',
        ...style,
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLButtonElement).style.backgroundColor = 'var(--dr-sunken)';
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLButtonElement).style.backgroundColor = 'transparent';
      }}
    >
      {value ? (
        <Text style={{ color, fontSize: 13 }}>{value}</Text>
      ) : (
        <Text type="secondary" style={{ fontSize: 13 }}>
          {placeholder || '—'}
        </Text>
      )}
    </button>
  );
};

export default ClickToEdit;
