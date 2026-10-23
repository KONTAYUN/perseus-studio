package store

import (
	"context"
	"time"
)

// Background AI tasks have no durable run checkpoint. Restarting one from the
// original prompt could repeat a command or overwrite an already-produced file.
func (s *TaskStore) RequeueRunningTasks(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	const interrupted = "服务重启时后台任务已中断，执行结果未确认；请先核验工作区与命令产物，再新建任务。系统不会自动重复执行。"
	_, err = tx.ExecContext(ctx, `
INSERT INTO task_logs (user_id,task_id,seq,level,message,created_at)
SELECT t.user_id,t.id,COALESCE((SELECT MAX(seq)+1 FROM task_logs WHERE task_id=t.id),1),'system',
 CASE WHEN cancel_requested_at IS NOT NULL THEN 'service restarted; cancellation retained'
      WHEN type = 'mock' THEN 'service restarted, mock task requeued' ELSE ? END, ?
FROM tasks t WHERE status = 'running' OR (status = 'pending' AND cancel_requested_at IS NOT NULL)`, interrupted, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO task_events(id,user_id,task_id,execution_id,type,level,seq,payload_json,created_at)
SELECT 'tevt_recovery_' || lower(hex(randomblob(16))),t.user_id,t.id,'',
 CASE WHEN cancel_requested_at IS NOT NULL THEN 'task_canceled' ELSE 'task_failed' END,'warn',
 COALESCE((SELECT MAX(seq)+1 FROM task_events WHERE task_id=t.id),1),
 json_object('status',CASE WHEN cancel_requested_at IS NOT NULL THEN 'canceled' ELSE 'failed' END,
 'stage',CASE WHEN cancel_requested_at IS NOT NULL THEN 'canceled' ELSE 'recovery_required' END,
 'reason','service_restart','message',?),?
FROM tasks t WHERE type='ai_agent' AND (status='running' OR (status='pending' AND cancel_requested_at IS NOT NULL))`, interrupted, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE tasks SET
 status = CASE WHEN cancel_requested_at IS NOT NULL THEN 'canceled' WHEN type = 'mock' THEN 'pending' ELSE 'failed' END,
 worker_id = NULL,
 started_at = CASE WHEN type = 'mock' AND cancel_requested_at IS NULL THEN NULL ELSE started_at END,
 finished_at = CASE WHEN type = 'mock' AND cancel_requested_at IS NULL THEN NULL ELSE ? END,
 progress_stage = CASE WHEN cancel_requested_at IS NOT NULL THEN 'canceled' WHEN type = 'mock' THEN 'requeued' ELSE 'recovery_required' END,
 progress_message = CASE WHEN cancel_requested_at IS NOT NULL THEN 'service restarted; cancellation retained' WHEN type = 'mock' THEN 'service restarted, mock task requeued' ELSE ? END,
 error_message = CASE WHEN cancel_requested_at IS NULL AND type <> 'mock' THEN ? ELSE NULL END
WHERE status = 'running' OR (status = 'pending' AND cancel_requested_at IS NOT NULL)`, now, interrupted, interrupted)
	if err != nil {
		return err
	}
	return tx.Commit()
}
