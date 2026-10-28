package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
)

// Dedicated stateful protocol double. It models one row lock and transaction
// visibility for two reservations; it is NOT proof of real InnoDB scheduling.
type reservationDouble struct {
	mu                       sync.Mutex
	status, attempt          string
	rowLock                  chan struct{}
	firstLocked, secondBegun chan struct{}
	begun                    int
}
type reservationConnector struct{ d *reservationDouble }

func (c reservationConnector) Connect(context.Context) (driver.Conn, error) {
	return &reservationConn{d: c.d}, nil
}
func (c reservationConnector) Driver() driver.Driver { return reservationDriver{c.d} }

type reservationDriver struct{ d *reservationDouble }

func (d reservationDriver) Open(string) (driver.Conn, error) { return &reservationConn{d: d.d}, nil }

type reservationConn struct {
	d              *reservationDouble
	held           bool
	pendingAttempt string
	pendingSaving  bool
}

func (*reservationConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *reservationConn) Close() error            { c.unlock(); return nil }
func (*reservationConn) Begin() (driver.Tx, error) { return nil, errors.New("expected BeginTx") }
func (c *reservationConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.d.mu.Lock()
	c.d.begun++
	if c.d.begun == 2 {
		close(c.d.secondBegun)
	}
	c.d.mu.Unlock()
	return &reservationTx{c}, nil
}
func (c *reservationConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 1 || args[0].Value != catalogID {
		return nil, errors.New("missing bound image")
	}
	if q == lockDataImage {
		select {
		case <-c.d.rowLock:
			c.held = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.d.mu.Lock()
		first := c.d.begun == 1
		status := c.d.status
		c.d.mu.Unlock()
		if first {
			close(c.d.firstLocked)
			select {
			case <-c.d.secondBegun:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &scriptRows{width: 9, data: [][]driver.Value{{catalogID, status, false, nil, nil, nil, nil, nil, time.Unix(100, 0).UTC()}}}, nil
	}
	if q == lockDataOperation && c.held {
		c.d.mu.Lock()
		attempt := c.d.attempt
		c.d.mu.Unlock()
		var rows [][]driver.Value
		if attempt != "" {
			rows = [][]driver.Value{{catalogID, attempt, "reserved", nil, nil, nil, nil, nil, nil, nil, time.Unix(200, 0).UTC(), time.Unix(200, 0).UTC()}}
		}
		return &scriptRows{width: 12, data: rows}, nil
	}
	return nil, errors.New("unexpected reservation query")
}
func (c *reservationConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if !c.held {
		return nil, errors.New("write without row lock")
	}
	switch q {
	case reserveImageSQL:
		if len(args) != 2 || args[1].Value != catalogID {
			return nil, errors.New("unbound parent")
		}
		c.pendingSaving = true
	case reserveOperationSQL:
		if len(args) != 5 || args[0].Value != catalogID {
			return nil, errors.New("unbound intention")
		}
		c.pendingAttempt = args[1].Value.(string)
	default:
		return nil, errors.New("unexpected reservation SQL")
	}
	return driver.RowsAffected(1), nil
}
func (c *reservationConn) unlock() {
	if c.held {
		c.held = false
		c.d.rowLock <- struct{}{}
	}
}

type reservationTx struct{ c *reservationConn }

func (t *reservationTx) Commit() error {
	c := t.c
	c.d.mu.Lock()
	if c.pendingSaving {
		c.d.status = "saving"
		c.d.attempt = c.pendingAttempt
	}
	c.d.mu.Unlock()
	c.unlock()
	return nil
}
func (t *reservationTx) Rollback() error {
	t.c.pendingSaving = false
	t.c.pendingAttempt = ""
	t.c.unlock()
	return nil
}

var _ driver.QueryerContext = (*reservationConn)(nil)
var _ driver.ExecerContext = (*reservationConn)(nil)
var _ driver.ConnBeginTx = (*reservationConn)(nil)

func TestTwoConcurrentReservationsWithLockingDouble(t *testing.T) {
	d := &reservationDouble{status: "queued", rowLock: make(chan struct{}, 1), firstLocked: make(chan struct{}), secondBegun: make(chan struct{})}
	d.rowLock <- struct{}{}
	db := sql.OpenDB(reservationConnector{d})
	defer db.Close()
	db.SetMaxOpenConns(2)
	r := New(db, 2*time.Second)
	errs := make(chan error, 2)
	go func() { _, err := r.ReserveData(context.Background(), catalogID, attemptA, nil, dataNow); errs <- err }()
	select {
	case <-d.firstLocked:
	case <-time.After(time.Second):
		t.Fatal("first reservation did not acquire row lock")
	}
	go func() { _, err := r.ReserveData(context.Background(), catalogID, attemptB, nil, dataNow); errs <- err }()
	good, conflicts := 0, 0
	for n := 0; n < 2; n++ {
		select {
		case err := <-errs:
			if err == nil {
				good++
			} else if errors.Is(err, repository.ErrConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("reservation goroutine did not terminate")
		}
	}
	if good != 1 || conflicts != 1 {
		t.Fatal(good, conflicts)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != "saving" || d.attempt != attemptA {
		t.Fatal("wrong committed reservation", d.status, d.attempt)
	}
}
