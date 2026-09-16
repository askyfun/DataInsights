//go:build integration

package queryrecord

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"dataray/internal/database"
	"dataray/internal/model"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// openTestDB spins up a real PostgreSQL via the embedded migrations. It requires
// TEST_DATABASE_URL and is skipped otherwise.
func openTestDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { db.Close() })
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return db
}

func TestBunStoreUpsertDedupIntegration(t *testing.T) {
	db := openTestDB(t)
	store := newBunStore(db, defaultListWindow)
	ctx := context.Background()

	rec := func() *model.QueryRecord {
		return &model.QueryRecord{
			QueryID:   "q-integration-1",
			SpecJSON:  `{"v":1,"dimensions":[{"field":"x"}],"metrics":[]}`,
			SpecHash:  "hash-integration-1",
			DatasetID: 1,
			CreatedAt: time.Now(),
			HitCount:  1,
			ExpiresAt: sql.NullTime{Time: time.Now().Add(90 * 24 * time.Hour), Valid: true},
		}
	}
	if err := store.Upsert(ctx, rec()); err != nil {
		t.Fatal(err)
	}
	// 同 hash 第二次 → 更新 hit_count，不新增行
	if err := store.Upsert(ctx, rec()); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByID(ctx, "q-integration-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.HitCount != 2 {
		t.Fatalf("expected hit_count==2 after dedup upsert, got %d", got.HitCount)
	}
}

func TestBunStoreListKeysetIntegration(t *testing.T) {
	db := openTestDB(t)
	store := newBunStore(db, defaultListWindow)
	ctx := context.Background()

	base := time.Now().Add(-time.Hour)
	for i := 0; i < 12; i++ {
		r := &model.QueryRecord{
			QueryID:   "qk-" + string(rune('a'+i)),
			SpecJSON:  `{"v":1,"n":` + string(rune('0'+i)) + `}`,
			SpecHash:  "hk-" + string(rune('a'+i)),
			DatasetID: 1,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
			HitCount:  1,
		}
		if err := store.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	cursor := ""
	seen := 0
	for {
		recs, err := store.List(ctx, ListOptions{Limit: 5, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		seen += len(recs)
		if len(recs) < 5 {
			break
		}
		cursor = encodeCursor(recs[len(recs)-1].CreatedAt, recs[len(recs)-1].QueryID)
		if seen > 12 {
			t.Fatal("pagination did not terminate")
		}
	}
	if seen != 12 {
		t.Fatalf("expected 12 rows via keyset, got %d", seen)
	}
}
