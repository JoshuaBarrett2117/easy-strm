package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

type cancelledShareParser struct{ err error }

func (p cancelledShareParser) ParseShareLink(context.Context, string, string) (*domain.ParseShareResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &domain.ParseShareResponse{}, nil
}

// TestShareCancellationPersistence 验证取消状态入库、恢复清除、普通错误不改状态及数据库失败上报。
func TestShareCancellationPersistence(t *testing.T) {
	for _, tt := range []struct {
		name     string
		old      bool
		parseErr error
		write    bool
		next     bool
		fail     bool
	}{
		{"取消", false, mapShareSnapError(fmt.Errorf("分享已取消")), true, true, false},
		{"已标记", true, ErrShareCancelled, false, true, false},
		{"恢复", true, nil, true, false, false},
		{"正常", false, nil, false, false, false},
		{"超时", false, context.DeadlineExceeded, false, false, false},
		{"任务取消", false, context.Canceled, false, false, false},
		{"密码错误", false, mapShareSnapError(fmt.Errorf("990011")), false, false, false},
		{"失效原因不明", false, mapShareSnapError(fmt.Errorf("4100009")), false, false, false},
		{"网络错误保留旧状态", true, fmt.Errorf("network error"), false, true, false},
		{"数据库失败", false, ErrShareCancelled, true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			record := domain.ShareRecord{ID: 7, URL: "https://115.com/s/test", Password: "0000", ShareCancelled: tt.old}
			if tt.write {
				e := mock.ExpectExec("UPDATE t_share_record SET share_cancelled").WithArgs(tt.next, 7, record.URL, record.Password)
				if tt.fail {
					e.WillReturnError(fmt.Errorf("db offline"))
				} else {
					e.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			s := &ShareRecordService{dao: dao.NewShareRecordDAO(db), parser: cancelledShareParser{tt.parseErr}}
			_, err := s.parseRecordShare(context.Background(), record)
			if tt.fail {
				if err == nil || errors.Is(err, ErrShareCancelled) {
					t.Fatalf("应上报数据库失败: %v", err)
				}
			} else if !errors.Is(err, tt.parseErr) {
				t.Fatalf("err=%v want=%v", err, tt.parseErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareCancellationClassification(t *testing.T) {
	for _, msg := range []string{"分享已取消", "分享已被取消", "share has been cancelled"} {
		if !errors.Is(mapShareSnapError(fmt.Errorf("%s", msg)), ErrShareCancelled) {
			t.Fatal(msg)
		}
	}
	if isShareCancelledError(fmt.Errorf("分享已取消: %w", context.DeadlineExceeded)) {
		t.Fatal("超时不能标记分享取消")
	}
}
