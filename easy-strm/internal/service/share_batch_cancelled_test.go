package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"strings"
	"testing"
)

type cancelledBatchParser struct {
	calls    []string
	ordinary bool
}

func (p *cancelledBatchParser) ParseShareLink(_ context.Context, url, password string) (*domain.ParseShareResponse, error) {
	p.calls = append(p.calls, url)
	if url == "cancelled" {
		if p.ordinary {
			return nil, fmt.Errorf("访问密码错误")
		}
		return nil, ErrShareCancelled
	}
	return &domain.ParseShareResponse{}, nil
}

// TestCancelledShareDoesNotFailBatch 验证跳过已取消分享、继续后续解析及全部取消时正常完成。
func TestCancelledShareDoesNotFailBatch(t *testing.T) {
	for _, tt := range []struct {
		name              string
		healthy, ordinary bool
		want              string
	}{
		{"继续下一个分享", true, false, "completed"},
		{"全部取消", false, false, "completed"},
		{"普通失败不能伪装为全部跳过", false, true, "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			mini := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
			defer client.Close()
			dao.InitTaskRedisDAO(client)
			tasks := NewTaskService(dao.NewTaskRedisDAO(client))
			if err := tasks.Create("skip-test", "share_identify", "测试"); err != nil {
				t.Fatal(err)
			}
			list := func(cancelled bool) {
				count := 1
				if tt.healthy {
					count++
				}
				mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
				rows := sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"})
				// 已取消分享保留历史待识别文件，必须排除，否则会调用未配置的TMDB服务。
				rows.AddRow(1, "auto", "取消分享", "cancelled", "", "", 1, "now", "now", cancelled, 11, "旧媒体.mkv", "auto", "pending", nil, "", 1)
				if tt.ordinary {
					rows = sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}).AddRow(1, "auto", "取消分享", "cancelled", "", "", 1, "now", "now", false, nil, nil, nil, nil, nil, nil, nil)
				}
				if tt.healthy {
					rows.AddRow(2, "auto", "正常分享", "healthy", "", "", 1, "now", "now", false, 22, "目录***", "auto", "masked", nil, "", 1)
				}
				mock.ExpectQuery("FROM \\(SELECT .* FROM t_share_record").WillReturnRows(rows)
			}
			list(false)
			if !tt.ordinary {
				mock.ExpectExec("UPDATE t_share_record SET share_cancelled").WithArgs(true, 1, "cancelled", "").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			list(!tt.ordinary)
			parser := &cancelledBatchParser{ordinary: tt.ordinary}
			s := &ShareRecordService{dao: dao.NewShareRecordDAO(db), tasks: tasks, parser: parser}
			s.runBatchIdentify(context.Background(), "skip-test", nil, false, nil)
			task, err := tasks.Get("skip-test")
			if err != nil || task["status"] != tt.want {
				t.Fatalf("%+v %v", task, err)
			}
			if tt.healthy && (len(parser.calls) != 2 || parser.calls[1] != "healthy") {
				t.Fatal(parser.calls)
			}
			if !tt.ordinary {
				raw, _ := json.Marshal(task["metadata"])
				if !strings.Contains(string(raw), "cancelled_shares") {
					t.Fatalf("缺少取消分享统计: %+v", task)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
