package queryrecord

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dataray/internal/model"

	"github.com/uptrace/bun"
)

// Store is the persistence boundary for query records. It is an interface so the
// service can be unit-tested against an in-memory implementation (memStore)
// without a live PostgreSQL; bunStore is the production implementation.
type Store interface {
	// Upsert inserts a record, or on a spec_hash conflict increments hit_count
	// and refreshes last_accessed_at WITHOUT touching the original query_id,
	// created_at, or expires_at. This is the deduplication path.
	Upsert(ctx context.Context, rec *model.QueryRecord) error
	// GetByID returns the record by its primary key, or ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.QueryRecord, error)
	// Touch increments hit_count and sets last_accessed_at, but never refreshes
	// expires_at. Used when a record is opened via its id.
	Touch(ctx context.Context, id string) error
	// List returns metadata-only rows (no spec_json) matching the options,
	// already ordered by (created_at DESC, query_id DESC) for keyset paging.
	List(ctx context.Context, opts ListOptions) ([]model.QueryRecord, error)
}

// ListOptions parameterizes List.
type ListOptions struct {
	Limit          int        // page size; 0 → default
	Cursor         string     // opaque keyset cursor from a previous List
	DatasetID      *int       // optional filter
	SourceType     string     // optional filter (e.g. "build", "share_url")
	Keyword        string     // optional substring match against spec_json
	Since          *time.Time // lower bound on created_at; nil → now - listWindow
	IncludeExpired bool       // if false (default) hide soft-expired records
}

func (o ListOptions) pageSize() int {
	if o.Limit <= 0 {
		return defaultListPageSize
	}
	return o.Limit
}

