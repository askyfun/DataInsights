import { LoadingOutlined, SearchOutlined } from '@ant-design/icons';
import { Button, Checkbox, Empty, Input, Segmented, Select, Typography } from 'antd';
import React, { useCallback, useEffect, useMemo, useState } from 'react';

const { Text } = Typography;

/** 候选值超过该数量时只渲染前 SAMPLE_LIMIT 条，其余经「更多数据项」展开（性能保护）。 */
export const ENUM_SAMPLE_LIMIT = 50;

export type EnumSortOrder = 'asc' | 'desc' | 'manual';

export interface EnumPanelProps {
  /** 候选值全集（含「加入选项」新增项），展示顺序已由宿主排好。 */
  candidates: string[];
  loading: boolean;
  /** 已选值，顺序即「手动」排序的展示顺序。 */
  value: string[];
  onChange: (next: string[]) => void;
  /** 「加入选项」把搜索词注册为新候选（宿主并集展示）。 */
  onAddOption?: (v: string) => void;
  testIdPrefix: string;
}

/**
 * 精确筛选面板（对齐火山「编辑筛选-精确筛选」）：
 * 搜索定位 + 关键词批量选中、查看已选项视图、全选（作用于当前过滤结果）、
 * 排序（升/降/手动）、大数据量抽样 +「更多数据项」。
 * 勾选即值，不引入确认步骤；全选/取消全选按当前视图的可见集合切换。
 */
