// 仪表盘文件夹（归档树）的持久化，migration 00007 的服务端对应物。
// 包级文档在 impl.go 顶部，这里只写本文件自己的职责。
//
// 与 dashboard 同包但独立类型：文件夹是「容器的树」，仪表盘是「容器里的叶子」，
// 两者的生命周期守卫不同（夹要查子夹/子盘/环，盘只要 layout 校验）。分开两个
// Service 接口，cmd/routes.go 各自装配一个 handler，也各自 mock 一个。
//
// 树的存在形式是本文件最重要的约定：
//   - **落库与出口都是扁平的**（parent_id 自引用），后端不返回嵌套 children，
//     因此改名/移动只写一行，没有派生结构要同步；
//   - **环由应用层当场拒绝**（DB 无递归 CHECK），做法是沿 target 的祖先链上溯，
//     而不是把整棵子树拉下来比对——上溯的代价是「夹深」而非「夹内子项数」；
//   - **删除不级联**（第一期「仅空夹可删」）：级联删仪表盘会连带销毁整份
//     layout_json，而第一期没有批量移动端点让用户先把子项挪走，所以宁可拒绝。
//
// 两条与 dashboard 一致的红线：读路径恒带 `deleted_at IS NULL`；永不物理删除。
package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// maxFolderDepth 是祖先上溯的步数上限。它不是产品规格而是**止损**：万一历史数据
// 里已经存在环（守卫上线前写下的），上溯会无限循环；走到上限就当环拒绝。
const maxFolderDepth = 100

// FolderService defines the interface for dashboard folder operations.
type FolderService interface {
	// ListFolders returns every not-soft-deleted folder, flat, oldest first —
	// the frontend assembles the tree from parent_id.
	ListFolders(ctx context.Context) ([]entity.DashboardFolder, error)
	// GetFolder returns one folder by UUID (missing/soft-deleted → 20300).
	GetFolder(ctx context.Context, id string) (*entity.DashboardFolder, error)
	// CreateFolder persists a new folder; parentID "" means root level.
	CreateFolder(ctx context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error)
	// UpdateFolder renames and/or moves a folder (three-state ParentID, see
	// entity.DashboardFolderUpdateRequest). Moving into a descendant is 20400.
	UpdateFolder(ctx context.Context, id string, in entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error)
	// DeleteFolder soft-deletes an **empty** folder. Folders still holding
	// not-deleted child folders or dashboards answer 20400 and delete nothing.
	DeleteFolder(ctx context.Context, id string) error
}

// folderService implements FolderService.
type folderService struct {
	db *bun.DB
	// now / newID are the same test seams as dashboardService: fixed clock and
	// id in unit tests, no real clock or entropy dependency.
	now   func() time.Time
	newID func() (uuid.UUID, error)
}

// NewFolderService creates a new FolderService.
func NewFolderService(db *bun.DB) FolderService {
	return &folderService{
		db:    db,
		now:   func() time.Time { return time.Now().UTC() },
		newID: func() (uuid.UUID, error) { return uuid.NewV7() },
	}
}

// ListFolders returns the flat folder list, ordered `created_at ASC, id ASC`.
//
// 升序（而非 dashboard 的 DESC）是有意的：建树时先序遍历的输出应当「父在前、
// 建得早的在前」，前端按这个顺序排兄弟节点，新建的夹不会插到旧夹前面。
func (s *folderService) ListFolders(ctx context.Context) ([]entity.DashboardFolder, error) {
	var rows []model.DashboardFolder
	err := s.db.NewSelect().Model(&rows).
		Where("deleted_at IS NULL").
		OrderExpr("created_at ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list dashboard folders: %w", err)
	}
	out := make([]entity.DashboardFolder, 0, len(rows))
	for i := range rows {
		out = append(out, toDashboardFolderEntity(&rows[i]))
	}
	return out, nil
}

// GetFolder returns a folder by its UUID. A missing (or soft-deleted) row is a
// NotFound business error; any other failure stays an internal error so a
// database outage is not reported as 404.
func (s *folderService) GetFolder(ctx context.Context, id string) (*entity.DashboardFolder, error) {
	m, err := s.getFolderModel(ctx, id)
	if err != nil {
		return nil, err
	}
	e := toDashboardFolderEntity(m)
	return &e, nil
}

