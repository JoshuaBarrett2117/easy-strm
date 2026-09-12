package service

import (
	"context"
	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"testing"
)

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
			m.ExpectQuery("SELECT owner_key,export_key").WillReturnRows(rows)
			if scenario == "unchanged" || scenario == "missing" {
				m.ExpectExec("INSERT INTO t_strm_export_history VALUES").WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectBegin()
				m.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 0))
				m.ExpectExec("INSERT INTO t_strm_export_state").WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			}
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
