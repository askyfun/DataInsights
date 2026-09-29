import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  HolderOutlined,
  MoreOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  horizontalListSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button, Dropdown, Input, type MenuProps, Space, Typography } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { useIntl } from 'react-intl';
import type { DashboardPage } from '@/lib/dashboardLayoutSchema';

const { Text } = Typography;

export interface DashboardPageTabsProps {
  pages: DashboardPage[];
  activePageId: string;
  /** 只剩一页时不允许删除：盘必须至少有一页，否则「添加图表」无处安放。 */
  canRemove: boolean;
  onActivate: (pageId: string) => void;
  onAdd: () => void;
  onRename: (pageId: string, name: string) => void;
  onDuplicate: (pageId: string) => void;
  onRemove: (pageId: string) => void;
  /** 拖拽落点：把 fromId 页移到 toId 页原来的位置。 */
  onReorder: (fromId: string, toId: string) => void;
}

interface PageTabProps {
  page: DashboardPage;
  active: boolean;
  canRemove: boolean;
  onActivate: () => void;
  onRename: (name: string) => void;
  onDuplicate: () => void;
  onRemove: () => void;
}

/**
 * 单个页面标签。三种手势各走各的通道，互不抢事件：
 *   - 标签体：单击=激活、双击=就地改名；
 *   - 左侧把手：拖拽排序（listeners 只挂在把手上，所以拖拽不会误触发激活）；
 *   - 右侧 ⋮：复制 / 改名 / 删除。
 */
const PageTab: React.FC<PageTabProps> = ({
  page,
  active,
  canRemove,
  onActivate,
  onRename,
  onDuplicate,
  onRemove,
}) => {
  const intl = useIntl();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(page.name);
  const inputRef = useRef<React.ComponentRef<typeof Input>>(null);

  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: page.id,
  });

  useEffect(() => {
    if (editing) {
      setDraft(page.name);
      inputRef.current?.focus({ cursor: 'end' });
    }
  }, [editing, page.name]);

  const commit = () => {
    setEditing(false);
    const trimmed = draft.trim();
    // 空名不接受：改名半途清空后回车不该把标签变成空白（此时保留原名）。
    if (trimmed && trimmed !== page.name) {
      onRename(trimmed);
    }
  };

  const menu: MenuProps = {
    items: [
      {
        key: 'rename',
        icon: <EditOutlined />,
        label: intl.formatMessage({ id: 'dashboard.pageRename' }),
      },
      {
        key: 'duplicate',
        icon: <CopyOutlined />,
        label: intl.formatMessage({ id: 'dashboard.pageDuplicate' }),
      },
      {
        key: 'remove',
        icon: <DeleteOutlined />,
        danger: true,
        disabled: !canRemove,
        label: intl.formatMessage({ id: 'dashboard.pageDelete' }),
      },
    ],
    onClick: ({ key, domEvent }) => {
      domEvent.stopPropagation();
      if (key === 'rename') setEditing(true);
      if (key === 'duplicate') onDuplicate();
      if (key === 'remove') onRemove();
    },
  };

  return (
    <div
      ref={setNodeRef}
      data-testid="dashboard-page-tab"
      data-page-id={page.id}
      data-active={active}
      style={{
        transform: CSS.Translate.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        padding: '2px 4px 2px 2px',
        // 逐条 longhand 而不是 border 简写：同一对象里简写 + longhand 混用会让 React
        // 在重渲染时打「Updating a style property ... conflicting property」告警。
        borderStyle: 'solid',
        borderWidth: 1,
        borderColor: active ? 'var(--dr-accent, #1677ff)' : 'transparent',
        borderRadius: 6,
        background: active ? 'var(--dr-accent-soft, rgba(22,119,255,0.08))' : 'transparent',
      }}
    >
      <Button
        type="text"
        size="small"
        icon={<HolderOutlined />}
        aria-label={intl.formatMessage({ id: 'dashboard.pageDrag' })}
        data-testid="dashboard-page-drag"
        style={{ cursor: 'grab', color: 'var(--dr-text-3, rgba(0,0,0,0.45))' }}
        {...attributes}
        {...listeners}
      />
      {editing ? (
        <Input
          ref={inputRef}
          size="small"
          value={draft}
          data-testid="dashboard-page-rename-input"
          style={{ width: 120 }}
          onChange={(event) => setDraft(event.target.value)}
          onPressEnter={commit}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              setEditing(false);
            }
          }}
          onClick={(event) => event.stopPropagation()}
        />
      ) : (
        <button
          type="button"
          data-testid="dashboard-page-label"
          onClick={onActivate}
          onDoubleClick={() => setEditing(true)}
          style={{
            cursor: 'pointer',
            padding: '0 4px',
            // 长名不撑破标签行：超长省略，完整名靠 title 提示。
            maxWidth: 180,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            // 原生 button 的重置：标签是「看起来像文字」的控件，不吃浏览器默认按钮样式。
            // 逐条 longhand（不用 `font` 简写）以免与下面的 fontWeight 冲突。
            background: 'none',
            borderStyle: 'none',
            borderWidth: 0,
            color: 'inherit',
            fontFamily: 'inherit',
            fontSize: 'inherit',
            lineHeight: 'inherit',
            fontWeight: active ? 600 : 400,
            textAlign: 'inherit',
          }}
          title={page.name}
        >
          {page.name}
        </button>
      )}
      {/*
        destroyOnHidden：antd 关闭下拉后默认把面板留在 DOM 里，于是**关掉菜单那一刻的
        回调闭包会一直挂着**——再打开别的页的菜单时，命中的可能是上一个页的旧菜单项，
        带着上一轮的文档快照去执行（浏览器验收里表现为「删一页却少了两页」）。
        卸载面板即断掉这条陈旧引用的路径。
      */}
      <Dropdown menu={menu} trigger={['click']} destroyOnHidden>
        <Button
          type="text"
          size="small"
          icon={<MoreOutlined />}
          aria-label={intl.formatMessage({ id: 'dashboard.pageActions' })}
          data-testid="dashboard-page-actions"
          onClick={(event) => event.stopPropagation()}
        />
      </Dropdown>
    </div>
  );
};