// CreateFolder persists a new folder. The id is a server-generated UUIDv7; an
// empty ParentID means root level.
//
// 父级存在性在这里**是**校验的（与 dashboard.folder_id 不校验相反）：夹的树语义
// 完全依赖 parent_id 可解析，写进一个悬空父级会让子夹在前端静默掉到根级，那是
// 「树长错了」而不是「归档标记悬空」，属于必须当场拒绝的输入。
func (s *folderService) CreateFolder(ctx context.Context, in entity.DashboardFolderCreateRequest) (*entity.DashboardFolder, error) {
	if in.Name == "" {
		return nil, router.NewBusinessError(response.CodeBadRequest, "文件夹名称不能为空")
	}

	parentID, err := s.resolveParentID(ctx, in.ParentID)
	if err != nil {
		return nil, err
	}

	id, err := s.newID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate dashboard folder id: %w", err)
	}
	now := s.now()
	m := &model.DashboardFolder{
		ID:        id.String(),
		Name:      in.Name,
		ParentID:  parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create dashboard folder: %w", err)
	}
	e := toDashboardFolderEntity(m)
	return &e, nil
}

// UpdateFolder renames and/or moves a folder.
//
// 整行读出来再写回（不是裸 UPDATE），和 dashboardService.Update 一致，因此
// ExcludeColumn("deleted_at") 是必须的：模型里的 DeletedAt 是零值，让它进 SET
// 会把已软删的行复活（troubleshooting T-9）。WHERE 上的 deleted_at IS NULL
// 同时兜住「更新已删行」。
func (s *folderService) UpdateFolder(ctx context.Context, id string, in entity.DashboardFolderUpdateRequest) (*entity.DashboardFolder, error) {
	m, err := s.getFolderModel(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		if *in.Name == "" {
			return nil, router.NewBusinessError(response.CodeBadRequest, "文件夹名称不能为空")
		}
		m.Name = *in.Name
	}

	// ParentID 三态：nil = 不动（跳过整块），"" = 移到根级，UUID = 移到该夹下。
	if in.ParentID != nil {
		if *in.ParentID == "" {
			m.ParentID = sql.NullString{}
		} else {
			if *in.ParentID == m.ID {
				return nil, router.NewBusinessError(response.CodeBusinessError, "文件夹不能移动到自身")
			}
			parentID, err := s.resolveParentID(ctx, *in.ParentID)
			if err != nil {
				return nil, err
			}
			if err := s.ensureNotDescendant(ctx, m.ID, *in.ParentID); err != nil {
				return nil, err
			}
			m.ParentID = parentID
		}
	}
	m.UpdatedAt = s.now()

	if _, err := s.db.NewUpdate().Model(m).
		WherePK().
		Where("deleted_at IS NULL").
		ExcludeColumn("deleted_at").
		ExcludeColumn("created_at").
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update dashboard folder: %w", err)
	}
	e := toDashboardFolderEntity(m)
	return &e, nil
}

// DeleteFolder soft-deletes an empty folder only.
//
// 守卫顺序是「先子夹后子盘」，两条都是 COUNT 查询：任一非零就返回 20400 并带上
// 具体数量，让前端能把「还有 3 个文件夹」这类话直接说给用户，而不是一个笼统的
// 「删除失败」。幂等性来自 `deleted_at IS NULL` 这个 WHERE：重复删除影响 0 行、
// 不报错，与 dashboard/chart 的软删同例。
func (s *folderService) DeleteFolder(ctx context.Context, id string) error {
	if _, err := s.getFolderModel(ctx, id); err != nil {
		return err
	}

	subFolders, err := s.countChildFolders(ctx, id)
	if err != nil {
		return err
	}
	if subFolders > 0 {
		return router.NewBusinessError(response.CodeBusinessError,
			fmt.Sprintf("文件夹下还有 %d 个子文件夹，请先移动或删除它们", subFolders))
	}

	dashboards, err := s.countChildDashboards(ctx, id)
	if err != nil {
		return err
	}
	if dashboards > 0 {
		return router.NewBusinessError(response.CodeBusinessError,
			fmt.Sprintf("文件夹下还有 %d 个仪表盘，请先移动或删除它们", dashboards))
	}

	if _, err := s.db.NewUpdate().
		Model((*model.DashboardFolder)(nil)).
		Set("deleted_at = now()").
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete dashboard folder: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 内部守卫
// ---------------------------------------------------------------------------

// getFolderModel loads one live folder row.
func (s *folderService) getFolderModel(ctx context.Context, id string) (*model.DashboardFolder, error) {
	m := &model.DashboardFolder{ID: id}
	err := s.db.NewSelect().Model(m).WherePK().Where("deleted_at IS NULL").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeNotFound, "dashboard folder not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard folder: %w", err)
	}
	return m, nil
}

