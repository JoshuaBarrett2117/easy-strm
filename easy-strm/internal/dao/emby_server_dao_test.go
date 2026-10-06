package dao

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func embyServerTestRows(port int) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time", "proxy_port"}).
		AddRow(1, "test", "http://emby:8096", "key", true, true, now, now, port)
}

func TestEmbyServerDAOProxyPortConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("INSERT INTO t_emby_server").WithArgs("test", "http://emby", "key", false, false, 8097).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "uk_emby_server_proxy_port"})
	mock.ExpectRollback()
	_, err = NewEmbyServerDAO(db).Create("test", "http://emby", "key", false, false, 8097)
	if err == nil || !strings.Contains(err.Error(), "反代端口已被其他 Emby 实例配置") {
		t.Fatalf("冲突错误不可读: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyServerDAOProxyPortReads(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dao := NewEmbyServerDAO(db)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + embyServerColumns + " FROM t_emby_server ORDER BY is_default DESC, id ASC")).WillReturnRows(embyServerTestRows(8097))
	servers, err := dao.List()
	if err != nil || len(servers) != 1 || servers[0].ProxyPort != 8097 {
		t.Fatalf("列表=%v err=%v", servers, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + embyServerColumns + " FROM t_emby_server WHERE id=$1")).WithArgs(1).WillReturnRows(embyServerTestRows(8098))
	server, err := dao.GetByID(1)
	if err != nil || server.ProxyPort != 8098 {
		t.Fatalf("读取=%v err=%v", server, err)
	}
	mock.ExpectQuery("SELECT " + regexp.QuoteMeta(embyServerColumns) + " FROM t_emby_server WHERE enabled=true").WillReturnRows(embyServerTestRows(0))
	server, err = dao.GetDefault()
	if err != nil || server.ProxyPort != 0 {
		t.Fatalf("默认实例=%v err=%v", server, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyServerDAOProxyPortWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dao := NewEmbyServerDAO(db)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE t_emby_server SET is_default=false").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO t_emby_server(name, base_url, api_key, enabled, is_default, proxy_port) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+embyServerColumns)).WithArgs("test", "http://emby:8096", "key", true, true, 8097).WillReturnRows(embyServerTestRows(8097))
	mock.ExpectCommit()
	server, err := dao.Create("test", "http://emby:8096/", "key", true, false, 8097)
	if err != nil || server.ProxyPort != 8097 {
		t.Fatalf("新增=%v err=%v", server, err)
	}
	for _, failure := range []bool{false, true} {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE t_emby_server SET is_default=false").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 0))
		query := mock.ExpectQuery(regexp.QuoteMeta("UPDATE t_emby_server SET name=$2, base_url=$3, api_key=CASE WHEN $4='' THEN api_key ELSE $4 END, enabled=$5, is_default=$6, proxy_port=$7, update_time=CURRENT_TIMESTAMP WHERE id=$1 RETURNING "+embyServerColumns)).WithArgs(1, "test", "http://emby:8096", "", false, true, 0)
		if failure {
			query.WillReturnError(errors.New("write failed"))
			mock.ExpectRollback()
		} else {
			query.WillReturnRows(embyServerTestRows(0))
			mock.ExpectCommit()
		}
		server, err = dao.Update(1, "test", "http://emby:8096/", "", false, true, 0)
		if failure && err == nil {
			t.Fatal("必须返回写入失败")
		}
		if !failure && (err != nil || server.ProxyPort != 0) {
			t.Fatalf("更新=%v err=%v", server, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
