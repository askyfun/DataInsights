package dashboard

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
)

// zz_ 前缀是本仓库的探针约定（留工作区、不入库的除外——这条**入库**，与
// zz_realpg_test.go 同批）。它对真实 PostgreSQL 跑一遍文件夹链路，专抓 sqlmock
// 抓不到的东西：
//   - uuid 列绑定与 NULL 根级（parent_id / folder_id 都可为 NULL，sqlmock 只比字符串）；
//   - migration 00007 的表/列真的存在（包括 ALTER 加的 bi_dashboard.folder_id）；
//   - 环检测的祖先上溯在真驱动下能跑完（sqlmock 只看语句，不看返回类型转换）；
//   - 删除守卫的两条 COUNT 真的按 parent_id / folder_id 命中。
//
// 未设置 TEST_DATABASE_URL 时跳过，因此不影响 go test ./...。它写入的行在结尾
// **物理删除**（仅此一处允许真删），并且要求目标库已跑过 migration 00007
// （启动后端即会自动跑；缺表时探针直接报错，不会静默跳过）。
func TestZZRealPGFolderRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	db := bun.NewDB(sqldb, pgdialect.New())
	ctx := context.Background()

	// 前置：表必须存在（migration 00007 已跑）。缺表就明确失败，别让人以为链路通了。
	var exists bool
	if err := db.NewRaw(`SELECT to_regclass('bi_dashboard_folder') IS NOT NULL`).Scan(ctx, &exists); err != nil {
		t.Fatalf("探测表存在性: %v", err)
	}
	if !exists {
		t.Fatal("bi_dashboard_folder 不存在：请先启动后端（或手工跑 migration 00007）再执行本探针")
	}
	var folderIDCol bool
	if err := db.NewRaw(`SELECT to_regclass('bi_dashboard') IS NOT NULL
		AND EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_name='bi_dashboard' AND column_name='folder_id')`).Scan(ctx, &folderIDCol); err != nil {
		t.Fatalf("探测 bi_dashboard.folder_id: %v", err)
	}
	if !folderIDCol {
		t.Fatal("bi_dashboard.folder_id 不存在：migration 00007 未生效")
	}

	fsvc := NewFolderService(db).(*folderService)
	dsvc := NewService(db)

	// 收尾物理删除：按 zz 前缀名清，避免残留污染真实列表页。
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM bi_dashboard WHERE name LIKE 'zz-folder-%'`,
			`DELETE FROM bi_dashboard_folder WHERE name LIKE 'zz-folder-%'`,
		} {
			if _, err := db.NewRaw(stmt).Exec(ctx); err != nil {
				t.Errorf("清理失败，请手工执行 %s: %v", stmt, err)
			}
		}
	})

	root, err := fsvc.CreateFolder(ctx, entity.DashboardFolderCreateRequest{Name: "zz-folder-root"})
	if err != nil {
		t.Fatalf("CreateFolder(root): %v", err)
	}
	if root.ParentID != nil {
		t.Errorf("根级夹的 parent_id 落库后应为 NULL, 实际 %q", *root.ParentID)
	}

	kid, err := fsvc.CreateFolder(ctx, entity.DashboardFolderCreateRequest{
		Name: "zz-folder-kid", ParentID: root.ID,
	})
	if err != nil {
		t.Fatalf("CreateFolder(kid): %v", err)
	}
	if kid.ParentID == nil || *kid.ParentID != root.ID {
		t.Fatalf("子夹 parent_id 未正确落库: %+v", kid.ParentID)
	}

	// 悬空父级必须被拒（真 uuid 列上的存在性查询）。
	if _, err := fsvc.CreateFolder(ctx, entity.DashboardFolderCreateRequest{
		Name: "zz-folder-orphan", ParentID: "00000000-0000-0000-0000-000000000000",
	}); err == nil {
		t.Error("指向不存在父级的创建应被拒绝")
	}

	// 归档一个盘进根夹，验证 folder_id 的写入/回读与删除守卫。
	dash, err := dsvc.Create(ctx, entity.DashboardCreateRequest{
		Name: "zz-folder-dash", FolderID: root.ID,
	})
	if err != nil {
		t.Fatalf("Create(dashboard with folder_id): %v", err)
	}
	if dash.FolderID == nil || *dash.FolderID != root.ID {
		t.Errorf("folder_id 未回显: %+v", dash.FolderID)
	}
	back, err := dsvc.Get(ctx, dash.ID)
	if err != nil {
		t.Fatalf("Get(dashboard): %v", err)
	}
	if back.FolderID == nil || *back.FolderID != root.ID {
		t.Fatalf("folder_id 未从库里读回: %+v", back.FolderID)
	}

	// 移出文件夹：""（哨兵）必须写回 NULL。
	cleared, err := dsvc.Update(ctx, dash.ID, entity.DashboardUpdateRequest{FolderID: ptr("")})
	if err != nil {
		t.Fatalf("Update(folder_id=\"\"): %v", err)
	}
	if cleared.FolderID != nil {
		t.Errorf("空串哨兵未把 folder_id 清成 null, 实际 %q", *cleared.FolderID)
	}
	// 移回去，让删除守卫有东西可拦。
	if _, err := dsvc.Update(ctx, dash.ID, entity.DashboardUpdateRequest{FolderID: ptr(root.ID)}); err != nil {
		t.Fatalf("Update(归档回 root): %v", err)
	}

	// 环检测（真库跑通爬链）：把 root 移到它自己的子夹 kid 下 → 必须拒绝。
	if _, err := fsvc.UpdateFolder(ctx, root.ID, entity.DashboardFolderUpdateRequest{
		ParentID: ptr(kid.ID),
	}); err == nil {
		t.Error("移动到自身后代下应被拒绝（环）")
	} else if bizCode(t, err) != 20400 {
		t.Errorf("环的错误码应为 20400, 实际 %d (%v)", bizCode(t, err), err)
	}

	// 纯改名不得动 created_at（整行更新的副作用哨兵）。
	var before, after string
	if err := db.NewRaw(`SELECT created_at::text FROM bi_dashboard_folder WHERE id = ?`, root.ID).Scan(ctx, &before); err != nil {
		t.Fatalf("读 created_at: %v", err)
	}
	renamed, err := fsvc.UpdateFolder(ctx, root.ID, entity.DashboardFolderUpdateRequest{Name: ptr("zz-folder-root2")})
	if err != nil {
		t.Fatalf("UpdateFolder(改名): %v", err)
	}
	if err := db.NewRaw(`SELECT created_at::text FROM bi_dashboard_folder WHERE id = ?`, root.ID).Scan(ctx, &after); err != nil {
		t.Fatalf("读 created_at(改名后): %v", err)
	}
	if before != after {
		t.Errorf("改名把 created_at 刷成了 %q（原 %q）", after, before)
	}
	// 根级夹纯改名后仍应是根级：parent_id 不得被整行更新写成别的值。
	if renamed.ParentID != nil {
		t.Errorf("改名后根级夹的 parent_id 被写坏了: %q", *renamed.ParentID)
	}

	// 移到根级（""）：kid 的 parent 应落回 NULL。
	moved, err := fsvc.UpdateFolder(ctx, kid.ID, entity.DashboardFolderUpdateRequest{ParentID: ptr("")})
	if err != nil {
		t.Fatalf("UpdateFolder(移到根级): %v", err)
	}
	if moved.ParentID != nil {
		t.Errorf("空串哨兵未把 parent 清成 NULL, 实际 %q", *moved.ParentID)
	}

	// 删除守卫：root 下有子夹（kid 已挪走，这里再挪回来）与一个盘。
	if _, err := fsvc.UpdateFolder(ctx, kid.ID, entity.DashboardFolderUpdateRequest{ParentID: ptr(root.ID)}); err != nil {
		t.Fatalf("UpdateFolder(kid 挪回 root): %v", err)
	}
	if err := fsvc.DeleteFolder(ctx, root.ID); err == nil {
		t.Error("非空夹（含子夹/仪表盘）删除应被拒绝")
	} else if bizCode(t, err) != 20400 {
		t.Errorf("非空的错误码应为 20400, 实际 %d", bizCode(t, err))
	}

	// 清空后删除应当成功，且软删行不再出现在列表里。
	// 顺序是「先把盘移出 → 再删子夹 → 最后删根夹」：上面刚把盘归档回 root，
	// 不移出去的话 root 永远非空，删它必然 20400。
	if _, err := dsvc.Update(ctx, dash.ID, entity.DashboardUpdateRequest{FolderID: ptr("")}); err != nil {
		t.Fatalf("Update(把盘移出 root): %v", err)
	}
	if err := fsvc.DeleteFolder(ctx, kid.ID); err != nil {
		t.Fatalf("DeleteFolder(kid 空夹): %v", err)
	}
	if err := fsvc.DeleteFolder(ctx, root.ID); err != nil {
		t.Fatalf("DeleteFolder(root 清空后): %v", err)
	}
	list, err := fsvc.ListFolders(ctx)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	for _, f := range list {
		if f.ID == kid.ID || f.ID == root.ID {
			t.Errorf("软删的夹仍出现在列表里: %+v", f)
		}
	}

	// 模型层表名与真实表对齐（改名会让所有语句指向不存在的表，真库是唯一能抓到的地方）。
	if got := (&model.DashboardFolder{}).TableName(); got != "bi_dashboard_folder" {
		t.Errorf("TableName = %q", got)
	}
}
