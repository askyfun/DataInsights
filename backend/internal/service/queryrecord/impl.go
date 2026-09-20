// Package queryrecord 实现查询记录的落库与寻址（R-08' / R-30'）。
//
// 职责边界：只负责 bi_query 这一张表的读写——按 spec_hash 去重 upsert、
// 按 base58 短码还原、以及访问留痕。它不执行查询、不认识数据集/字段，
// 因此 spec_json 对它是不可解释的载荷（信封校验后原样存取）。
//
// 两条与 PRD 一致的生命周期语义：
//   - 去重命中时**不刷新** created_at / expires_at —— 已分享出去的链接不会
//     因为别人又查了一次同样的条件而改变有效期；
//   - 永不物理删除，过期只由 expires_at 单字段判定（NULL = 永久）。
package queryrecord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/idcodec"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	// MaxSpecBytes 是 spec_json 的应用层上限（PRD R-46）：超限拒存而不是截断，
	// 否则还原出来的条件会与用户当时看到的不一致。
	MaxSpecBytes = 32 * 1024

	// TTLDays 是记录默认存活天数，创建时写入 created_at + TTLDays。
	TTLDays = 90

	// SourceTypeBuild / SourceTypeShareURL 是 source_type 的两个约定取值。
	SourceTypeBuild    = "build"
	SourceTypeShareURL = "share_url"

	// maxSourceTypeLen 对应 bi_query.source_type 的 VARCHAR(50)。
	maxSourceTypeLen = 50
)

// Service 是查询记录的服务接口。
type Service interface {
	// Save 落库一次查询配置，返回出地址栏用的短码。
	// 同 dataset_id + 同 spec 内容命中 spec_hash 唯一约束时复用原记录。
	Save(ctx context.Context, in entity.QueryRecordSaveRequest, ip string) (*entity.QueryRecordSaved, error)
	// GetByShortID 按地址栏短码取回记录，并做一次访问留痕。
	GetByShortID(ctx context.Context, shortID string) (*entity.QueryRecord, error)
}

type service struct {
	db *bun.DB
	// now / newID 是注入点：让单测能固定时间与 id，不依赖真实时钟与随机源。
	now   func() time.Time
	newID func() (uuid.UUID, error)
}

// NewService 创建查询记录服务。
func NewService(db *bun.DB) Service {
	return &service{
		db:    db,
		now:   func() time.Time { return time.Now().UTC() },
		newID: func() (uuid.UUID, error) { return uuid.NewV7() },
	}
}

// savedRow 是 upsert 的 RETURNING 落地结构；去重命中时它承载的是**原记录**的三列。
type savedRow struct {
	QueryID   string       `bun:"query_id"`
	CreatedAt time.Time    `bun:"created_at"`
	ExpiresAt sql.NullTime `bun:"expires_at"`
}

