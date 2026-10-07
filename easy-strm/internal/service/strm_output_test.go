package service

import (
	"context"
	"easy-strm/internal/dao"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitStrmOutputRetriesBusyDirectory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root, err := NormalizeStrmOutputPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chain := []string{}
	for p := root; ; p = filepath.Dir(p) {
		chain = append([]string{p}, chain...)
		if filepath.Dir(p) == p {
			break
		}
	}
	for _, busy := range []bool{true, false} {
		for i, p := range chain {
			query := "SELECT pg_try_advisory_lock_shared"

			mock.ExpectQuery(query).WithArgs(p).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(!busy || i != len(chain)-1))
		}
		if busy {
			mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}
	mock.ExpectQuery("SELECT strm_config_id,local_strm_path").WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
	mock.ExpectQuery("SELECT output_path FROM t_strm_export_state").WillReturnRows(sqlmock.NewRows([]string{"path"}))
	mock.ExpectQuery("SELECT export_key,last_seen_run_id").WithArgs("cloud115:1").WillReturnRows(sqlmock.NewRows([]string{"key", "run"}))
	mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := WaitStrmOutput(ctx, db, root, "cloud115:1", "run", nil)
	if err != nil {
		t.Fatal(err)
	}
	output.Store.Close()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitStrmOutputCancellationAndDatabaseFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	if _, err := WaitStrmOutput(context.Background(), db, root, "owner", "run", func() bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消无效: %v", err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := WaitStrmOutput(ctx, db, root, "owner", "run", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时无效: %v", err)
	}
	failure := errors.New("database unavailable")
	mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnError(failure)
	mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if _, err := WaitStrmOutput(context.Background(), db, root, "owner", "run", nil); !errors.Is(err, failure) {
		t.Fatalf("数据库错误必须直接返回: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStrmOutputPreservesUnchangedAndForeignFiles(t *testing.T) {
	for _, scenario := range []string{"unchanged", "foreign", "unregistered", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			db, m, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			c, e := db.Conn(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			root := t.TempDir()
			p := filepath.Join(root, "episode.strm")
			content := "https://example.test/play\n"
			if scenario != "missing" {
				if e = os.WriteFile(p, []byte(content), 0644); e != nil {
					t.Fatal(e)
				}
			}
			before, _ := os.Stat(p)
			rows := sqlmock.NewRows([]string{"owner_key", "export_key"})
			if scenario != "unregistered" {
				owner := "share:default"
				if scenario == "foreign" {
					owner = "cloud115:1"
				}
				rows.AddRow(owner, "1:1:1")
			}
			m.ExpectQuery(`SELECT pg_try_advisory_lock\(`).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
			m.ExpectQuery("SELECT owner_key,export_key").WillReturnRows(rows)
			if scenario == "unchanged" || scenario == "missing" {
				m.ExpectExec("INSERT INTO t_strm_export_history VALUES").WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectBegin()
				m.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 0))
				m.ExpectExec("INSERT INTO t_strm_export_state").WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			}
			m.ExpectExec("SELECT pg_advisory_unlock\\(").WillReturnResult(sqlmock.NewResult(0, 0))
			s := &StrmOutput{Store: &dao.StrmExportDAO{Conn: c}, Root: root, Owner: "share:default", Run: "run", Paths: map[string]bool{}}
			changed, e := s.Write(context.Background(), "1:1:1", p, content, "", "")
			if scenario == "foreign" || scenario == "unregistered" {
				if e == nil {
					t.Fatal("冲突路径被允许写入")
				}
			} else if e != nil {
				t.Fatal(e)
			}
			after, statErr := os.Stat(p)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if scenario != "missing" && (!before.ModTime().Equal(after.ModTime()) || changed) {
				t.Fatal("原文件被重写")
			}
			if scenario == "missing" && !changed {
				t.Fatal("未补回丢失文件")
			}
			if e = m.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestStrmOutputRejectsTraversal(t *testing.T) {
	s := &StrmOutput{Root: t.TempDir()}
	if _, e := s.Write(context.Background(), "k", filepath.Join(s.Root, "..", "escape.strm"), "x", "", ""); e == nil {
		t.Fatal("允许越界路径")
	}
}

func TestStrmClearResumeKeepsGeneratedFiles(t *testing.T) {
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	c, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	root := t.TempDir()
	p := filepath.Join(root, "new.strm")
	if e = os.WriteFile(p, []byte("new"), 0644); e != nil {
		t.Fatal(e)
	}
	m.ExpectExec("INSERT INTO t_strm_clear_run").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT completed").WillReturnRows(sqlmock.NewRows([]string{"completed"}).AddRow(true))
	s := &StrmOutput{Root: root, Run: "same-task", Store: &dao.StrmExportDAO{Conn: c}}
	if e = s.Clear(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); e != nil {
		t.Fatal("恢复时删除了已生成文件")
	}
	if e = m.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestStrmClearRefreshesManifestAndReleasesDirectoryExclusive(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	c, _ := db.Conn(context.Background())
	defer c.Close()
	root := t.TempDir()
	p := filepath.Join(root, "concurrent.strm")
	if err := os.WriteFile(p, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	m.ExpectExec("INSERT INTO t_strm_clear_run").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("SELECT completed").WillReturnRows(sqlmock.NewRows([]string{"completed"}).AddRow(false))
	m.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	chain := []string{}
	for q := root; ; q = filepath.Dir(q) {
		chain = append([]string{q}, chain...)
		if filepath.Dir(q) == q {
			break
		}
	}
	for i, q := range chain {
		fn := "SELECT pg_try_advisory_lock_shared"
		if i == len(chain)-1 {
			fn = "SELECT pg_try_advisory_lock\\("
		}
		m.ExpectQuery(fn).WithArgs(q).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	}
	m.ExpectQuery("SELECT output_path FROM t_strm_export_state").WillReturnRows(sqlmock.NewRows([]string{"path"}).AddRow(p))
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO t_strm_export_state").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE t_strm_export_state SET state='missing'").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("DELETE FROM t_strm_export_history").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE t_strm_clear_run").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	m.ExpectExec("SELECT pg_advisory_lock_shared\\(").WithArgs(root).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("SELECT pg_advisory_unlock\\(").WithArgs(root).WillReturnResult(sqlmock.NewResult(0, 1))
	s := &StrmOutput{Store: &dao.StrmExportDAO{Conn: c}, Root: root, Run: "clear", Paths: map[string]bool{}}
	if err := s.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("目录没有清空", err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