/**
 * 底部页面标签条（多页面盘）。
 *
 * 拖拽排序用 dnd-kit；`onReorder` 只报「从哪到哪」，重排数组的纯逻辑在
 * `lib/dashboardLayoutSchema.reorderPages`（可单测，不依赖 DOM）。
 */
const DashboardPageTabs: React.FC<DashboardPageTabsProps> = ({
  pages,
  activePageId,
  canRemove,
  onActivate,
  onAdd,
  onRename,
  onDuplicate,
  onRemove,
  onReorder,
}) => {
  const intl = useIntl();
  // 小位移不启动拖拽：把手本身很小，误按一下不该把页面顺序改掉。
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) {
      return;
    }
    onReorder(String(active.id), String(over.id));
  };

  return (
    <div
      data-testid="dashboard-page-tabs"
      style={{
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'wrap',
        gap: 4,
        marginTop: 12,
        paddingTop: 8,
        borderTop: '1px solid var(--dr-border, rgba(5,5,5,0.06))',
      }}
    >
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext
          items={pages.map((page) => page.id)}
          strategy={horizontalListSortingStrategy}
        >
          <Space size={4} wrap>
            {pages.map((page) => (
              <PageTab
                key={page.id}
                page={page}
                active={page.id === activePageId}
                canRemove={canRemove}
                onActivate={() => onActivate(page.id)}
                onRename={(name) => onRename(page.id, name)}
                onDuplicate={() => onDuplicate(page.id)}
                onRemove={() => onRemove(page.id)}
              />
            ))}
          </Space>
        </SortableContext>
      </DndContext>
      <Button
        type="text"
        size="small"
        icon={<PlusOutlined />}
        data-testid="dashboard-page-add"
        onClick={onAdd}
      >
        {intl.formatMessage({ id: 'dashboard.pageAdd' })}
      </Button>
      {!canRemove && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {intl.formatMessage({ id: 'dashboard.pageLastHint' })}
        </Text>
      )}
    </div>
  );
};

export default DashboardPageTabs;
