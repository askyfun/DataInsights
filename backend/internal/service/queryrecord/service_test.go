package queryrecord

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"dataray/internal/model"
)

func newTestService(store Store, opts ...Option) *Service {
	svc := NewServiceWithStore(store, opts...)
	return svc
}

func sampleInput(spec string) RecordInput {
	return RecordInput{
		SpecJSON:   spec,
		DatasetID:  7,
		SourceType: "build",
	}
}

// TestHashDedupSameSpecTwice 同一 spec 落库两次 → 只有 1 行，hit_count == 2。
func TestHashDedupSameSpecTwice(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem)
	defer svc.Close()
	ctx := context.Background()

	spec := `{"v":1,"dimensions":[{"field":"region"}],"metrics":[{"field":"sales","agg":"sum"}]}`
	if err := svc.Record(ctx, sampleInput(spec)); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	if err := svc.Record(ctx, sampleInput(spec)); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected exactly 1 row after dedup, got %d", len(res.Items))
	}
	if res.Items[0].HitCount != 2 {
		t.Fatalf("expected hit_count==2, got %d", res.Items[0].HitCount)
	}
}

// TestHashDedupCanonicalization 仅键序不同的 spec 应被规范化为同一 hash → 去重为 1 行。
func TestHashDedupCanonicalization(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem)
	defer svc.Close()
	ctx := context.Background()

	a := `{"v":1,"metrics":[{"agg":"sum","field":"sales"}],"dimensions":[{"field":"region"}]}`
	b := `{"dimensions":[{"field":"region"}],"v":1,"metrics":[{"field":"sales","agg":"sum"}]}`

	if err := svc.Record(ctx, sampleInput(a)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(ctx, sampleInput(b)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("canonicalized specs should dedup to 1 row, got %d", len(res.Items))
	}
}

// TestConcurrentUpsertSameHash 并发写入同一 hash → 幂等、无重复行、最后写入生效（hit_count 累加）。
func TestConcurrentUpsertSameHash(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem, WithWorkers(4), WithQueueCapacity(256))
	defer svc.Close()
	ctx := context.Background()

	spec := `{"v":1,"dimensions":[],"metrics":[{"field":"x","agg":"count"}]}`
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := svc.Record(ctx, sampleInput(spec)); err != nil {
				t.Errorf("record: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := svc.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("concurrent same-hash writes must produce 1 row, got %d", len(res.Items))
	}
	if res.Items[0].HitCount != n {
		t.Fatalf("expected hit_count==%d, got %d", n, res.Items[0].HitCount)
	}
}

// countingStore 记录 Upsert 调用次数，并人为放慢以在突发期间填满队列、触发同步兜底。
type countingStore struct {
	*memStore
	mu    sync.Mutex
	calls int
}

func (c *countingStore) Upsert(ctx context.Context, rec *model.QueryRecord) error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	return c.memStore.Upsert(ctx, rec)
}

// TestQueueFullSyncFallbackNoLoss 队列满时同步兜底写入，且绝不静默丢记录。
func TestQueueFullSyncFallbackNoLoss(t *testing.T) {
	cs := &countingStore{memStore: newMemStore()}
	// 容量 1 + 单 worker + 慢存储：突发期间队列必然被填满，多余写入走同步兜底。
	svc := newTestService(cs, WithQueueCapacity(1), WithWorkers(1))
	defer svc.Close()
	ctx := context.Background()

	spec := `{"v":1,"dimensions":[{"field":"d"}],"metrics":[{"field":"m","agg":"avg"}]}`
	const n = 10
	for i := 0; i < n; i++ {
		if err := svc.Record(ctx, sampleInput(spec)); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	cs.mu.Lock()
	calls := cs.calls
	cs.mu.Unlock()
	if calls != n {
		t.Fatalf("every enqueued record must result in exactly one Upsert; got %d, want %d", calls, n)
	}

	res, err := svc.List(ctx, ListOptions{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 deduped row (no loss), got %d", len(res.Items))
	}
	if res.Items[0].HitCount != n {
		t.Fatalf("expected hit_count==%d (no loss), got %d", n, res.Items[0].HitCount)
	}
}

// TestAsyncFlushVisible 写入后经 Flush 可被 GetByID 取到；且 GetByID 只刷新访问计数，不刷新 expires_at。
func TestAsyncFlushVisible(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem)
	defer svc.Close()
	ctx := context.Background()

	spec := `{"v":1,"dimensions":[{"field":"a"}],"metrics":[]}`
	if err := svc.Record(ctx, sampleInput(spec)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("record should be visible after flush: %v items=%d", err, len(res.Items))
	}
	id := res.Items[0].QueryID
	expiresBefore := res.Items[0].ExpiresAt

	rec, err := svc.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if rec.SpecJSON == "" {
		t.Fatal("GetByID should return the persisted spec_json")
	}
	if !rec.ExpiresAt.Valid || expiresBefore == nil || !rec.ExpiresAt.Time.Equal(*expiresBefore) {
		t.Fatalf("GetByID must NOT refresh expires_at: before=%v after=%v", expiresBefore, rec.ExpiresAt)
	}
	// GetByID 返回的是 touch 之前的值（hit=1），touch 之后 store 内应为 2。
	if rec.HitCount != 1 {
		t.Fatalf("GetByID returns pre-touch hit_count=1, got %d", rec.HitCount)
	}
	res2, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Items) != 1 || res2.Items[0].HitCount != 2 {
		t.Fatalf("GetByID must have incremented hit_count to 2, got %+v", res2.Items)
	}
}

