package mysql

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// deadlockRetries bounds how often a whole write transaction is re-run after
// MySQL aborted it as a deadlock victim (1213) or a lock wait timeout (1205).
// Official Glance does the same through oslo.db (retry_on_deadlock). Under
// REPEATABLE READ, SELECT ... FOR UPDATE on a missing child row takes a gap
// lock, and concurrent writers of DISTINCT images that insert into the same gap
// deadlock: with 16 upload workers that surfaced as a 409 on PUT. Re-running is
// safe because nothing outside the transaction runs inside it, and a failed
// commit is still reported as uncertain, never retried.
const deadlockRetries = 20

func retryableLock(err error) bool {
	var my *driver.MySQLError
	return errors.As(err, &my) && (my.Number == 1213 || my.Number == 1205)
}

// withLockRetry runs attempt until it succeeds, fails for another reason, the
// retry budget is spent or ctx ends; it returns the last attempt's error. The
// jittered backoff (1–128 ms) desynchronizes the contending writers.
func withLockRetry(ctx context.Context, attempt func() error) error {
	for n := 0; ; n++ {
		err := attempt()
		if err == nil || !retryableLock(err) || n >= deadlockRetries {
			return err
		}
		wait := time.Duration(1<<min(n, 6)) * time.Millisecond
		timer := time.NewTimer(wait + rand.N(wait))
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
}
