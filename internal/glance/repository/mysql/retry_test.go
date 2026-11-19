package mysql

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	driver "github.com/go-sql-driver/mysql"
)

func TestLockRetryRerunsDeadlockVictimsOnly(t *testing.T) {
	deadlock := fmt.Errorf("update: %w", &driver.MySQLError{Number: 1213, Message: "Deadlock found"})
	calls := 0
	err := withLockRetry(context.Background(), func() error {
		if calls++; calls < 3 {
			return deadlock
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatal("deadlock victims must be re-run until they succeed", calls, err)
	}
	for _, final := range []error{repository.ErrConflict, repository.ErrCommitUncertain, &driver.MySQLError{Number: 1062}, errors.New("other")} {
		calls = 0
		if err = withLockRetry(context.Background(), func() error { calls++; return final }); !errors.Is(err, final) || calls != 1 {
			t.Fatal("only lock aborts are retried", final, calls)
		}
	}
	calls = 0
	timeout := &driver.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"}
	if err = withLockRetry(context.Background(), func() error { calls++; return timeout }); !errors.Is(err, timeout) || calls != deadlockRetries+1 {
		t.Fatal("retries are bounded and return the last error", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls = 0
	err = withLockRetry(ctx, func() error { calls++; cancel(); return deadlock })
	if !errors.Is(err, deadlock) || calls != 1 {
		t.Fatal("a cancelled caller is not retried", calls, err)
	}
}

// The public writers still map an exhausted lock abort to the domain conflict.
func TestExhaustedLockRetryStillReportsConflict(t *testing.T) {
	if !errors.Is(conflict(withLockRetry(context.Background(), func() error { return &driver.MySQLError{Number: 1213} })), repository.ErrConflict) {
		t.Fatal("exhausted deadlock retries must surface as a conflict")
	}
}
