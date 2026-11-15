// Package setup 提供首启向导（R-101 / issue #226）所需的最小状态探测：
// 只回布尔与版本，绝不回任何用户信息，供匿名首屏决定是否渲染向导。
package setup

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Status 是 GET /api/setup/status 的响应载荷（全为布尔/版本，无 PII）。
type Status struct {
	NeedsAdmin     bool   `json:"needs_admin"`
	HasSample      bool   `json:"has_sample"`
	HasAnyResource bool   `json:"has_any_resource"`
	Version        string `json:"version"`
}

type Service struct {
	db      *bun.DB
	version string
}

func NewService(db *bun.DB, version string) *Service {
	return &Service{db: db, version: version}
}

// statusSQL 用一条只读聚合查询取三件事：是否存在任何管理员（含禁用/软删，
// 与注册关闭判据一致）、是否已载入示例数据集（tags 含 sample）、是否存在任意资源。
// 这是只读状态聚合，不参与图表取数链路（图表 SQL 唯一出口仍是 bun_builder）。
const statusSQL = `
SELECT
  (SELECT COUNT(*) FROM bi_user WHERE role = 'admin') AS admins,
  EXISTS(SELECT 1 FROM bi_dataset WHERE deleted_at IS NULL AND tags::text LIKE '%sample%') AS has_sample,
  (EXISTS(SELECT 1 FROM bi_datasource WHERE deleted_at IS NULL AND name <> 'tablewave internal storage (upload files)')
    OR EXISTS(SELECT 1 FROM bi_dataset WHERE deleted_at IS NULL)
    OR EXISTS(SELECT 1 FROM bi_chart WHERE deleted_at IS NULL)
    OR EXISTS(SELECT 1 FROM bi_dashboard WHERE deleted_at IS NULL)) AS has_any`

func (s *Service) Status(ctx context.Context) (*Status, error) {
	var admins int
	var hasSample, hasAny bool
	if err := s.db.NewRaw(statusSQL).Scan(ctx, &admins, &hasSample, &hasAny); err != nil {
		return nil, fmt.Errorf("setup status: %w", err)
	}
	return &Status{
		NeedsAdmin:     admins == 0,
		HasSample:      hasSample,
		HasAnyResource: hasAny,
		Version:        s.version,
	}, nil
}