// ListItem is a metadata-only projection of a query record. It deliberately
// omits spec_json so that listing never leaks filter value details in bulk.
type ListItem struct {
	QueryID        string     `json:"query_id"`
	DatasetID      int        `json:"dataset_id"`
	ChartID        *int       `json:"chart_id,omitempty"`
	SourceType     string     `json:"source_type"`
	RowCount       *int       `json:"row_count,omitempty"`
	DurationMs     *int       `json:"duration_ms,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	HitCount       int        `json:"hit_count"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// ListResult is the page returned by Service.List.
type ListResult struct {
	Items      []ListItem `json:"items"`
	NextCursor string     `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}

// encodeCursor packs the keyset pivot (created_at, query_id) into an opaque
// string. We never expose raw timestamps to clients.
func encodeCursor(createdAt time.Time, queryID string) string {
	raw := fmt.Sprintf("%d|%s", createdAt.UnixNano(), queryID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor reverses encodeCursor.
func decodeCursor(s string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("decode cursor: malformed payload %q", s)
	}
	ns, err := parseInt(parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	return time.Unix(0, ns).UTC(), parts[1], nil
}

// --- bunStore (production) ---

type bunStore struct {
	db         *bun.DB
	listWindow time.Duration
}

func newBunStore(db *bun.DB, listWindow time.Duration) *bunStore {
	return &bunStore{db: db, listWindow: listWindow}
}

func (b *bunStore) Upsert(ctx context.Context, rec *model.QueryRecord) error {
	_, err := b.db.NewInsert().
		Model(rec).
		On("CONFLICT (spec_hash) DO UPDATE").
		Set("hit_count = bi_query.hit_count + 1").
		Set("last_accessed_at = now()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert query record: %w", err)
	}
	return nil
}

func (b *bunStore) GetByID(ctx context.Context, id string) (*model.QueryRecord, error) {
	rec := new(model.QueryRecord)
	err := b.db.NewSelect().Model(rec).Where("query_id = ?", id).Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get query record: %w", err)
	}
	return rec, nil
}

func (b *bunStore) Touch(ctx context.Context, id string) error {
	_, err := b.db.NewUpdate().
		Model((*model.QueryRecord)(nil)).
		Set("hit_count = bi_query.hit_count + 1").
		Set("last_accessed_at = now()").
		Where("query_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("touch query record: %w", err)
	}
	return nil
}

func (b *bunStore) List(ctx context.Context, opts ListOptions) ([]model.QueryRecord, error) {
	var recs []model.QueryRecord
	q := b.db.NewSelect().Model(&recs).
		// 仅元数据：不 SELECT spec_json，避免批量泄露条件值明细。
		Column("query_id", "dataset_id", "chart_id", "source_type",
			"row_count", "duration_ms", "created_at",
			"last_accessed_at", "hit_count", "expires_at")

	if !opts.IncludeExpired {
		q = q.Where("expires_at IS NULL OR expires_at > now()")
	}

	since := opts.Since
	if since == nil {
		t := time.Now().Add(-b.listWindow)
		since = &t
	}
	q = q.Where("created_at >= ?", *since)

	if opts.DatasetID != nil {
		q = q.Where("dataset_id = ?", *opts.DatasetID)
	}
	if opts.SourceType != "" {
		q = q.Where("source_type = ?", opts.SourceType)
	}
	if opts.Keyword != "" {
		q = q.Where("spec_json::text ILIKE ?", "%"+opts.Keyword+"%")
	}
	if opts.Cursor != "" {
		ca, cid, err := decodeCursor(opts.Cursor)
		if err != nil {
			return nil, err
		}
		// keyset: 取比游标更"旧"的行（降序排列下的下一页）。
		q = q.Where("(created_at, query_id) < (?, ?)", ca, cid)
	}

	q = q.Order("created_at DESC", "query_id DESC").Limit(opts.pageSize())

	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("list query records: %w", err)
	}
	return recs, nil
}

// --- memStore (in-memory, for unit tests) ---

type memStore struct {
	mu     sync.RWMutex
	byHash map[string]*model.QueryRecord
	byID   map[string]*model.QueryRecord
}

func newMemStore() *memStore {
	return &memStore{
		byHash: make(map[string]*model.QueryRecord),
		byID:   make(map[string]*model.QueryRecord),
	}
}

func (m *memStore) Upsert(_ context.Context, rec *model.QueryRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.byHash[rec.SpecHash]; ok {
		existing.HitCount++
		existing.LastAccessedAt = sql.NullTime{Time: time.Now(), Valid: true}
		return nil
	}
	cp := *rec
	m.byHash[rec.SpecHash] = &cp
	m.byID[rec.QueryID] = &cp
	return nil
}

func (m *memStore) GetByID(_ context.Context, id string) (*model.QueryRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.byID[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, ErrNotFound
}

func (m *memStore) Touch(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[id]; ok {
		r.HitCount++
		r.LastAccessedAt = sql.NullTime{Time: time.Now(), Valid: true}
		return nil
	}
	return ErrNotFound
}

func (m *memStore) List(_ context.Context, opts ListOptions) ([]model.QueryRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	since := opts.Since
	if since == nil {
		t := now.Add(-defaultListWindow)
		since = &t
	}

	var out []*model.QueryRecord
	for _, r := range m.byID {
		if !opts.IncludeExpired && r.ExpiresAt.Valid && !r.ExpiresAt.Time.After(now) {
			continue // 软过期，默认视图隐藏
		}
		if r.CreatedAt.Before(*since) {
			continue
		}
		if opts.DatasetID != nil && r.DatasetID != *opts.DatasetID {
			continue
		}
		if opts.SourceType != "" && r.SourceType != opts.SourceType {
			continue
		}
		if opts.Keyword != "" &&
			!strings.Contains(strings.ToLower(r.SpecJSON), strings.ToLower(opts.Keyword)) {
			continue
		}
		out = append(out, r)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].QueryID > out[j].QueryID
	})

	if opts.Cursor != "" {
		ca, cid, err := decodeCursor(opts.Cursor)
		if err != nil {
			return nil, err
		}
		filtered := out[:0]
		for _, r := range out {
			if tupleLess(r.CreatedAt, r.QueryID, ca, cid) {
				filtered = append(filtered, r)
			}
		}
		out = filtered
	}

	if opts.pageSize() > 0 && len(out) > opts.pageSize() {
		out = out[:opts.pageSize()]
	}

	res := make([]model.QueryRecord, len(out))
	for i, r := range out {
		res[i] = *r
	}
	return res, nil
}

// tupleLess reports whether (aT, aID) is strictly less than (bT, bID)
// lexicographically — used for descending keyset pagination.
func tupleLess(aT time.Time, aID string, bT time.Time, bID string) bool {
	if aT.Before(bT) {
		return true
	}
	if aT.After(bT) {
		return false
	}
	return aID < bID
}
