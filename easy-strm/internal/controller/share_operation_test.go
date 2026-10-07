package controller

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"sync"
	"testing"
	"time"
)

// 此替身只验证HTTP/MCP的提交契约；执行、事务和恢复由Service与DAO测试覆盖。
type controllerOperationStore struct {
	mu   sync.Mutex
	row  domain.ShareOperation
	err  error
	done chan struct{}
	once sync.Once
}

func (m *controllerOperationStore) Enqueue(_ context.Context, v domain.ShareOperation) (domain.ShareOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return v, m.err
	}
	v.Status = "pending"
	m.row = v
	return v, nil
}
func (m *controllerOperationStore) Recoverable(context.Context) ([]domain.ShareOperation, error) {
	return nil, nil
}
func (m *controllerOperationStore) Get(context.Context, string) (domain.ShareOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.row
	v.Status = "cancelled"
	return v, nil
}
func (m *controllerOperationStore) Start(context.Context, string) (bool, error) { return false, nil }
func (m *controllerOperationStore) Cancel(context.Context, string) error        { return nil }
func (m *controllerOperationStore) Finish(context.Context, string, string, string, map[string]interface{}) error {
	return nil
}
func (m *controllerOperationStore) Published(context.Context, string) error {
	m.once.Do(func() { close(m.done) })
	return nil
}

func newControllerOperationService(t *testing.T, db *sql.DB) (*service.ShareRecordService, *controllerOperationStore) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	s := service.NewShareRecordService(dao.NewShareRecordDAO(db), nil, service.NewTaskService(dao.NewTaskRedisDAO(client)), nil)
	store := &controllerOperationStore{done: make(chan struct{})}
	s.SetOperationStore(store)
	t.Cleanup(func() {
		store.mu.Lock()
		started := store.row.TaskID != ""
		store.mu.Unlock()
		if started {
			select {
			case <-store.done:
			case <-time.After(time.Second):
				t.Error("提交任务未退出")
			}
		}
		client.Close()
	})
	return s, store
}

func checkAcceptedOperation(t *testing.T, payload map[string]interface{}) {
	t.Helper()
	if fmt.Sprint(payload["task_id"]) == "" || payload["task_id"] == nil {
		t.Fatalf("缺少task_id: %v", payload)
	}
	if _, ok := payload["deleted"]; ok {
		t.Fatalf("接受提交时误报已删除: %v", payload)
	}
}
