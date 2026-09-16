// Package queryrecord implements the "query record" service kernel (PRD Phase 0
// R-08'): every chart query execution is persisted as one row in bi_query,
// addressed by a non-guessable UUID (query_id) and deduplicated by the content
// hash of its spec_json.
//
// Key design decisions (see PRD v2 + v3):
//   - Persistence is asynchronous: a bounded in-process queue + a fixed pool of
//     goroutine workers absorb writes so a synchronous DB round-trip never sits
//     on the query hot path. No Redis/asynq/cron dependency is introduced.
//   - When the queue is full, Record falls back to a synchronous write rather
//     than silently dropping the record.
//   - Expiry is a single column (expires_at); NULL means permanent. We never
//     store a status enum and never physically delete a record. GetByID always
//     returns the record (even if expired); only the default List view hides it.
package queryrecord

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"dataray/internal/model"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	defaultQueueCapacity = 1024
	defaultWorkers       = 4
	defaultTTL           = 90 * 24 * time.Hour // created_at + 90d; NULL also allowed
	defaultListWindow    = 30 * 24 * time.Hour // default List filters last 30d
	defaultMaxSpecBytes  = 32 * 1024           // 32KB hard cap on spec_json
	defaultListPageSize  = 50
)

// RecordInput is the payload for a single query-execution record.
type RecordInput struct {
	SpecJSON   string // already-serialized spec, expected to carry a `v` version field
	DatasetID  int
	ChartID    *int
	SourceType string // e.g. "build", "share_url"
	RowCount   *int
	DurationMs *int
	IP         string
	OwnerID    *int // reserved for future account system (nullable, no filtering)
	TenantID   *int // reserved for future account system (nullable, no filtering)
}

// Service is the query-record service. Construct it with NewService (production,
// backed by PostgreSQL) or NewServiceWithStore (tests, in-memory).
type Service struct {
	store    Store
	clock    func() time.Time
	ttl      time.Duration
	listWin  time.Duration
	maxBytes int

	ch      chan *model.QueryRecord
	workers int

	mu      sync.Mutex // guards ch sends vs. Close
	closing bool
	pending atomic.Int64
	wg      sync.WaitGroup
	closeOnce sync.Once
}

type options struct {
	queueCapacity int
	workers       int
	ttl           time.Duration
	listWindow    time.Duration
	maxSpecBytes  int
	clock         func() time.Time
}

// Option overrides a Service configuration value.
type Option func(*options)

func WithQueueCapacity(n int) Option { return func(o *options) { o.queueCapacity = n } }
func WithWorkers(n int) Option       { return func(o *options) { o.workers = n } }
func WithTTL(d time.Duration) Option { return func(o *options) { o.ttl = d } }
func WithListWindow(d time.Duration) Option {
	return func(o *options) { o.listWindow = d }
}
func WithMaxSpecBytes(n int) Option { return func(o *options) { o.maxSpecBytes = n } }
func WithClock(fn func() time.Time) Option { return func(o *options) { o.clock = fn } }

func applyOptions(opts []Option) *options {
	o := &options{
		queueCapacity: defaultQueueCapacity,
		workers:       defaultWorkers,
		ttl:           defaultTTL,
		listWindow:    defaultListWindow,
		maxSpecBytes:  defaultMaxSpecBytes,
		clock:         time.Now,
	}
	for _, fn := range opts {
		fn(o)
	}
	if o.queueCapacity <= 0 {
		o.queueCapacity = defaultQueueCapacity
	}
	if o.workers <= 0 {
		o.workers = defaultWorkers
	}
	if o.ttl <= 0 {
		o.ttl = defaultTTL
	}
	if o.listWindow <= 0 {
		o.listWindow = defaultListWindow
	}
	if o.maxSpecBytes <= 0 {
		o.maxSpecBytes = defaultMaxSpecBytes
	}
	return o
}

// NewService builds the service backed by PostgreSQL.
func NewService(db *bun.DB, opts ...Option) *Service {
	return newService(newBunStore(db, applyOptions(opts).listWindow), opts...)
}

// NewServiceWithStore builds the service with an explicit Store (used in tests
// to inject the in-memory implementation).
func NewServiceWithStore(store Store, opts ...Option) *Service {
	return newService(store, opts...)
}

