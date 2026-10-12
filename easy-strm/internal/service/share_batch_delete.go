package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"strings"
)

func shareDeletedIDs(result map[string]interface{}) []int {
	ids := []int{}
	switch values := result["deleted_ids"].(type) {
	case []int:
		ids = append(ids, values...)
	case []interface{}:
		for _, value := range values {
			switch number := value.(type) {
			case float64:
				if number > 0 && float64(int(number)) == number {
					ids = append(ids, int(number))
				}
			case int:
				ids = append(ids, number)
			}
		}
	}
	return ids
}

// runDelete 接收执行上下文和删除任务，按 ShareIDs 顺序复用 deleteShare 清理分享。
// 仅跳过事务内持久化的成功检查点；失败与缺失项在任务恢复后重新尝试。
// 单项失败继续后续删除，sql.ErrNoRows 计入跳过项；最终将删除数及成功、失败、跳过ID汇总到 Result。
// 全部尝试结束后返回聚合错误，无失败时返回 nil。
func (q *shareOperationQueue) runDelete(ctx context.Context, operation *domain.ShareOperation) error {
	prior := map[int]bool{}
	for _, id := range shareDeletedIDs(operation.Result) {
		prior[id] = true
	}
	deleted, failed, skipped := []int{}, []int{}, []int{}
	for _, id := range operation.ShareIDs {
		if prior[id] {
			deleted = append(deleted, id)
		}
	}
	operation.Result = map[string]interface{}{"deleted": int64(len(deleted)), "deleted_ids": deleted, "failed_ids": failed, "skipped_ids": skipped}
	failures := []string{}
	for _, id := range operation.ShareIDs {
		if prior[id] {
			continue
		}
		deleteCtx := ctx
		if len(operation.ShareIDs) > 1 {
			deleteCtx = dao.WithShareBatchDelete(ctx, operation.TaskID, id)
		}
		err := q.s.deleteShare(deleteCtx, id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			skipped = append(skipped, id)
		case err != nil:
			failed = append(failed, id)
			failures = append(failures, fmt.Sprintf("%d: %v", id, err))
		default:
			deleted = append(deleted, id)
		}
		operation.Result["deleted"] = int64(len(deleted))
		operation.Result["deleted_ids"] = deleted
		operation.Result["failed_ids"] = failed
		operation.Result["skipped_ids"] = skipped
	}
	if len(failures) > 0 {
		return fmt.Errorf("分享删除失败（%d 个）：%s", len(failed), strings.Join(failures, "；"))
	}
	return nil
}
