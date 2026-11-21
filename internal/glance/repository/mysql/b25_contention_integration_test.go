//go:build integration

package mysql

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/google/uuid"
)

// Distinct images written at the same time must never fail as conflicts.
// SELECT ... FOR UPDATE on a missing child row takes an InnoDB gap lock; two
// transactions that then insert into the same gap deadlock (1213). The campaign
// of 2026-11-19 hit it as a 409 on PUT with 16 upload workers. Official Glance
// retries deadlocks (oslo.db); Go must too. MUTATING opt-in, NEW EMPTY schema.
func TestB25MySQLConcurrentDistinctImages(t *testing.T) {
	if os.Getenv("GLANCE_GO_B25_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable B25 MySQL opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, _, official := IntegrationFixture(t, ctx)
	var err error
	r := IntegrationRepo(t, db, 5*time.Second, official)
	if err = r.PrepareStatements(ctx); err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 32, 4
	receipt := model.DataContent{Size: 6, Checksum: strings.Repeat("a", 32), SHA512: strings.Repeat("b", 128)}
	parallel := func(stage string, f func(int) error) {
		t.Helper()
		start, errs := make(chan struct{}), make([]error, workers)
		var wg sync.WaitGroup
		for n := 0; n < workers; n++ {
			wg.Add(1)
			go func(n int) { defer wg.Done(); <-start; errs[n] = f(n) }(n)
		}
		close(start)
		wg.Wait()
		for n, err := range errs {
			if err != nil {
				t.Fatalf("%s: worker %d of %d distinct images failed: %v", stage, n, workers, err)
			}
		}
	}
	for round := 0; round < rounds; round++ {
		ids, attempts := make([]string, workers), make([]string, workers)
		parallel("create", func(n int) error {
			i, err := model.New(model.CreateInput{Properties: map[string]string{"k": "v"}, Tags: []string{"t"}}, "contention-project", time.Now())
			if err != nil {
				return err
			}
			ids[n] = i.ID
			return r.Create(ctx, i)
		})
		parallel("patch", func(n int) error {
			_, err := r.Update(ctx, ids[n], func(i model.Image) (model.Image, error) {
				return model.ApplyPatch(i, []model.PatchOperation{{Op: "add", Key: "p" + uuid.NewString()[:4], Value: json.RawMessage(`"x"`)}}, time.Now())
			})
			return err
		})
		parallel("reserve", func(n int) error {
			attempts[n] = uuid.NewString()
			_, err := r.ReserveData(ctx, ids[n], attempts[n], nil, time.Now())
			return err
		})
		parallel("stage", func(n int) error { return r.StageData(ctx, ids[n], attempts[n], receipt, time.Now()) })
		parallel("publish", func(n int) error { return r.PublishData(ctx, ids[n], attempts[n], time.Now()) })
		parallel("commit", func(n int) error { return r.CommitData(ctx, ids[n], attempts[n], time.Now()) })
		parallel("delete", func(n int) error {
			_, err := r.TombstoneData(ctx, ids[n], &attempts[n], time.Now(), func(model.Image) error { return nil })
			return err
		})
	}
}