// resolveParentID turns the wire form of a parent into the column form. "" is
// root level (NULL); anything else must be a live UUID-bearing row.
//
// 非 UUID 直接 20100：parent_id 是 uuid 列，畸形串交给 PostgreSQL 只会变成一个
// 转换 500，把「你传错了」报成「服务器坏了」。
func (s *folderService) resolveParentID(ctx context.Context, raw string) (sql.NullString, error) {
	if raw == "" {
		return sql.NullString{}, nil
	}
	if _, err := uuid.Parse(raw); err != nil {
		return sql.NullString{}, router.NewBusinessError(response.CodeBadRequest, "parent_id 必须是合法的 UUID")
	}
	if _, err := s.getFolderModel(ctx, raw); err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: raw, Valid: true}, nil
}

// ensureNotDescendant rejects moving `id` under `targetID` when targetID is a
// descendant of id (which would close a cycle).
//
// 方向是关键：从 target 沿 parent 链**往上爬**，只要爬到 id 就说明 target 在 id
// 的子树里。爬链的代价是树的深度，而不是 id 子树里的节点总数——后者在深夹上会
// 变成一次全子树拉取。每一步都是一条 WHEREPK 查询，因为祖先链不保证连续。
func (s *folderService) ensureNotDescendant(ctx context.Context, id, targetID string) error {
	cursor := targetID
	for depth := 0; depth < maxFolderDepth; depth++ {
		parent, err := s.parentIDOf(ctx, cursor)
		if err != nil {
			return err
		}
		if !parent.Valid {
			return nil // 爬到根了：target 不是 id 的后代
		}
		if parent.String == id {
			return router.NewBusinessError(response.CodeBusinessError, "文件夹不能移动到自身的子文件夹下")
		}
		cursor = parent.String
	}
	return router.NewBusinessError(response.CodeBusinessError, "文件夹层级过深或存在环，移动被拒绝")
}

// parentIDOf reads one row's parent_id without touching the rest of the folder.
func (s *folderService) parentIDOf(ctx context.Context, id string) (sql.NullString, error) {
	var parent sql.NullString
	err := s.db.NewSelect().
		Model((*model.DashboardFolder)(nil)).
		Column("parent_id").
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Scan(ctx, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		// 祖先链断裂（中间某个夹已被软删）：环不成立，放行；悬空父级按根级展示。
		return sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to inspect folder ancestry: %w", err)
	}
	return parent, nil
}

// countChildFolders counts live direct children (not the whole subtree — the
// delete guard only ever asks about the direct level, because deleting a parent
// never touches grandchildren in this phase).
func (s *folderService) countChildFolders(ctx context.Context, id string) (int, error) {
	count, err := s.db.NewSelect().
		Model((*model.DashboardFolder)(nil)).
		Where("parent_id = ?", id).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count child folders: %w", err)
	}
	return count, nil
}

// countChildDashboards counts live dashboards archived in this folder.
func (s *folderService) countChildDashboards(ctx context.Context, id string) (int, error) {
	count, err := s.db.NewSelect().
		Model((*model.Dashboard)(nil)).
		Where("folder_id = ?", id).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count dashboards in folder: %w", err)
	}
	return count, nil
}

func toDashboardFolderEntity(m *model.DashboardFolder) entity.DashboardFolder {
	e := entity.DashboardFolder{
		ID:        m.ID,
		Name:      m.Name,
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: m.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if m.ParentID.Valid {
		e.ParentID = &m.ParentID.String
	}
	return e
}
