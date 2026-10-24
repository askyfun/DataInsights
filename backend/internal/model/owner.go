// Package model 的归属助手：R-95（issue #220）把六个 create 路径的 owner_id
// 从占位 0 升级为「有已认证主体则写真实用户 id，匿名仍写 SystemOwnerID」。
package model

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// SystemOwnerID is the placeholder owner id recorded for rows created while the
// platform has no accounts (L0), and the value backfilled onto pre-existing rows
// by migration 00009. Zero is reserved so it never collides with a real
// bi_user.id (its autoincrement sequence starts at 1). #183 replaces this with
// the authenticated user's id taken from the request context; until then every
// create path stamps SystemOwnerID and Update never rewrites the column.
const SystemOwnerID int32 = 0

// ResolveOwnerID 把 handler 注入的 principal 用户 id 换算成要落库的 owner_id：
// 有已认证主体（id > 0）写真实用户 id，匿名请求保持系统占位 0（口径与 #171 一致，
// 0 永不与真实 bi_user.id 冲突）。调用方只做赋值，不感知占位语义。
func ResolveOwnerID(principalUserID int) int32 {
	if principalUserID > 0 {
		return int32(principalUserID)
	}
	return SystemOwnerID
}

// OwnerNames 按一批 owner_id 查 bi_user 的用户名，返回 id → username。
// 刻意不过滤 disabled/deleted_at（R-95 验收 4：禁用不删归属，名字照常显示）；
// 查不到的 id 不在返回 map 里，调用方据此留空（owner_id=0 的存量/匿名行）。
func OwnerNames(ctx context.Context, db bun.IDB, ownerIDs []int32) (map[int32]string, error) {
	names := make(map[int32]string, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return names, nil
	}
	var users []User
	if err := db.NewSelect().Model(&users).Where("id IN (?)", bun.In(ownerIDs)).Scan(ctx, &users); err != nil {
		return nil, fmt.Errorf("failed to resolve owner names: %w", err)
	}
	for _, u := range users {
		names[int32(u.ID)] = u.Username
	}
	return names, nil
}
