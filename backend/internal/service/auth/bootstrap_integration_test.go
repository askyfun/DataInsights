package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"data-insights/internal/database"
	"data-insights/internal/router"
)

// TestRegister_ConcurrentBootstrapOnlyOneWins exercises the advisory lock on a
// real PostgreSQL scratch database. It is skipped when TEST_DATABASE_URL is not
// set (CI has no database service), so run it locally against a throwaway DB:
//
//	TEST_DATABASE_URL=postgres://postgres:postgres@host:5432/scratch?sslmode=disable \
//	  go test -race ./internal/service/auth -run ConcurrentBootstrap
func TestRegister_ConcurrentBootstrapOnlyOneWins(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		"TRUNCATE bi_token, bi_user, bi_setting RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate scratch tables: %v", err)
	}

	svc := NewService(db, AdminConfig{})
	raw, generated, err := svc.EnsureBootstrap(context.Background())
	if err != nil || !generated {
		t.Fatalf("EnsureBootstrap: generated=%v err=%v", generated, err)
	}

	usernames := []string{"alice", "bob"}
	errs := make([]error, len(usernames))
	var wg sync.WaitGroup
	for i, name := range usernames {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			_, errs[i] = svc.Register(context.Background(), name, "pw-"+name, raw, "127.0.0.1", "test")
		}(i, name)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		var be router.BusinessError
		if !errors.As(err, &be) || be.Code != 20200 {
			t.Fatalf("loser must fail with 20200, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one concurrent bootstrap must win, got %d", successes)
	}

	needs, _, err := svc.BootstrapStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("bootstrap must be closed after the winner created the admin")
	}
	if _, ok, err := settingGet(context.Background(), db, settingBootstrapHash); err != nil || ok {
		t.Fatalf("bootstrap token must be consumed, found=%v err=%v", ok, err)
	}
}