// TestExpiredGetByIDStillWorks 过期记录仍可被 GetByID 取到并完整还原（永不物理删除）；默认 List 不含它。
func TestExpiredGetByIDStillWorks(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem)
	defer svc.Close()
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	expired := &model.QueryRecord{
		QueryID:   "expired-id",
		SpecJSON:  `{"v":1,"dimensions":[{"field":"x"}],"metrics":[]}`,
		SpecHash:  "expired-hash",
		DatasetID: 3,
		CreatedAt: past,
		HitCount:  5,
		ExpiresAt: sql.NullTime{Time: past.Add(-time.Minute), Valid: true}, // 已过期
	}
	if err := mem.Upsert(ctx, expired); err != nil {
		t.Fatal(err)
	}

	// 直链打开：仍可取、完整还原
	rec, err := svc.GetByID(ctx, "expired-id")
	if err != nil {
		t.Fatalf("expired record must still be retrievable via GetByID: %v", err)
	}
	if !svc.IsExpired(rec) {
		t.Fatal("IsExpired should report true for an expired record")
	}
	if rec.SpecJSON == "" {
		t.Fatal("expired record must restore its spec_json fully")
	}

	// 默认历史列表：不含过期记录
	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.QueryID == "expired-id" {
			t.Fatal("default List must exclude expired records")
		}
	}

	// 显式包含过期：可取到
	res2, err := svc.List(ctx, ListOptions{Limit: 10, IncludeExpired: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range res2.Items {
		if it.QueryID == "expired-id" {
			found = true
		}
	}
	if !found {
		t.Fatal("IncludeExpired list must surface the expired record")
	}
}

// TestSpecTooLargeRejected spec_json > 32KB 被拒存、返回可识别错误、且未写入。
func TestSpecTooLargeRejected(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem, WithMaxSpecBytes(32*1024))
	defer svc.Close()
	ctx := context.Background()

	// 构造 > 32KB 的合法 JSON（一个大字符串字段），规范后仍超上限。
	var b strings.Builder
	b.WriteString(`{"v":1,"padding":"`)
	for i := 0; i < 5000; i++ {
		b.WriteString("abcdefghij") // 每个 10 字节 → 约 50KB
	}
	b.WriteString(`"}`)
	big := b.String()

	if err := svc.Record(ctx, sampleInput(string(big))); err == nil {
		t.Fatal("expected ErrSpecTooLarge, got nil")
	} else if !errors.Is(err, ErrSpecTooLarge) {
		t.Fatalf("expected ErrSpecTooLarge, got %v", err)
	}

	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := svc.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("oversized spec must NOT be persisted, got %d rows", len(res.Items))
	}
}

// TestKeysetPaginationNoOverlapNoGap keyset 分页翻页不重不漏（造 >2 页数据验证）。
func TestKeysetPaginationNoOverlapNoGap(t *testing.T) {
	mem := newMemStore()
	svc := newTestService(mem)
	defer svc.Close()
	ctx := context.Background()

	const total = 12
	base := time.Now().Add(-time.Hour)
	for i := 0; i < total; i++ {
		rec := &model.QueryRecord{
			QueryID:   fmt.Sprintf("id-%d", i),
			SpecJSON:  fmt.Sprintf(`{"v":1,"n":%d}`, i), // 每个唯一 → 不触发去重
			SpecHash:  fmt.Sprintf("hash-%d", i),
			DatasetID: 1,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
			HitCount:  1,
		}
		if err := mem.Upsert(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}

	const pageSize = 5
	var seen []string
	cursor := ""
	pages := 0
	for {
		res, err := svc.List(ctx, ListOptions{Limit: pageSize, Cursor: cursor})
		if err != nil {
			t.Fatalf("list page %d: %v", pages, err)
		}
		for _, it := range res.Items {
			seen = append(seen, it.QueryID)
		}
		pages++
		if !res.HasMore {
			break
		}
		cursor = res.NextCursor
		if pages > total {
			t.Fatalf("pagination did not terminate")
		}
	}

	if len(seen) != total {
		t.Fatalf("expected %d rows across pages, got %d (%v)", total, len(seen), seen)
	}
	// 不重
	uniq := make(map[string]bool)
	for _, id := range seen {
		if uniq[id] {
			t.Fatalf("duplicate id across pages: %s", id)
		}
		uniq[id] = true
	}
	// 不漏 + 顺序：按 created_at 降序应为 id-11, id-10, ... id-0
	want := make([]string, total)
	for i := 0; i < total; i++ {
		want[i] = fmt.Sprintf("id-%d", total-1-i)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("page order wrong at %d: got %s want %s (full=%v)", i, seen[i], want[i], seen)
		}
	}
}

// TestCursorRoundTrip 游标编解码往返一致。
func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	c := encodeCursor(ts, "abc-123")
	gotTs, gotID, err := decodeCursor(c)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotTs.Equal(ts) {
		t.Fatalf("timestamp mismatch: %v != %v", gotTs, ts)
	}
	if gotID != "abc-123" {
		t.Fatalf("id mismatch: %q", gotID)
	}
	if _, _, err := decodeCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("expected decode error for garbage cursor")
	}
}
