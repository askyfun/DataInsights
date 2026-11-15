import type { SelectProps as AntSelectProps } from 'antd';
import { Select as AntSelect } from 'antd';
import { useIntl } from 'react-intl';

/**
 * 统一 Select 封装（#264）：默认开启关键词搜索，大小写不敏感、按展示文本（label）过滤。
 *
 * 适用：选项数不可预知的下拉（数据源 / 表 / 数据集 / 图表 / 字段）。
 * 数值型或固定小枚举等不需要搜索的场景，显式传 `allowSearch={false}` 关闭。
 *
 * ⚠️ 使用 `options`（含 label）形态时过滤最准；JSX 子节点形态建议同样补 `label`。
 * 未显式给 `optionFilterProp` 时默认按 `label` 过滤。
 */
export interface SelectProps<ValueType = unknown> extends AntSelectProps<ValueType> {
  /** 是否允许关键词搜索，默认 true；固定枚举/数值型下拉传 false */
  allowSearch?: boolean;
}

function Select<ValueType = unknown>(props: SelectProps<ValueType>) {
  const intl = useIntl();
  const { allowSearch = true, showSearch, optionFilterProp, notFoundContent, ...rest } = props;

  if (!allowSearch) {
    return <AntSelect<ValueType> {...rest} showSearch={false} notFoundContent={notFoundContent} />;
  }

  return (
    <AntSelect<ValueType>
      {...rest}
      showSearch={showSearch ?? true}
      optionFilterProp={optionFilterProp ?? 'label'}
      notFoundContent={notFoundContent ?? intl.formatMessage({ id: 'common.noMatch' })}
    />
  );
}

export { Select };
export default Select;
