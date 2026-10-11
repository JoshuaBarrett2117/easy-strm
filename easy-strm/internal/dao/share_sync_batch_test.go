package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

type syncAttemptToken struct {
	prefix string
	value  string
}

func (token *syncAttemptToken) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok || !strings.HasPrefix(text, token.prefix+":") || text == token.prefix {
		return false
	}
	if token.value == "" {
		token.value = text
	}
	return text == token.value
}

func expectSyncSession(mock sqlmock.Sqlmock, version int) {
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(34984,\$1\)`).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT version FROM t_share_record WHERE id=\\$1").WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(version))
}

func expectSyncRelease(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT pg_advisory_unlock\(34984,\$1\)`).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))
}

func expectSyncBatchLock(mock sqlmock.Sqlmock, version int) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM t_share_record WHERE id=\$1 AND version=\$2 FOR UPDATE`).WithArgs(1, version).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
}

func syncFixtureFiles(count int) []domain.ShareMedia {
	files := make([]domain.ShareMedia, count)
	for index := range files {
		files[index] = domain.ShareMedia{RemoteFileID: fmt.Sprint(index), FileName: fmt.Sprintf("file-%d.mkv", index)}
	}
	return files
}

func TestSyncFailedMiddleBatchNeverFinalizes(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	expectSyncSession(mock, 2)
	expectSyncBatchLock(mock, 2)
	mock.ExpectQuery("WITH input AS").WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(512, "{}"))
	mock.ExpectCommit()
	expectSyncBatchLock(mock, 2)
	failure := errors.New("middle batch failed")
	mock.ExpectQuery("WITH input AS").WillReturnError(failure)
	mock.ExpectRollback()
	expectSyncRelease(mock)
	count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "retry", syncFixtureFiles(1025))
	if count != 0 || !errors.Is(err, failure) {
		test.Fatalf("count=%d err=%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncConcurrentSessionDoesNotWrite(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(34984,\$1\)`).WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "same-token", syncFixtureFiles(1))
	if count != 0 || err == nil || db.Stats().InUse != 0 {
		test.Fatalf("count=%d err=%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func expectSyncWrite(mock sqlmock.Sqlmock, attempt *syncAttemptToken, files []domain.ShareMedia, orphans string) {
	input, _ := prepareShareSyncInput(context.Background(), files)
	args := shareSyncBatchArgs(1, "placeholder", input)
	values := make([]driver.Value, len(args))
	for index := range args {
		values[index] = args[index]
	}
	values[1] = attempt
	mock.ExpectQuery(regexp.QuoteMeta(shareSyncBatchSQL)).WithArgs(values...).
		WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(len(input), orphans))
}