func newService(store Store, opts ...Option) *Service {
	o := applyOptions(opts)
	s := &Service{
		store:    store,
		clock:    o.clock,
		ttl:      o.ttl,
		listWin:  o.listWindow,
		maxBytes: o.maxSpecBytes,
		ch:       make(chan *model.QueryRecord, o.queueCapacity),
		workers:  o.workers,
	}
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

// Record persists one query execution. It is asynchronous: the record is handed
// to the in-memory queue and processed by a background worker. If the queue is
// full it falls back to a synchronous write so the record is never dropped.
// A spec_json larger than the byte cap is rejected up front with ErrSpecTooLarge.
func (s *Service) Record(_ context.Context, in RecordInput) error {
	canon, err := canonicalizeSpec([]byte(in.SpecJSON))
	if err != nil {
		return fmt.Errorf("record query: %w", err)
	}
	if len(canon) > s.maxBytes {
		return fmt.Errorf("spec_json size %d bytes exceeds limit %d bytes: %w",
			len(canon), s.maxBytes, ErrSpecTooLarge)
	}

	now := s.clock()
	rec := &model.QueryRecord{
		QueryID:    uuid.NewString(),
		SpecJSON:   string(canon),
		SpecHash:   hashSpec(canon),
		DatasetID:  in.DatasetID,
		SourceType: in.SourceType,
		IP:         in.IP,
		CreatedAt:  now,
		HitCount:   1, // this execution counts as the first hit
		LastAccessedAt: sql.NullTime{Time: now, Valid: true},
		ExpiresAt:  sql.NullTime{Time: now.Add(s.ttl), Valid: true},
		ChartID:    nullInt32(in.ChartID),
		RowCount:   nullInt32(in.RowCount),
		DurationMs: nullInt32(in.DurationMs),
		OwnerID:    nullInt32(in.OwnerID),
		TenantID:   nullInt32(in.TenantID),
	}
	return s.enqueue(rec)
}

// GetByID returns the record by id and records an access (hit_count +1,
// last_accessed_at refreshed). It NEVER refreshes expires_at and NEVER fails on
// a soft-expired record — expired records remain fully restorable.
func (s *Service) GetByID(ctx context.Context, queryID string) (*model.QueryRecord, error) {
	rec, err := s.store.GetByID(ctx, queryID)
	if err != nil {
		return nil, err
	}
	if err := s.store.Touch(ctx, queryID); err != nil {
		slog.Error("queryrecord: touch failed", "query_id", queryID, "err", err)
	}
	return rec, nil
}

// IsExpired reports whether a record is soft-expired: expires_at is set and is at
// or before now. A nil expires_at means permanent.
func (s *Service) IsExpired(rec *model.QueryRecord) bool {
	return rec.ExpiresAt.Valid && !rec.ExpiresAt.Time.After(s.clock())
}

// List returns a metadata-only page of records using keyset pagination over
// (created_at, query_id). The default view filters out soft-expired records and
// records older than the list window; both can be overridden via opts.
func (s *Service) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	fetch := opts
	fetch.Limit = opts.pageSize() + 1 // fetch one extra to detect "has more"

	recs, err := s.store.List(ctx, fetch)
	if err != nil {
		return nil, err
	}

	hasMore := len(recs) > opts.pageSize()
	if hasMore {
		recs = recs[:opts.pageSize()]
	}

	items := make([]ListItem, len(recs))
	var nextCursor string
	for i, r := range recs {
		items[i] = toListItem(&r)
		if i == len(recs)-1 {
			nextCursor = encodeCursor(r.CreatedAt, r.QueryID)
		}
	}
	if !hasMore {
		nextCursor = ""
	}
	return &ListResult{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// Flush blocks until every record enqueued before this call has been persisted.
// It is intended for explicit user actions (save / share) that must not return
// before the record is durable.
func (s *Service) Flush(ctx context.Context) error {
	for {
		if s.pending.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

// Close gracefully stops the background workers. Any records still queued are
// drained and persisted before Close returns.
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		close(s.ch)
		s.mu.Unlock()
	})
	s.wg.Wait()
	return nil
}

// enqueue hands a record to the queue. Under load (queue full) it falls back to
// a synchronous write. All paths keep s.pending accurate so Flush can wait.
func (s *Service) enqueue(rec *model.QueryRecord) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return s.store.Upsert(context.Background(), rec) // 关闭中：同步兜底
	}
	s.pending.Add(1)
	select {
	case s.ch <- rec:
		s.mu.Unlock()
		return nil
	default:
		// 队列满：同步兜底写入，绝不静默丢记录
		s.pending.Add(-1)
		s.mu.Unlock()
		return s.store.Upsert(context.Background(), rec)
	}
}

func (s *Service) worker() {
	defer s.wg.Done()
	for rec := range s.ch {
		if err := s.store.Upsert(context.Background(), rec); err != nil {
			slog.Error("queryrecord: async upsert failed",
				"query_id", rec.QueryID, "err", err)
		}
		s.pending.Add(-1)
	}
}

func toListItem(r *model.QueryRecord) ListItem {
	it := ListItem{
		QueryID:    r.QueryID,
		DatasetID:  r.DatasetID,
		SourceType: r.SourceType,
		CreatedAt:  r.CreatedAt,
		HitCount:   r.HitCount,
	}
	if r.ChartID.Valid {
		v := int(r.ChartID.Int32)
		it.ChartID = &v
	}
	if r.RowCount.Valid {
		v := int(r.RowCount.Int32)
		it.RowCount = &v
	}
	if r.DurationMs.Valid {
		v := int(r.DurationMs.Int32)
		it.DurationMs = &v
	}
	if r.LastAccessedAt.Valid {
		t := r.LastAccessedAt.Time
		it.LastAccessedAt = &t
	}
	if r.ExpiresAt.Valid {
		t := r.ExpiresAt.Time
		it.ExpiresAt = &t
	}
	return it
}

func nullInt32(p *int) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*p), Valid: true}
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// compile-time check that bunStore satisfies Store.
var _ Store = (*bunStore)(nil)
