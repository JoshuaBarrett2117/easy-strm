package dao

import (
	"database/sql"
	"fmt"
	"time"

	"easy-strm/internal/domain"
)

type TaskStepDAO struct{}

func NewTaskStepDAO() *TaskStepDAO {
	return &TaskStepDAO{}
}

const taskStepColumns = `id, task_id, step_key, step_name, status, sort_order, input_summary, output_summary, error_message, started_at, finished_at, created_at, updated_at`

func scanTaskStep(scanner interface{ Scan(...interface{}) error }) (*domain.TaskStep, error) {
	step := &domain.TaskStep{}
	err := scanner.Scan(
		&step.ID,
		&step.TaskID,
		&step.StepKey,
		&step.StepName,
		&step.Status,
		&step.SortOrder,
		&step.InputSummary,
		&step.OutputSummary,
		&step.ErrorMessage,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
	)
	return step, err
}

func (d *TaskStepDAO) Create(step *domain.TaskStep) (*domain.TaskStep, error) {
	if step == nil {
		return nil, fmt.Errorf("TaskStepDAO[Create] 步骤不能为空")
	}
	row := DB.QueryRow(
		`INSERT INTO t_task_step (task_id, step_key, step_name, status, sort_order, input_summary, output_summary, error_message, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (task_id, step_key) DO UPDATE SET
			step_name=EXCLUDED.step_name,
			status=EXCLUDED.status,
			sort_order=EXCLUDED.sort_order,
			input_summary=EXCLUDED.input_summary,
			output_summary=EXCLUDED.output_summary,
			error_message=EXCLUDED.error_message,
			started_at=EXCLUDED.started_at,
			finished_at=EXCLUDED.finished_at,
			updated_at=CURRENT_TIMESTAMP
		RETURNING `+taskStepColumns,
		step.TaskID,
		step.StepKey,
		step.StepName,
		defaultString(step.Status, domain.TaskStatusPending),
		step.SortOrder,
		step.InputSummary,
		step.OutputSummary,
		step.ErrorMessage,
		step.StartedAt,
		step.FinishedAt,
	)
	result, err := scanTaskStep(row)
	if err != nil {
		return nil, fmt.Errorf("TaskStepDAO[Create] 写入失败: %v", err)
	}
	return result, nil
}

func (d *TaskStepDAO) UpdateStatus(taskID, stepKey, status, outputSummary, errorMessage string, startedAt, finishedAt *time.Time) error {
	result, err := DB.Exec(
		`UPDATE t_task_step
		SET status=$3, output_summary=$4, error_message=$5,
			started_at=COALESCE($6, started_at),
			finished_at=COALESCE($7, finished_at),
			updated_at=CURRENT_TIMESTAMP
		WHERE task_id=$1 AND step_key=$2`,
		taskID,
		stepKey,
		status,
		outputSummary,
		errorMessage,
		startedAt,
		finishedAt,
	)
	if err != nil {
		return fmt.Errorf("TaskStepDAO[UpdateStatus] 更新失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("TaskStepDAO[UpdateStatus] 步骤不存在")
	}
	return nil
}

func (d *TaskStepDAO) ListByTask(taskID string) ([]*domain.TaskStep, error) {
	rows, err := DB.Query(
		`SELECT `+taskStepColumns+` FROM t_task_step WHERE task_id=$1 ORDER BY sort_order ASC, id ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("TaskStepDAO[ListByTask] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.TaskStep
	for rows.Next() {
		step, err := scanTaskStep(rows)
		if err != nil {
			return nil, fmt.Errorf("TaskStepDAO[ListByTask] 扫描失败: %v", err)
		}
		list = append(list, step)
	}
	return list, nil
}

func (d *TaskStepDAO) Get(taskID, stepKey string) (*domain.TaskStep, error) {
	step, err := scanTaskStep(DB.QueryRow(
		`SELECT `+taskStepColumns+` FROM t_task_step WHERE task_id=$1 AND step_key=$2`,
		taskID,
		stepKey,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("TaskStepDAO[Get] 查询失败: %v", err)
	}
	return step, nil
}
