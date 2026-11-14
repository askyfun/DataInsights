import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { Button, Input, Space, Typography } from 'antd';
import type { TextAreaRef } from 'antd/es/input/TextArea';
import { useMemo, useRef } from 'react';
import { useIntl } from 'react-intl';
import { type ExprErrorCode, expressionFieldRef, validateExpr } from '../../lib/expr';

const { Text } = Typography;

export interface ExpressionEditorProps {
  /** antd Form.Item 注入 */
  value?: string;
  /** antd Form.Item 注入 */
  onChange?: (value: string) => void;
  /** 可插入/可引用的数据集字段名；同时用于未知字段检查。 */
  fields?: readonly string[];
  rows?: number;
  placeholder?: string;
}

/**
 * 计算字段表达式编辑器（issue #181）。
 *
 * 职责：把纯文本框升级为「可插入字段 + 即时校验」的输入体。校验只是体验层，
 * 权威围栏仍是后端 internal/expr（保存时 UpdateColumns、查询期 buildColumnIndex）。
 * 字段存在性检查是前端独有的提示，不改变后端行为。
 *
 * DatasetDetail 的虚拟字段弹窗与 ChartBuilder 的「新建计算字段」共用本组件，
 * 不要两处各写一份编辑逻辑。
 */
const ExpressionEditor: React.FC<ExpressionEditorProps> = ({
  value,
  onChange,
  fields,
  rows = 3,
  placeholder,
}) => {
  const intl = useIntl();
  const textAreaRef = useRef<TextAreaRef>(null);
  const text = value ?? '';
  const validation = useMemo(() => validateExpr(text, fields), [text, fields]);

  const insertField = (name: string) => {
    const textarea = textAreaRef.current?.resizableTextArea?.textArea;
    const start = textarea?.selectionStart ?? text.length;
    const end = textarea?.selectionEnd ?? text.length;
    const ref = expressionFieldRef(name);
    const next = `${text.slice(0, start)}${ref}${text.slice(end)}`;
    onChange?.(next);
    // 光标落在插入内容之后，便于继续输入运算符。
    requestAnimationFrame(() => {
      const el = textAreaRef.current?.resizableTextArea?.textArea;
      if (!el) {
        return;
      }
      el.focus();
      const caret = start + ref.length;
      el.setSelectionRange(caret, caret);
    });
  };

  const errorMessage = (code: ExprErrorCode, detail?: string) => {
    const values = { name: detail ?? '' };
    const messages: Record<ExprErrorCode, string> = {
      empty: intl.formatMessage({ id: 'expr.error.empty' }),
      unexpected_token: intl.formatMessage({ id: 'expr.error.unexpectedToken' }),
      unbalanced_parenthesis: intl.formatMessage({ id: 'expr.error.unbalancedParenthesis' }),
      reserved_keyword: intl.formatMessage({ id: 'expr.error.reservedKeyword' }, values),
      function_not_allowed: intl.formatMessage({ id: 'expr.error.functionNotAllowed' }, values),
      nesting_too_deep: intl.formatMessage({ id: 'expr.error.nestingTooDeep' }),
      comment_not_allowed: intl.formatMessage({ id: 'expr.error.commentNotAllowed' }),
      string_backslash: intl.formatMessage({ id: 'expr.error.stringBackslash' }),
      unterminated_string: intl.formatMessage({ id: 'expr.error.unterminatedString' }),
      quoted_identifier: intl.formatMessage({ id: 'expr.error.quotedIdentifier' }),
      unknown_field: intl.formatMessage({ id: 'expr.error.unknownField' }, values),
    };
    return messages[code];
  };

  const showStatus = text.trim().length > 0;
  const invalid = !validation.ok;

  return (
    <div data-testid="expression-editor">
      <Input.TextArea
        ref={textAreaRef}
        value={text}
        onChange={(event) => onChange?.(event.target.value)}
        rows={rows}
        placeholder={placeholder}
        status={showStatus && invalid ? 'error' : undefined}
        data-testid="expression-input"
      />
      {fields && fields.length > 0 && (
        <div style={{ marginTop: 6 }} data-testid="expression-fields">
          <Text type="secondary" style={{ fontSize: 12, marginRight: 6 }}>
            {intl.formatMessage({ id: 'expr.insertField' })}:
          </Text>
          <Space size={4} wrap>
            {fields.map((name) => (
              <Button
                key={name}
                size="small"
                onClick={() => insertField(name)}
                data-testid={`expression-insert-${name}`}
                style={{ fontSize: 12 }}
              >
                {name}
              </Button>
            ))}
          </Space>
        </div>
      )}
      {showStatus && (
        <div style={{ marginTop: 6 }} data-testid="expression-status">
          {validation.ok ? (
            <Text type="success" style={{ fontSize: 12 }}>
              <CheckCircleOutlined /> {intl.formatMessage({ id: 'expr.valid' })}
            </Text>
          ) : (
            <Text type="danger" style={{ fontSize: 12 }}>
              <CloseCircleOutlined /> {errorMessage(validation.code, validation.detail)}
            </Text>
          )}
        </div>
      )}
    </div>
  );
};

export default ExpressionEditor;