// Save 实现 Service。
//
// 落库路径（PRD §5.3）：内容 hash 去重 upsert。去重在数据库层靠 spec_hash 唯一
// 约束完成，因此并发写同一配置是幂等的——后到者走 DO UPDATE 分支拿到同一条
// query_id，不会产生重复行，也不会覆盖原记录的 spec。
func (s *service) Save(
	ctx context.Context, in entity.QueryRecordSaveRequest, ip string,
) (*entity.QueryRecordSaved, error) {
	if in.DatasetID <= 0 {
		return nil, router.NewBusinessError(response.CodeBadRequest, "dataset_id 不能为空")
	}

	sourceType, err := normalizeSourceType(in.SourceType)
	if err != nil {
		return nil, err
	}

	canonicalSpec, specHash, err := canonicalize(in.Spec, in.DatasetID)
	if err != nil {
		return nil, err
	}

	newID, err := s.newID()
	if err != nil {
		return nil, fmt.Errorf("生成 query_id 失败: %w", err)
	}

	now := s.now()
	record := &model.QueryRecord{
		QueryID:    newID.String(),
		SpecJSON:   string(canonicalSpec),
		SpecHash:   specHash,
		DatasetID:  in.DatasetID,
		ChartID:    nullableInt32(in.ChartID),
		SourceType: sourceType,
		RowCount:   nullableIntPtr(in.RowCount),
		DurationMs: nullableIntPtr(in.DurationMs),
		IP:         ip,
		CreatedAt:  now,
		ExpiresAt:  sql.NullTime{Time: now.AddDate(0, 0, TTLDays), Valid: true},
	}

	var saved savedRow
	// 去重命中时只更新结果元信息，绝不触碰 spec / created_at / expires_at /
	// hit_count：已发出的链接必须逐字保持原样。
	//
	// 「未上报则保留」不靠 COALESCE(EXCLUDED.col, bi_query.col) 实现——bun 生成的是
	// `INSERT INTO "bi_query" AS "query_record"`，目标表被起了别名，PG 在 DO UPDATE
	// 里按原名 bi_query 引用会直接报 42P01 invalid reference to FROM-clause entry。
	// 改成按需拼 SET：没上报的列压根不进 SET 子句，语义比 COALESCE 更直接，
	// 也不再依赖 bun 的别名声（那属于内部实现，会随版本变）。
	insert := s.db.NewInsert().Model(record).
		On("CONFLICT (spec_hash) DO UPDATE")
	if record.RowCount.Valid {
		insert = insert.Set("row_count = EXCLUDED.row_count")
	}
	if record.DurationMs.Valid {
		insert = insert.Set("duration_ms = EXCLUDED.duration_ms")
	}
	if !record.RowCount.Valid && !record.DurationMs.Valid {
		// 两个指标都未上报时 SET 子句不能为空（纯语法错误）。兜底表达式只能用
		// 冲突判定列自身：ON CONFLICT (spec_hash) 成立蕴含 EXCLUDED.spec_hash
		// 等于存量值，因此这是一条可证明的空赋值。
		//
		// ⚠️ 绝不能拿 query_id 兜底。`query_id = EXCLUDED.query_id` 看着像自赋值，
		// 实际是把存量行主键改写成本次新生成的 UUID —— 已发出的短码会当场失效
		// （2026-09-19 端到端验证真实踩到：同一 spec 连发三次，前两个短码全部查无此记录）。
		insert = insert.Set("spec_hash = EXCLUDED.spec_hash")
	}
	if err := insert.Returning("query_id, created_at, expires_at").Scan(ctx, &saved); err != nil {
		return nil, fmt.Errorf("保存查询记录失败: %w", err)
	}

	persisted, err := uuid.Parse(saved.QueryID)
	if err != nil {
		return nil, fmt.Errorf("查询记录 query_id 非法: %w", err)
	}
	shortID, err := idcodec.Encode(persisted)
	if err != nil {
		return nil, fmt.Errorf("查询记录无法编码为分享短码: %w", err)
	}

	return &entity.QueryRecordSaved{
		QueryID:   shortID,
		CreatedAt: saved.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt: formatNullableTime(saved.ExpiresAt),
	}, nil
}

// GetByShortID 实现 Service。
//
// 过期记录**照常返回**（PRD R-42 / U-10：直链保活，条件 100% 还原），过期与否
// 只通过 Expired 告诉调用方，由界面决定提示文案。
func (s *service) GetByShortID(ctx context.Context, shortID string) (*entity.QueryRecord, error) {
	id, err := idcodec.Decode(shortID)
	if err != nil {
		return nil, router.NewBusinessError(response.CodeBadRequest, "分享链接无效：id 格式不正确")
	}

	var record model.QueryRecord
	err = s.db.NewSelect().Model(&record).Where("query_id = ?", id.String()).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeNotFound, "查询记录不存在或链接已失效")
	}
	if err != nil {
		return nil, fmt.Errorf("读取查询记录失败: %w", err)
	}

	var spec entity.QuerySpecEnvelope
	if err := json.Unmarshal([]byte(record.SpecJSON), &spec); err != nil {
		return nil, fmt.Errorf("查询记录 spec_json 损坏: %w", err)
	}

	now := s.now()
	out := &entity.QueryRecord{
		QueryID:   shortID,
		Spec:      spec,
		DatasetID: record.DatasetID,
		ChartID:   int32Ptr(record.ChartID),
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt: formatNullableTime(record.ExpiresAt),
		Expired:   record.ExpiresAt.Valid && !record.ExpiresAt.Time.After(now),
		HitCount:  record.HitCount + 1,
	}

	// 访问留痕：hit_count / last_accessed_at 每次读取都更新，expires_at 不刷新
	// （PRD §5.4）。写失败只告警——记账不该让一次本来就成功的读取变成失败。
	record.HitCount++
	record.LastAccessedAt = sql.NullTime{Time: now, Valid: true}
	if _, err := s.db.NewUpdate().Model(&record).
		Column("hit_count", "last_accessed_at").
		Where("query_id = ?", record.QueryID).
		Exec(ctx); err != nil {
		slog.Warn("查询记录访问留痕失败", "query_id", record.QueryID, "error", err)
	}

	return out, nil
}

