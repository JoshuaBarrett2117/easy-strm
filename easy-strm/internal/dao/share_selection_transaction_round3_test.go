package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

var selectionProbeNumber atomic.Uint64
var selectionProbeStop = errors.New("isolated transaction probe")

type selectionProbe struct {
	gate        chan struct{}
	attempts    chan string
	syncHeld    chan struct{}
	releaseSync chan struct{}
	mu          sync.Mutex
	options     []driver.TxOptions
	first       []string
	revision    int64
	exported    int64
	observed    int64
	stop        bool
}

type selectionProbeDriver struct{ probe *selectionProbe }
type selectionProbeConn struct {
	probe       *selectionProbe
	transaction *selectionProbeTx
}
type selectionProbeTx struct {
	connection        *selectionProbeConn
	operation         string
	statements        int
	locked            bool
	selectionRevision int64
}
type selectionProbeRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (factory selectionProbeDriver) Open(string) (driver.Conn, error) {
	return &selectionProbeConn{probe: factory.probe}, nil
}
func (connection *selectionProbeConn) Prepare(string) (driver.Stmt, error) {
	return nil, selectionProbeStop
}
func (connection *selectionProbeConn) Close() error              { return nil }
func (connection *selectionProbeConn) Begin() (driver.Tx, error) { return nil, selectionProbeStop }
func (connection *selectionProbeConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	operation, _ := ctx.Value(selectionProbeOperation{}).(string)
	connection.transaction = &selectionProbeTx{connection: connection, operation: operation}
	connection.probe.mu.Lock()
	connection.probe.options = append(connection.probe.options, options)
	connection.probe.mu.Unlock()
	return connection.transaction, nil
}
func (connection *selectionProbeConn) statement(query string) {
	if connection.transaction.statements == 0 {
		connection.probe.mu.Lock()
		connection.probe.first = append(connection.probe.first, query)
		connection.probe.mu.Unlock()
	}
	connection.transaction.statements++
}
func (connection *selectionProbeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	connection.statement(query)
	probe, transaction := connection.probe, connection.transaction
	if probe.stop {
		return nil, selectionProbeStop
	}
	if query == shareSelectionLockSQL {
		probe.attempts <- transaction.operation
		select {
		case probe.gate <- struct{}{}:
			transaction.locked = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if transaction.operation == "sync" {
			close(probe.syncHeld)
			select {
			case <-probe.releaseSync:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	if strings.HasPrefix(query, "UPDATE t_share_media_selection SET stable_relative_path") {
		transaction.selectionRevision = args[2].Value.(int64)
	}
	return driver.RowsAffected(1), nil
}
func (connection *selectionProbeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	connection.statement(query)
	probe := connection.probe
	if probe.stop {
		return nil, selectionProbeStop
	}
	if !connection.transaction.locked {
		return nil, errors.New("read before advisory lock")
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.observed = probe.revision
	return &selectionProbeRows{columns: []string{"revision", "path"}, values: []driver.Value{probe.revision, "fixed.strm"}}, nil
}
func (transaction *selectionProbeTx) Commit() error {
	probe := transaction.connection.probe
	probe.mu.Lock()
	if transaction.operation == "sync" {
		probe.revision++
	}
	if transaction.selectionRevision == probe.revision {
		probe.exported = transaction.selectionRevision
	}
	probe.mu.Unlock()
	return transaction.Rollback()
}
func (transaction *selectionProbeTx) Rollback() error {
	if transaction.locked {
		<-transaction.connection.probe.gate
		transaction.locked = false
	}
	return nil
}
func (rows *selectionProbeRows) Columns() []string { return rows.columns }
func (rows *selectionProbeRows) Close() error      { return nil }
func (rows *selectionProbeRows) Next(values []driver.Value) error {
	if rows.done {
		return io.EOF
	}
	copy(values, rows.values)
	rows.done = true
	return nil
}

type selectionProbeOperation struct{}

func newSelectionProbe(t *testing.T, stop bool) (*sql.DB, *selectionProbe) {
	t.Helper()
	probe := &selectionProbe{gate: make(chan struct{}, 1), attempts: make(chan string, 2), syncHeld: make(chan struct{}), releaseSync: make(chan struct{}), revision: 2, stop: stop}
	name := fmt.Sprintf("selection-probe-%d", selectionProbeNumber.Add(1))
	sql.Register(name, selectionProbeDriver{probe: probe})
	database, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database, probe
}

func TestSelectionWriteIsolationAndFirstStatement(t *testing.T) {
	for _, operation := range []string{"sync", "change", "resolve", "save"} {
		t.Run(operation, func(t *testing.T) {
			database, probe := newSelectionProbe(t, true)
			ctx := context.Background()
			store := NewShareRecordDAO(database)
			var err error
			switch operation {
			case "sync":
				err = store.SyncSelections(ctx, "work")
			case "change":
				err = store.ChangeSelection(ctx, domain.ShareSelectionChange{})
			case "resolve":
				_, _, err = store.ResolveSelection(ctx, "work:0:0")
			case "save":
				connection, connectionErr := database.Conn(ctx)
				if connectionErr != nil {
					t.Fatal(connectionErr)
				}
				defer connection.Close()
				err = (&StrmExportDAO{Conn: connection}).SaveSelectionExport(ctx, ExportState{}, domain.ShareSelection{}, "fixed.strm")
			}
			if !errors.Is(err, selectionProbeStop) {
				t.Fatal(err)
			}
			if len(probe.options) != 1 || probe.options[0].Isolation != driver.IsolationLevel(sql.LevelReadCommitted) || probe.options[0].ReadOnly {
				t.Fatalf("explicit READ COMMITTED required: %+v", probe.options)
			}
			if len(probe.first) != 1 || probe.first[0] != shareSelectionLockSQL {
				t.Fatalf("first SQL: %v", probe.first)
			}
		})
	}
}

func TestSelectionReadIsolationRemainsReadOnlyRepeatableRead(t *testing.T) {
	for _, operation := range []string{"list", "detail"} {
		t.Run(operation, func(t *testing.T) {
			database, probe := newSelectionProbe(t, true)
			store := NewShareRecordDAO(database)
			if operation == "list" {
				_, _, _ = store.ListSelections(context.Background(), domain.ShareSelectionQuery{})
			} else {
				_, _ = store.SelectionDetail(context.Background(), "work:0:0")
			}
			if len(probe.options) != 1 || !probe.options[0].ReadOnly || probe.options[0].Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
				t.Fatalf("read isolation: %+v", probe.options)
			}
			if len(probe.first) != 1 || probe.first[0] == shareSelectionLockSQL {
				t.Fatalf("read advisory added: %v", probe.first)
			}
		})
	}
}

func TestMockSelectionSyncSerializesReceiptAndObservesCommittedRevision(t *testing.T) {
	database, probe := newSelectionProbe(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	syncDone, receiptDone := make(chan error, 1), make(chan error, 1)
	go func() {
		syncDone <- NewShareRecordDAO(database).SyncSelections(context.WithValue(ctx, selectionProbeOperation{}, "sync"), "work")
	}()
	select {
	case <-probe.syncHeld:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if attempt := <-probe.attempts; attempt != "sync" {
		t.Fatal(attempt)
	}
	go func() {
		connection, err := database.Conn(ctx)
		if err != nil {
			receiptDone <- err
			return
		}
		defer connection.Close()
		receiptDone <- (&StrmExportDAO{Conn: connection}).SaveSelectionExport(context.WithValue(ctx, selectionProbeOperation{}, "receipt"), ExportState{Owner: "share:default", Key: "work:0:0"}, domain.ShareSelection{ItemKey: "work:0:0", Revision: 2}, "fixed.strm")
	}()
	select {
	case attempt := <-probe.attempts:
		if attempt != "receipt" {
			t.Fatal(attempt)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	probe.mu.Lock()
	if probe.observed != 0 {
		t.Errorf("receipt read while sync owns lock: %d", probe.observed)
	}
	probe.mu.Unlock()
	select {
	case err := <-receiptDone:
		t.Fatalf("receipt escaped lock: %v", err)
	default:
	}
	close(probe.releaseSync)
	if err := <-syncDone; err != nil {
		t.Fatal(err)
	}
	if err := <-receiptDone; !errors.Is(err, ErrShareExportChanged) {
		t.Fatalf("stale receipt not rejected: %v", err)
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.revision != 3 || probe.observed != 3 || probe.exported != 0 {
		t.Fatalf("mock committed state: %+v", probe)
	}
	for _, options := range probe.options {
		if options.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
			t.Fatalf("snapshot isolation: %+v", options)
		}
	}
	t.Log("isolated advisory/commit model: receipt waits for SyncSelections and reads committed revision 3; stale revision 2 remains unacknowledged; NOT PostgreSQL integration")
}