func TestSyncBoundedBatchesCommitBeforeAtomicFinalization(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	files := syncFixtureFiles(1025)
	files = append(files, files[0])
	attempt := &syncAttemptToken{prefix: "complete"}
	expectSyncSession(mock, 2)
	for start := 0; start < len(files); start += shareSyncBatchSize {
		end := min(start+shareSyncBatchSize, len(files))
		expectSyncBatchLock(mock, 2)
		expectSyncWrite(mock, attempt, files[start:end], "{11,12}")
		mock.ExpectExec(regexp.QuoteMeta(shareSyncOrphanSQL)).WithArgs(pq.Array([]int64{11, 12})).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	expectSyncBatchLock(mock, 2)
	mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(1, attempt).WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectCommit()
	expectSyncRelease(mock)
	count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "complete", files)
	if err != nil || count != len(files) {
		test.Fatalf("count=%d err=%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncRetryAfterFailedMiddleBatchUsesIndependentSeenSet(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	files := syncFixtureFiles(513)
	failed := &syncAttemptToken{prefix: "same"}
	expectSyncSession(mock, 2)
	expectSyncBatchLock(mock, 2)
	expectSyncWrite(mock, failed, files[:512], "{}")
	mock.ExpectCommit()
	expectSyncBatchLock(mock, 2)
	mock.ExpectQuery("WITH input AS").WillReturnError(errors.New("failed remainder"))
	mock.ExpectRollback()
	expectSyncRelease(mock)
	count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "same", files)
	if count != 0 || err == nil {
		test.Fatalf("count=%d err=%v", count, err)
	}
	retried := &syncAttemptToken{prefix: "same"}
	expectSyncSession(mock, 2)
	expectSyncBatchLock(mock, 2)
	expectSyncWrite(mock, retried, files[:1], "{}")
	mock.ExpectCommit()
	expectSyncBatchLock(mock, 2)
	mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(1, retried).WillReturnResult(sqlmock.NewResult(0, 511))
	mock.ExpectCommit()
	expectSyncRelease(mock)
	count, err = NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "same", files[:1])
	if count != 1 || err != nil || retried.value == failed.value {
		test.Fatalf("count=%d err=%v tokens=%q/%q", count, err, failed.value, retried.value)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncEmptyScanAndReusedTokenUseFreshAttempt(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	var previous string
	for index := 0; index < 3; index++ {
		attempt := &syncAttemptToken{prefix: "same"}
		expectSyncSession(mock, 2)
		expectSyncBatchLock(mock, 2)
		mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(1, attempt).WillReturnResult(sqlmock.NewResult(0, 10))
		if index == 0 {
			mock.ExpectCommit().WillReturnError(errors.New("commit outcome unknown"))
		} else {
			mock.ExpectCommit()
		}
		expectSyncRelease(mock)
		count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "same", nil)
		if count != 0 || (err != nil) != (index == 0) || attempt.value == previous {
			test.Fatalf("count=%d err=%v attempt=%q", count, err, attempt.value)
		}
		previous = attempt.value
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncRejectsChangedOrDeletedShareAfterCommittedBatch(test *testing.T) {
	for _, phase := range []string{"middle_batch", "finalization"} {
		test.Run(phase, func(test *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer db.Close()
			files := syncFixtureFiles(shareSyncBatchSize)
			if phase == "middle_batch" {
				files = append(files, domain.ShareMedia{RemoteFileID: "next", FileName: "next.mkv"})
			}
			expectSyncSession(mock, 2)
			expectSyncBatchLock(mock, 2)
			expectSyncWrite(mock, &syncAttemptToken{prefix: "stale"}, files[:shareSyncBatchSize], "{}")
			mock.ExpectCommit()
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT id FROM t_share_record WHERE id=\$1 AND version=\$2 FOR UPDATE`).WithArgs(1, 2).WillReturnError(sql.ErrNoRows)
			mock.ExpectRollback()
			expectSyncRelease(mock)
			count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "stale", files)
			if count != 0 || !errors.Is(err, sql.ErrNoRows) {
				test.Fatalf("count=%d err=%v", count, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestSyncBatchAndFinalizationFailuresRollback(test *testing.T) {
	for _, phase := range []string{"begin", "batch", "count", "orphan", "batch_commit", "final_update", "final_commit"} {
		test.Run(phase, func(test *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer db.Close()
			failure := errors.New(phase + " failure")
			expectSyncSession(mock, 2)
			if phase == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
			} else {
				expectSyncBatchLock(mock, 2)
				write := mock.ExpectQuery(regexp.QuoteMeta(shareSyncBatchSQL))
				if phase == "batch" {
					write.WillReturnError(failure)
				} else if phase == "count" {
					write.WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(0, "{}"))
				} else {
					write.WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(1, "{11}"))
					cleanup := mock.ExpectExec(regexp.QuoteMeta(shareSyncOrphanSQL)).WithArgs(pq.Array([]int64{11}))
					if phase == "orphan" {
						cleanup.WillReturnError(failure)
					} else {
						cleanup.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				switch phase {
				case "batch", "count", "orphan":
					mock.ExpectRollback()
				case "batch_commit":
					mock.ExpectCommit().WillReturnError(failure)
				default:
					mock.ExpectCommit()
					expectSyncBatchLock(mock, 2)
					final := mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE")
					if phase == "final_update" {
						final.WillReturnError(failure)
						mock.ExpectRollback()
					} else {
						final.WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectCommit().WillReturnError(failure)
					}
				}
			}
			expectSyncRelease(mock)
			count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "failed", syncFixtureFiles(1))
			if count != 0 || err == nil || phase != "count" && !errors.Is(err, failure) {
				test.Fatalf("count=%d err=%v", count, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

type cancelSyncQuery struct {
	cancel context.CancelFunc
}

func (query cancelSyncQuery) Match(driver.Value) bool {
	query.cancel()
	return true
}

func TestSyncCancellationDuringBatchOrFinalization(test *testing.T) {
	for _, phase := range []string{"batch", "finalization"} {
		test.Run(phase, func(test *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer db.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expectSyncSession(mock, 2)
			expectSyncBatchLock(mock, 2)
			files := syncFixtureFiles(513)
			if phase == "finalization" {
				files = syncFixtureFiles(1)
			}
			expectSyncWrite(mock, &syncAttemptToken{prefix: "cancelled"}, files[:min(shareSyncBatchSize, len(files))], "{}")
			mock.ExpectCommit()
			expectSyncBatchLock(mock, 2)
			if phase == "batch" {
				mock.ExpectQuery("WITH input AS").WithArgs(cancelSyncQuery{cancel}, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(1, "{}"))
			} else {
				mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(cancelSyncQuery{cancel}, sqlmock.AnyArg()).
					WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectRollback()
			mock.ExpectClose()
			count, err := NewShareRecordDAO(db).SyncFiles(ctx, 1, "cancelled", files)
			if count != 0 || !errors.Is(err, context.Canceled) || ctx.Err() != context.Canceled {
				test.Fatalf("count=%d err=%v context=%v", count, err, ctx.Err())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestSyncPreCancelledContextNeverAcquiresConnection(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, files := range [][]domain.ShareMedia{nil, syncFixtureFiles(513)} {
		count, err := NewShareRecordDAO(db).SyncFiles(ctx, 1, "cancelled", files)
		if count != 0 || !errors.Is(err, context.Canceled) {
			test.Fatalf("count=%d err=%v", count, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncSessionUnlockFailureDiscardsPhysicalConnection(test *testing.T) {
	for _, acknowledged := range []bool{false, true} {
		test.Run(fmt.Sprint(acknowledged), func(test *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer db.Close()
			expectSyncSession(mock, 2)
			expectSyncBatchLock(mock, 2)
			mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			unlock := mock.ExpectQuery(`SELECT pg_advisory_unlock\(34984,\$1\)`).WithArgs(1)
			if acknowledged {
				unlock.WillReturnError(errors.New("unlock connection lost"))
			} else {
				unlock.WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(false))
			}
			mock.ExpectClose()
			count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "cleanup", nil)
			if count != 0 || err == nil || db.Stats().OpenConnections != 0 {
				test.Fatalf("count=%d err=%v stats=%+v", count, err, db.Stats())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestSyncSessionAcquisitionFailureDiscardsUncertainLock(test *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	defer db.Close()
	failure := errors.New("lock result lost")
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(34984,\$1\)`).WillReturnError(failure)
	mock.ExpectClose()
	count, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 1, "failed", nil)
	if count != 0 || !errors.Is(err, failure) || db.Stats().OpenConnections != 0 {
		test.Fatalf("count=%d err=%v stats=%+v", count, err, db.Stats())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestSyncDuplicateReductionPreservesSequentialInvalidation(test *testing.T) {
	inputs := []domain.ShareMedia{
		{RemoteFileID: "remote", FileName: "a.mkv", SHA1: "A"},
		{RemoteFileID: "remote", FileName: "b.mkv", SHA1: "B"},
		{RemoteFileID: "remote", FileName: "a.mkv", SHA1: "A"},
		{FileName: "a.mkv", SHA1: "C"},
		{RemoteFileID: "other", FileName: "a.mkv", SHA1: "D"},
		{FileName: "a.mkv", SHA1: ""},
	}
	input, err := prepareShareSyncInput(context.Background(), inputs)
	if err != nil || len(input) != 3 {
		test.Fatalf("input=%+v err=%v", input, err)
	}
	if input[0].versionBumps != 2 || !input[0].internalChanged || input[0].firstSHA != "A" || input[0].file.SHA1 != "A" ||
		input[1].versionBumps != 0 || input[1].internalChanged || input[1].firstSHA != "C" || input[1].file.SHA1 != "" {
		test.Fatalf("input=%+v", input)
	}
	if !reflect.DeepEqual(inputs[0], domain.ShareMedia{RemoteFileID: "remote", FileName: "a.mkv", SHA1: "A"}) {
		test.Fatal("input was mutated")
	}
	for _, oldHash := range []string{"", "A", "B"} {
		for _, oldName := range []string{"a.mkv", "old.mkv"} {
			version, reset := int64(0), false
			name, hash := oldName, oldHash
			for _, file := range inputs[:3] {
				changed := hash != "" && file.SHA1 != "" && hash != file.SHA1
				reset = reset || changed
				if changed || name != file.FileName {
					version++
				}
				name, hash = file.FileName, file.SHA1
			}
			firstChanged := oldHash != "" && input[0].firstSHA != "" && oldHash != input[0].firstSHA
			expectedVersion := input[0].versionBumps
			if firstChanged || oldName != input[0].firstName {
				expectedVersion++
			}
			if version != expectedVersion || reset != (input[0].internalChanged || firstChanged) {
				test.Fatalf("hash=%q name=%q version=%d/%d reset=%v", oldHash, oldName, version, expectedVersion, reset)
			}
		}
	}
}

func TestSyncDuplicateHashAndPathTransitionsMatchSequentialRules(test *testing.T) {
	variants := []domain.ShareMedia{}
	for _, name := range []string{"a.mkv", "b.mkv"} {
		for _, hash := range []string{"", "A", "B"} {
			variants = append(variants, domain.ShareMedia{RemoteFileID: "same", FileName: name, SHA1: hash})
		}
	}
	for _, first := range variants {
		for _, second := range variants {
			for _, third := range variants {
				files := []domain.ShareMedia{first, second, third}
				input, err := prepareShareSyncInput(context.Background(), files)
				if err != nil || len(input) != 1 {
					test.Fatalf("input=%+v err=%v", input, err)
				}
				for _, baseline := range variants {
					version, reset := int64(0), false
					old := baseline
					for _, next := range files {
						changed := old.SHA1 != "" && next.SHA1 != "" && old.SHA1 != next.SHA1
						if changed || old.FileName != next.FileName {
							version++
						}
						reset = reset || changed
						old = next
					}
					if !reflect.DeepEqual(old, input[0].file) {
						test.Fatal("last file data lost")
					}
					firstChanged := baseline.SHA1 != "" && first.SHA1 != "" && baseline.SHA1 != first.SHA1
					expectedVersion := input[0].versionBumps
					if firstChanged || baseline.FileName != first.FileName {
						expectedVersion++
					}
					if version != expectedVersion || reset != (input[0].internalChanged || firstChanged) {
						test.Fatalf("baseline=%+v files=%+v version=%d/%d reset=%v input=%+v", baseline, files, version, expectedVersion, reset, input)
					}
				}
			}
		}
	}
}

func TestSyncSQLAndTransactionShapeGuards(test *testing.T) {
	for _, clause := range []string{
		"FROM unnest($3::text[]", "input.remote_id<>'' AND f.file_id=input.remote_id",
		"input.remote_id='' AND f.file_id=''", "ORDER BY f.id LIMIT 1 FOR UPDATE OF f",
		"old_sha<>'' AND first_sha<>'' AND old_sha<>first_sha", "old_name IS DISTINCT FROM first_name",
		"status=CASE WHEN c.content_changed THEN 'pending' ELSE f.status END",
		"media_id=CASE WHEN c.content_changed THEN NULL ELSE f.media_id END",
		"result=CASE WHEN c.content_changed THEN NULL ELSE f.result END",
		"error=CASE WHEN c.content_changed THEN '' ELSE f.error END",
		"WHERE e.file_id=c.id AND u.id=c.id AND c.content_changed", "1+version_bumps",
		"version=f.version+c.existing_version_bumps", "AND (c.payload_changed OR c.existing_version_bumps>0 OR c.content_changed)",
	} {
		if !strings.Contains(shareSyncBatchSQL, clause) {
			test.Fatalf("missing SQL contract: %s", clause)
		}
	}
	if strings.Contains(shareSyncBatchSQL, "metadata_source=") || !strings.Contains(shareSyncOrphanSQL, "m.id=ANY($1::integer[])") {
		test.Fatal("manual source overwritten or unbounded orphan cleanup")
	}
	data, err := os.ReadFile("share_sync.go")
	if err != nil {
		test.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "share_sync.go", data, 0)
	if err != nil {
		test.Fatal(err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if loop, ok := node.(*ast.ForStmt); ok && function.Name.Name == "SyncFiles" {
				if !strings.Contains(string(data[loop.Pos()-1:loop.End()-1]), "start += shareSyncBatchSize") {
					test.Fatal("scan loop is not bounded")
				}
			}
			var loopBody *ast.BlockStmt
			switch loop := node.(type) {
			case *ast.RangeStmt:
				loopBody = loop.Body
			case *ast.ForStmt:
				loopBody = loop.Body
			default:
				return true
			}
			ast.Inspect(loopBody, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && (selector.Sel.Name == "QueryContext" || selector.Sel.Name == "QueryRowContext" || selector.Sel.Name == "ExecContext" || selector.Sel.Name == "BeginTx") {
					test.Fatalf("per-file database access in %s", function.Name.Name)
				}
				return true
			})
			return true
		})
		if function.Name.Name == "SyncFiles" {
			body := string(data[function.Body.Pos()-1 : function.Body.End()-1])
			if strings.Contains(body, "BeginTx") || strings.Contains(body, "tx.Commit") || !strings.Contains(body, "finalizeShareSync") {
				test.Fatal("scan-wide transaction or missing atomic finalization")
			}
		}
	}
}