// canonicalize 校验 spec 信封并产出「规范化 JSON + 内容 hash」。
//
// 规范化用 json.Unmarshal → json.Marshal 往返：Go 的 map 序列化键序稳定（字典序），
// 因此同一份配置无论键序如何书写都得到同一个 hash，去重才成立。落库存的是规范化
// 之后的字节，保证「库里的内容」与「hash 的依据」永远是同一份。
func canonicalize(raw json.RawMessage, datasetID int) (canonical []byte, hash string, err error) {
	spec := bytes.TrimSpace(raw)
	if len(spec) == 0 {
		return nil, "", router.NewBusinessError(response.CodeBadRequest, "spec 不能为空")
	}
	if len(spec) > MaxSpecBytes {
		return nil, "", router.NewBusinessError(
			response.CodeBadRequest,
			fmt.Sprintf("查询配置过大（%d 字节，上限 %d 字节）", len(spec), MaxSpecBytes),
		)
	}

	var envelope entity.QuerySpecEnvelope
	if err := json.Unmarshal(spec, &envelope); err != nil {
		return nil, "", router.NewBusinessError(response.CodeBadRequest, "spec 必须是合法 JSON")
	}
	if envelope.V != entity.CurrentQuerySpecVersion {
		return nil, "", router.NewBusinessError(
			response.CodeBadRequest,
			fmt.Sprintf("不支持的 spec 版本 %d（当前支持 %d）", envelope.V, entity.CurrentQuerySpecVersion),
		)
	}
	document := bytes.TrimSpace(envelope.Document)
	if len(document) == 0 || document[0] != '{' {
		return nil, "", router.NewBusinessError(response.CodeBadRequest, "spec.document 必须是非空 JSON 对象")
	}

	var generic any
	if err := json.Unmarshal(spec, &generic); err != nil {
		return nil, "", router.NewBusinessError(response.CodeBadRequest, "spec 必须是合法 JSON")
	}
	canonical, err = json.Marshal(generic)
	if err != nil {
		return nil, "", fmt.Errorf("规范化 spec 失败: %w", err)
	}

	// dataset_id 参与 hash：同一份图表文档挂在不同数据集上是两个不同的查询，
	// 不参与会把它们错误地合并成同一条记录。
	digest := sha256.New()
	fmt.Fprintf(digest, "%d\n", datasetID)
	digest.Write(canonical)
	return canonical, hex.EncodeToString(digest.Sum(nil)), nil
}

func normalizeSourceType(raw string) (string, error) {
	sourceType := strings.TrimSpace(raw)
	if sourceType == "" {
		return SourceTypeBuild, nil
	}
	if len(sourceType) > maxSourceTypeLen {
		return "", router.NewBusinessError(response.CodeBadRequest, "source_type 过长")
	}
	return sourceType, nil
}

func nullableInt32(value int) sql.NullInt32 {
	if value <= 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(value), Valid: true}
}

func nullableIntPtr(value *int) sql.NullInt32 {
	if value == nil || *value < 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*value), Valid: true}
}

func int32Ptr(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int32)
	return &out
}

func formatNullableTime(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339)
	return &formatted
}
