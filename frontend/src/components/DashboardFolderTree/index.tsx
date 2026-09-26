import {
  DashboardOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import type { MenuProps, TreeDataNode } from 'antd';
import { App, Button, Dropdown, Form, Input, Modal, Space, Tree, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useIntl } from 'react-intl';
import type { Dashboard, DashboardFolder } from '../../api';
import {
  buildDashboardFolderTree,
  dashboardIdFromKey,
  type FolderSelection,
  folderIdFromKey,
  isFolderDropAllowed,
  ROOT_KEY,
  resolveTreeMove,
  selectionFromKey,
  targetFolderIdForKey,
} from '../../lib/dashboardFolderTree';

const { Text } = Typography;

/** 树节点上多带的三个字段：allowDrop / nodeDraggable / onDrop 都靠它们判别形态。 */
interface FolderTreeDataNode extends TreeDataNode {
  kind: 'folder' | 'dashboard';
  folderId: string | null;
  dashboardCount: number;
}

/**
 * 仪表盘归档树（左侧整列，可折叠）。
 *
 * 只做「渲染 + 交互」，不碰 API：所有写操作由父页面注入，这样页面的取数/刷新与树
 * 的渲染各自可测（组树纯逻辑在 lib/dashboardFolderTree，落库在页面）。
 *
 * 盘与夹**都可拖动**（验收要求两者都能移动）。夹的拖拽多一条守卫：不能拖进自己的
 * 后代。那条规则在 allowDrop 里当场挡掉（连落点高亮都不给），而不是等后端回 20400
 * —— 用户已经把手松开了才告诉他「不能放这儿」是最差的反馈。
 */
interface DashboardFolderTreeProps {
  folders: DashboardFolder[];
  dashboards: Dashboard[];
  selection: FolderSelection;
  onSelect: (selection: FolderSelection) => void;
  /** 新建夹：parentId 为 null = 根级。返回 false 表示失败（弹窗保持打开）。 */
  onCreateFolder: (parentId: string | null, name: string) => Promise<boolean>;
  /** 改名。返回 false 表示失败。 */
  onRenameFolder: (id: string, name: string) => Promise<boolean>;
  onDeleteFolder: (id: string) => Promise<void>;
  /** 拖拽移动仪表盘：folderId 为 null = 移出文件夹（未归档）。 */
  onMoveDashboard: (dashboardId: string, folderId: string | null) => Promise<void>;
  /** 拖拽移动文件夹：folderId 为 null = 移到根级。 */
  onMoveFolder: (folderId: string, parentId: string | null) => Promise<void>;
}

type DialogState =
  | { mode: 'none' }
  | { mode: 'create'; parentId: string | null }
  | { mode: 'rename'; id: string };

const DashboardFolderTree: React.FC<DashboardFolderTreeProps> = ({
  folders,
  dashboards,
  selection,
  onSelect,
  onCreateFolder,
  onRenameFolder,
  onDeleteFolder,
  onMoveDashboard,
  onMoveFolder,
}) => {
  const intl = useIntl();
  // message / modal 取 App 上下文实例（静态方法读不到 ConfigProvider 的主题上下文），
  // 与 Dashboards 页同一口径。
  const { message, modal } = App.useApp();
  const [form] = Form.useForm<{ name: string }>();
  const [dialog, setDialog] = useState<DialogState>({ mode: 'none' });
  const [submitting, setSubmitting] = useState(false);
  // 受控的右键菜单：⋮ 按钮与右键两个入口共用同一个实例。
  const [menuKey, setMenuKey] = useState<string | null>(null);

  const rootTitle = intl.formatMessage({ id: 'dashboard.folderRoot' });

  // 展开集合走受控 + 增量合并，不用 defaultExpandAll：树数据是异步到达的
  // （首帧只有根节点），defaultExpandAll 只在挂载那一刻生效，之后新出现的夹
  // 全是收起状态 —— 表现就是「文件夹建好了但看不见」，新建后尤其明显。
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  useEffect(() => {
    // 根节点始终算「已展开」，否则子级不渲染。
    const keys = [ROOT_KEY, ...folders.map((f) => `f:${f.id}`)];
    setExpandedKeys((prev) => {
      const merged = new Set(prev);
      let changed = false;
      for (const k of keys) {
        if (!merged.has(k)) {
          merged.add(k);
          changed = true;
        }
      }
      return changed ? [...merged] : prev;
    });
  }, [folders]);

  const treeData = useMemo<FolderTreeDataNode[]>(() => {
    const walk = (n: ReturnType<typeof buildDashboardFolderTree>): FolderTreeDataNode => ({
      key: n.key,
      title: n.title,
      icon:
        n.kind === 'dashboard' ? (
          <DashboardOutlined />
        ) : n.folderId === null ? (
          <FolderOpenOutlined />
        ) : (
          <FolderOutlined />
        ),
      children: (n.children ?? []).map(walk),
      kind: n.kind,
      folderId: n.folderId,
      dashboardCount: n.dashboardCount,
    });
    return [walk(buildDashboardFolderTree(folders, dashboards, rootTitle))];
  }, [folders, dashboards, rootTitle]);

  const folderNameOf = (id: string): string => folders.find((f) => f.id === id)?.name ?? '';

  const openCreate = (parentId: string | null) => {
    form.resetFields();
    setDialog({ mode: 'create', parentId });
  };

  const openRename = (id: string) => {
    form.setFieldsValue({ name: folderNameOf(id) });
    setDialog({ mode: 'rename', id });
  };

  const confirmDelete = (id: string) => {
    modal.confirm({
      title: intl.formatMessage({ id: 'dashboard.folderDeleteConfirm' }),
      okText: intl.formatMessage({ id: 'common.yes' }),
      cancelText: intl.formatMessage({ id: 'common.no' }),
      okButtonProps: { danger: true },
      onOk: () => onDeleteFolder(id),
    });
  };

  const submitDialog = async () => {
    const values = await form.validateFields();
    const name = values.name.trim();
    // 后端对空串是 20100，但 trim 后为空是纯前端形态问题：当场挡住，别发一次必然失败的请求。
    if (!name) {
      message.error(intl.formatMessage({ id: 'dashboard.folderEmptyName' }));
      return;
    }
    setSubmitting(true);
    try {
      if (dialog.mode === 'create') {
        if (await onCreateFolder(dialog.parentId, name)) setDialog({ mode: 'none' });
      } else if (dialog.mode === 'rename') {
        if (await onRenameFolder(dialog.id, name)) setDialog({ mode: 'none' });
      }
    } finally {
      setSubmitting(false);
    }
  };

  const menuFor = (node: FolderTreeDataNode): NonNullable<MenuProps['items']> => {
    if (node.kind === 'dashboard') return [];
    if (node.key === ROOT_KEY) {
      return [
        {
          key: 'new',
          label: intl.formatMessage({ id: 'dashboard.folderNewRoot' }),
          onClick: () => openCreate(null),
        },
      ];
    }
    const id = folderIdFromKey(String(node.key));
    if (!id) return [];
    return [
      {
        key: 'newSub',
        label: intl.formatMessage({ id: 'dashboard.folderNewSub' }),
        onClick: () => openCreate(id),
      },
      {
        key: 'rename',
        label: intl.formatMessage({ id: 'dashboard.folderRename' }),
        onClick: () => openRename(id),
      },
      { type: 'divider' },
      {
        key: 'delete',
        label: intl.formatMessage({ id: 'dashboard.folderDelete' }),
        danger: true,
        onClick: () => confirmDelete(id),
      },
    ];
  };

  return (
    <>
      <Tree<FolderTreeDataNode>
        blockNode
        showIcon
        expandedKeys={expandedKeys}
        onExpand={(keys) => setExpandedKeys(keys.map(String))}
        treeData={treeData}
        selectedKeys={[selection.kind === 'root' ? ROOT_KEY : `f:${selection.id}`]}
        onSelect={(keys) => {
          // 再次点击已选中节点，antd 会给空数组：此时保持当前视图，不要跳回根。
          if (keys.length === 0) return;
          onSelect(selectionFromKey(String(keys[0])));
        }}
        titleRender={(node) => {
          const key = String(node.key);
          const label = (
            <Space size={4}>
              <span>{String(node.title ?? '')}</span>
              {node.kind === 'folder' && node.dashboardCount > 0 && (
                <Text type="secondary">({node.dashboardCount})</Text>
              )}
              {node.kind === 'folder' && (
                // ⋮ 必须真的能开菜单：只有右键入口的话，这个图标就是一个纯装饰的
                // 假 affordance（E2E 里点它毫无反应）。菜单本身受控，两种入口共用。
                <Button
                  type="text"
                  size="small"
                  icon={<MoreOutlined />}
                  aria-label={intl.formatMessage({ id: 'dashboard.folderActions' })}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation(); // 不要顺带触发节点的选中（会把列表切走）
                    setMenuKey(key);
                  }}
                />
              )}
            </Space>
          );
          const items = menuFor(node);
          if (items.length === 0) return label; // 仪表盘叶子没有菜单，不套 Dropdown
          return (
            <Dropdown
              menu={{ items }}
              trigger={['contextMenu']}
              open={menuKey === key}
              onOpenChange={(open) => setMenuKey(open ? key : null)}
            >
              {label}
            </Dropdown>
          );
        }}
        draggable={{
          icon: false,
          // 根节点没有可移动的实体（它只是「全部」的视图入口）。
          nodeDraggable: (node) => String(node.key) !== ROOT_KEY,
        }}
        allowDrop={({ dragNode, dropNode }) =>
          // 唯一的非法落点：把夹拖进自己的子树。后端同规则会回 20400，但既然在拖动
          // 当场就能判出来，就不该给出一个注定失败的落点（连高亮都不给）。
          // 只吃 key —— 见 targetFolderIdForKey 的说明。
          isFolderDropAllowed(String(dragNode.key), String(dropNode.key), folders)
        }
        onDrop={(info) => {
          const dragKey = String(info.dragNode.key);
          const dropKey = String(info.node.key);
          const dashboardId = dashboardIdFromKey(dragKey);
          const draggedFolderId = folderIdFromKey(dragKey);
          if (!dashboardId && !draggedFolderId) return; // 根节点：不响应

          // 被拖对象当前所在位置：盘看 folder_id，夹看 parent_id。
          const currentFolderId = dashboardId
            ? (dashboards.find((d) => d.id === dashboardId)?.folder_id ?? null)
            : (folders.find((f) => f.id === draggedFolderId)?.parent_id ?? null);

          const intent = resolveTreeMove({
            dragKey,
            dropKey,
            dropNodeFolderId: targetFolderIdForKey(dropKey, dashboards),
            currentFolderId,
          });
          if (!intent) return; // 落点即当前位置：不发无意义的 PUT
          if (intent.kind === 'dashboard') {
            void onMoveDashboard(intent.dashboardId, intent.intoFolderId);
          } else {
            void onMoveFolder(intent.folderId, intent.intoFolderId);
          }
        }}
      />

      <Modal
        open={dialog.mode !== 'none'}
        title={intl.formatMessage({
          id: dialog.mode === 'rename' ? 'dashboard.folderRename' : 'dashboard.folderNewRoot',
        })}
        okText={intl.formatMessage({ id: 'common.confirm' })}
        cancelText={intl.formatMessage({ id: 'common.cancel' })}
        confirmLoading={submitting}
        onOk={submitDialog}
        onCancel={() => setDialog({ mode: 'none' })}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={submitDialog}>
          <Form.Item
            name="name"
            label={intl.formatMessage({ id: 'dashboard.name' })}
            rules={[
              {
                required: true,
                message: intl.formatMessage({ id: 'dashboard.folderNameRequired' }),
              },
            ]}
          >
            <Input autoFocus maxLength={255} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default DashboardFolderTree;