const EnumPanel: React.FC<EnumPanelProps> = ({
  candidates,
  loading,
  value,
  onChange,
  onAddOption,
  testIdPrefix,
}) => {
  const [search, setSearch] = useState('');
  const [view, setView] = useState<'all' | 'selected'>('all');
  const [sort, setSort] = useState<EnumSortOrder>('asc');
  const [expanded, setExpanded] = useState(false);

  // 「手动」排序以选中顺序为准：宿主值里缺序号的（如外部加入）按出现顺序补齐。
  const [manualOrder, setManualOrder] = useState<string[]>(value);
  useEffect(() => {
    setManualOrder((prev) => {
      const kept = prev.filter((v) => value.includes(v));
      const added = value.filter((v) => !kept.includes(v));
      return added.length ? [...kept, ...added] : kept;
    });
  }, [value]);

  const comparator = useMemo(() => {
    if (sort === 'asc')
      return (a: string, b: string) => a.localeCompare(b, 'zh-Hans-CN', { numeric: true });
    if (sort === 'desc')
      return (a: string, b: string) => b.localeCompare(a, 'zh-Hans-CN', { numeric: true });
    // 手动：已选按选中先后在前，未选恒在后（组内保持字典序），避免未选项挤进选中区。
    const idx = new Map(manualOrder.map((v, i) => [v, i]));
    return (a: string, b: string) => {
      const ia = idx.get(a);
      const ib = idx.get(b);
      if (ia != null && ib != null) return ia - ib;
      if (ia != null) return -1;
      if (ib != null) return 1;
      return a.localeCompare(b, 'zh-Hans-CN', { numeric: true });
    };
  }, [sort, manualOrder]);

  const keyword = search.trim().toLowerCase();
  const matchesSearch = useCallback(
    (v: string) => !keyword || v.toLowerCase().includes(keyword),
    [keyword]
  );

  const visibleAll = useMemo(
    () => candidates.filter(matchesSearch).sort(comparator),
    [candidates, matchesSearch, comparator]
  );
  const visibleSelected = useMemo(
    () => value.filter(matchesSearch).sort(comparator),
    [value, matchesSearch, comparator]
  );

  const source = view === 'all' ? visibleAll : visibleSelected;
  const sampled = !expanded && source.length > ENUM_SAMPLE_LIMIT;
  const visible = sampled ? source.slice(0, ENUM_SAMPLE_LIMIT) : source;

  const allChecked = visible.length > 0 && visible.every((v) => value.includes(v));
  const someChecked = visible.some((v) => value.includes(v));

  const toggleAllVisible = (checked: boolean) => {
    if (checked) {
      const merged = [...value];
      for (const v of visible) if (!merged.includes(v)) merged.push(v);
      onChange(merged);
    } else {
      onChange(value.filter((v) => !visible.includes(v)));
    }
  };

  const toggleOne = (v: string, checked: boolean) => {
    if (checked) onChange([...value, v]);
    else onChange(value.filter((x) => x !== v));
  };

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}>
        <Input
          size="small"
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索候选值"
          style={{ flex: 1 }}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          data-testid={`${testIdPrefix}-search`}
        />
        <Segmented
          size="small"
          value={view}
          onChange={(v) => setView(v as 'all' | 'selected')}
          options={[
            { value: 'all', label: '全部' },
            { value: 'selected', label: `已选(${value.length})` },
          ]}
          data-testid={`${testIdPrefix}-view`}
        />
        <Select<EnumSortOrder>
          size="small"
          style={{ width: 88 }}
          value={sort}
          onChange={setSort}
          options={[
            { value: 'asc', label: '升序' },
            { value: 'desc', label: '降序' },
            { value: 'manual', label: '手动' },
          ]}
          data-testid={`${testIdPrefix}-sort`}
        />
      </div>
      <div
        style={{
          border: '1px solid var(--dr-border)',
          borderRadius: 6,
          padding: '6px 10px',
          maxHeight: 220,
          overflowY: 'auto',
        }}
      >
        <div style={{ padding: '2px 0 6px', borderBottom: '1px solid var(--dr-border)' }}>
          <Checkbox
            checked={allChecked}
            indeterminate={someChecked && !allChecked}
            onChange={(e) => toggleAllVisible(e.target.checked)}
            data-testid={`${testIdPrefix}-select-all`}
          >
            <Text type="secondary" style={{ fontSize: 12 }}>
              {view === 'all' && keyword ? '全选当前搜索结果' : '全选'}
            </Text>
          </Checkbox>
        </div>
        {loading ? (
          <div style={{ padding: 16, textAlign: 'center' }}>
            <LoadingOutlined spin />
          </div>
        ) : visible.length === 0 ? (
          keyword ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                view === 'selected' ? (
                  '无匹配的已选项'
                ) : (
                  <span>
                    未找到「{search.trim()}」
                    <Button
                      type="link"
                      size="small"
                      onClick={() => {
                        const v = search.trim();
                        onChange([...value, v]);
                        onAddOption?.(v);
                      }}
                      data-testid={`${testIdPrefix}-add-option`}
                    >
                      加入选项
                    </Button>
                  </span>
                )
              }
            />
          ) : (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                view === 'selected' ? '尚未选择任何值' : '暂无候选值，可在「手动输入」中粘贴批量值'
              }
            />
          )
        ) : (
          visible.map((v) => (
            <div key={v} style={{ padding: '2px 0' }}>
              <Checkbox
                checked={value.includes(v)}
                onChange={(e) => toggleOne(v, e.target.checked)}
                data-testid={`${testIdPrefix}-item`}
              >
                {v}
              </Checkbox>
            </div>
          ))
        )}
      </div>
      <div style={{ marginTop: 6, display: 'flex', justifyContent: 'space-between' }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          共 {source.length} 项{sampled ? `，已展示前 ${ENUM_SAMPLE_LIMIT} 项` : ''}
        </Text>
        {sampled && (
          <Button
            type="link"
            size="small"
            onClick={() => setExpanded(true)}
            data-testid={`${testIdPrefix}-expand`}
          >
            更多数据项（剩余 {source.length - ENUM_SAMPLE_LIMIT}）
          </Button>
        )}
      </div>
      {visible.length > 0 && (
        <Text type="secondary" style={{ fontSize: 12 }} data-testid={`${testIdPrefix}-count`}>
          已选 {value.length} / {view === 'all' ? visibleAll.length : visibleSelected.length} 项可见
        </Text>
      )}
    </div>
  );
};

export default EnumPanel;
