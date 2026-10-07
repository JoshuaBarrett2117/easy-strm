package service

import (
	"context"
	"testing"
)

// TestClearMediaWhileIdentifying 清空不得与后台识别同时发生，也不能对非法ID访问DAO。
func TestClearMediaWhileIdentifying(t *testing.T) {
	s := &ShareRecordService{}
	if _, err := s.ClearMedia(context.Background(), 0); err == nil {
		t.Fatal("应拒绝非法ID")
	}
	release, err := s.Coordinator().acquire(context.Background(), nil, shareResource{key: shareKey(9)})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := s.ClearMedia(context.Background(), 9); err == nil {
		t.Fatal("识别运行中不得清空")
	}
}
