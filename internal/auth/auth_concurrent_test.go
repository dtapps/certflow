package auth

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	esql "entgo.io/ent/dialect/sql"
	_ "cnb.cool/dtapp/certflow/internal/sqlite"
)

func newConcurrentTestService(t *testing.T) *AuthService {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:ent?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}

	drv := esql.OpenDB("sqlite3", db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("Schema.Create: %v", err)
	}

	return NewAuthService(client)
}

// TestConcurrentPasswordSet 并发设置密码
func TestConcurrentPasswordSet(t *testing.T) {
	svc := newConcurrentTestService(t)
	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.SetPassword(fmt.Sprintf("password%d", idx))
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: SetPassword failed: %v", i, err)
		}
	}

	if !svc.IsPasswordSet() {
		t.Fatal("expected password to be set after concurrent writes")
	}
}

// TestConcurrentVerifyPassword 并发验证密码
func TestConcurrentVerifyPassword(t *testing.T) {
	svc := newConcurrentTestService(t)

	if err := svc.SetPassword("correct"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	results := make([]bool, goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				results[idx] = svc.VerifyPassword("correct")
			} else {
				results[idx] = svc.VerifyPassword("wrong")
			}
		}(i)
	}

	wg.Wait()

	for i, ok := range results {
		if i%2 == 0 && !ok {
			t.Errorf("goroutine %d: expected correct password to verify", i)
		}
		if i%2 == 1 && ok {
			t.Errorf("goroutine %d: expected wrong password to fail", i)
		}
	}
}

// TestConcurrentReadWrite 并发读写混合
func TestConcurrentReadWrite(t *testing.T) {
	svc := newConcurrentTestService(t)
	const goroutines = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			switch idx % 3 {
			case 0:
				errs[idx] = svc.SetPassword(fmt.Sprintf("pass%03d", idx))
			case 1:
				svc.VerifyPassword("pass000")
				svc.IsPasswordSet()
				svc.GetActiveMethod()
			case 2:
				svc.GetAvailableMethods()
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}
