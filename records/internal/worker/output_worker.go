package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"records/internal/ai"
	"records/internal/models"
	"records/internal/normalization"
	"records/internal/repository"
	"records/pkg/logger"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// OutputTask 输出任务
type OutputTask struct {
	SessionID uuid.UUID
	UserID    string
	CreatedAt time.Time
}

// OutputWorker 输出阶段异步工作器
type OutputWorker struct {
	db         *sqlx.DB
	repo       *repository.Repository
	normalizer *normalization.Normalizer
	logger     logger.Logger
	taskQueue  chan OutputTask
	wg         sync.WaitGroup
	stopCh     chan struct{}
	workerSize int
}

// NewOutputWorker 创建输出工作器
func NewOutputWorker(
	db *sqlx.DB,
	aiClient ai.Client,
	repo *repository.Repository,
	logger logger.Logger,
	workerSize int,
) *OutputWorker {
	if workerSize <= 0 {
		workerSize = 5 // 默认5个工作协程
	}

	normalizer := normalization.NewNormalizer(aiClient, repo, logger)

	return &OutputWorker{
		db:         db,
		repo:       repo,
		normalizer: normalizer,
		logger:     logger,
		taskQueue:  make(chan OutputTask, 100), // 缓冲队列，最多100个任务
		stopCh:     make(chan struct{}),
		workerSize: workerSize,
	}
}

// Start 启动工作器
func (w *OutputWorker) Start() {
	w.logger.Info("Starting output worker", "worker_size", w.workerSize)

	for i := 0; i < w.workerSize; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
}

// Stop 停止工作器
func (w *OutputWorker) Stop() {
	w.logger.Info("Stopping output worker")
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("Output worker stopped")
}

// SubmitTask 提交输出任务（非阻塞）
func (w *OutputWorker) SubmitTask(sessionID uuid.UUID, userID string) error {
	task := OutputTask{
		SessionID: sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	select {
	case w.taskQueue <- task:
		w.logger.Info("Output task submitted", "session_id", sessionID, "user_id", userID)
		return nil
	default:
		// 队列已满，返回错误
		return fmt.Errorf("output task queue is full")
	}
}

// worker 工作协程
func (w *OutputWorker) worker(id int) {
	defer w.wg.Done()

	w.logger.Info("Output worker started", "worker_id", id)

	for {
		select {
		case <-w.stopCh:
			w.logger.Info("Output worker stopping", "worker_id", id)
			return

		case task := <-w.taskQueue:
			w.logger.Info("Processing output task",
				"worker_id", id,
				"session_id", task.SessionID,
				"user_id", task.UserID)

			// 处理任务
			if err := w.processTask(context.Background(), task); err != nil {
				w.logger.Error("Failed to process output task",
					"worker_id", id,
					"session_id", task.SessionID,
					"error", err)
			} else {
				w.logger.Info("Output task completed",
					"worker_id", id,
					"session_id", task.SessionID,
					"duration", time.Since(task.CreatedAt))
			}
		}
	}
}

// processTask 处理输出任务
func (w *OutputWorker) processTask(ctx context.Context, task OutputTask) error {
	// 使用事务确保数据一致性
	return w.repo.WithTx(ctx, func(txCtx context.Context) error {
		// 1. 执行客户/联系人归一处理
		w.logger.Info("Starting entity normalization", "session_id", task.SessionID)
		mergeMap, err := w.normalizer.NormalizeEntities(ctx, task.SessionID)
		if err != nil {
			w.logger.Error("Entity normalization failed", "error", err)
			// 归一失败不影响后续流程，继续执行
		} else if len(mergeMap) > 0 {
			w.logger.Info("Entity normalization completed", "merge_count", len(mergeMap))
			// 执行客户合并
			if err := w.mergeCustomers(ctx, mergeMap); err != nil {
				w.logger.Error("Failed to merge customers", "error", err)
				// 合并失败不影响后续流程
			}
		}

		// 2. 输出跟进记录
		if err := w.outputFollowRecords(ctx, task.SessionID, task.UserID); err != nil {
			return fmt.Errorf("failed to output follow records: %w", err)
		}

		// 3. 删除该会话的 dialog 记录（需先于 session 删除以满足 FK 约束；按 session_id 删除，利用索引，不同会话无锁竞争）
		if err := w.repo.DeleteDialogsBySession(ctx, task.SessionID); err != nil {
			w.logger.Error("Failed to delete dialogs after outputting", "session_id", task.SessionID, "error", err)
			// 删除失败不阻断流程，数据已持久化到 follow_records
		}

		// 4. 删除 session 记录（OUTPUTTING 已完成，释放存储；按主键删除，无锁竞争）
		if err := w.repo.DeleteSession(ctx, task.SessionID); err != nil {
			w.logger.Error("Failed to delete session after outputting", "session_id", task.SessionID, "error", err)
			// 兜底：删除失败时更新为 EXIT，避免 dialogs 已删但 session 仍为 OUTPUTTING 导致被误判为活跃
			endTime := time.Now()
			_ = w.repo.UpdateSession(ctx, &models.Session{ID: task.SessionID, Status: models.StatusExit, EndedAt: &endTime})
		}

		return nil
	})
}

func (w *OutputWorker) outputFollowRecords(ctx context.Context, sessionID uuid.UUID, userID string) error {
	// 获取最新 dialog 的 runtime_snapshot，从中读取 pending_updates
	latestDialog, err := w.repo.GetLatestDialog(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get latest dialog for session %s: %w", sessionID, err)
	}
	if latestDialog == nil {
		return nil
	}

	pendingUpdates := w.loadPendingUpdatesFromSnapshot(latestDialog)
	if len(pendingUpdates) == 0 {
		return nil
	}

	// 获取各客户首次聚焦时间（用于 follow_time）
	dialogs, err := w.repo.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get dialogs for session %s: %w", sessionID, err)
	}
	firstFocusTimes := make(map[uuid.UUID]time.Time)
	for _, dialog := range dialogs {
		if dialog.FocusCustomerID != nil && dialog.IsFirstFocus {
			firstFocusTimes[*dialog.FocusCustomerID] = dialog.CreatedAt
		}
	}

	var batchErrors []string
	successCount := 0

	for customerKey, data := range pendingUpdates {
		if len(data) == 0 {
			continue
		}
		customerID, err := uuid.Parse(customerKey)
		if err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("invalid customer id %s: %v", customerKey, err))
			continue
		}

		customer, err := w.repo.GetCustomer(ctx, customerID)
		if err != nil || customer == nil {
			batchErrors = append(batchErrors, fmt.Sprintf("get customer %s: %v", customerID, err))
			continue
		}

		// 从 pending_updates 构建 FollowRecord 并创建到 DB
		followRecord := w.buildFollowRecordFromPendingData(customer, data)

		followTime := time.Now()
		if t, exists := firstFocusTimes[customerID]; exists {
			followTime = t
		}
		followRecord.FollowTime = followTime
		followRecord.UserID = userID
		followRecord.ID = uuid.New()

		if err := w.repo.CreateFollowRecord(ctx, followRecord); err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("create follow record %s: %v", customerID, err))
			continue
		}
		successCount++
	}

	if len(batchErrors) > 0 {
		w.logger.Warn("Output follow records completed with errors",
			"session_id", sessionID,
			"total", len(pendingUpdates),
			"success", successCount,
			"failed", len(batchErrors),
			"first_error", batchErrors[0])
	}

	return nil
}

// loadPendingUpdatesFromSnapshot 从快照加载 pending_updates，兼容旧版 flat 结构
func (w *OutputWorker) loadPendingUpdatesFromSnapshot(dialog *models.Dialog) map[string]map[string]interface{} {
	var snapshot struct {
		PendingUpdates  json.RawMessage `json:"pending_updates"`
		FocusCustomerID *uuid.UUID      `json:"focus_customer_id"`
	}
	if err := json.Unmarshal(dialog.RuntimeSnapshot, &snapshot); err != nil || len(snapshot.PendingUpdates) == 0 || string(snapshot.PendingUpdates) == "null" {
		return nil
	}
	// 新结构：map[customer_id]map[field]value
	var newFormat map[string]map[string]interface{}
	if err := json.Unmarshal(snapshot.PendingUpdates, &newFormat); err == nil && len(newFormat) > 0 {
		return newFormat
	}
	// 旧版 flat 结构
	var flatFormat map[string]interface{}
	if err := json.Unmarshal(snapshot.PendingUpdates, &flatFormat); err == nil && len(flatFormat) > 0 && snapshot.FocusCustomerID != nil {
		return map[string]map[string]interface{}{snapshot.FocusCustomerID.String(): flatFormat}
	}
	return nil
}

// buildFollowRecordFromPendingData 从 pending_updates 构建 FollowRecord
func (w *OutputWorker) buildFollowRecordFromPendingData(customer *models.Customer, data map[string]interface{}) *models.FollowRecord {
	r := &models.FollowRecord{
		CustomerID:   customer.ID,
		CustomerName: customer.Name,
		FollowTime:   time.Now(),
	}
	if customer.ContactPerson != nil {
		r.ContactPerson = customer.ContactPerson
	}
	if customer.ContactPhone != nil {
		r.ContactPhone = customer.ContactPhone
	}
	if customer.ContactRole != nil {
		r.ContactRole = customer.ContactRole
	}
	for k, v := range data {
		s := fmt.Sprintf("%v", v)
		switch k {
		case "follow_method":
			r.FollowMethod = &s
		case "follow_content":
			r.FollowContent = &s
		case "follow_goal":
			r.FollowGoal = &s
		case "follow_result":
			r.FollowResult = &s
		case "next_plan":
			r.NextPlan = &s
		case "risk_content":
			r.RiskContent = &s
		}
	}
	return r
}

func (w *OutputWorker) mergeCustomers(ctx context.Context, mergeMap map[uuid.UUID]uuid.UUID) error {
	if len(mergeMap) == 0 {
		return nil
	}

	var batchErrors []string
	successCount := 0

	for sourceID, targetID := range mergeMap {
		sourceCustomer, err := w.repo.GetCustomer(ctx, sourceID)
		if err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("get source customer %s: %v", sourceID, err))
			continue
		}

		targetCustomer, err := w.repo.GetCustomer(ctx, targetID)
		if err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("get target customer %s: %v", targetID, err))
			continue
		}

		if sourceCustomer == nil || targetCustomer == nil {
			batchErrors = append(batchErrors, fmt.Sprintf("customer not found: source=%s, target=%s", sourceID, targetID))
			continue
		}

		if targetCustomer.ContactPerson == nil && sourceCustomer.ContactPerson != nil {
			targetCustomer.ContactPerson = sourceCustomer.ContactPerson
		}
		if targetCustomer.ContactPhone == nil && sourceCustomer.ContactPhone != nil {
			targetCustomer.ContactPhone = sourceCustomer.ContactPhone
		}
		if targetCustomer.ContactRole == nil && sourceCustomer.ContactRole != nil {
			targetCustomer.ContactRole = sourceCustomer.ContactRole
		}

		if err := w.repo.UpdateCustomer(ctx, targetCustomer); err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("update target customer %s: %v", targetID, err))
			continue
		}

		sourceRecords, err := w.repo.GetCustomerFollowRecords(ctx, sourceID)
		if err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("get source follow records %s: %v", sourceID, err))
			continue
		}

		recordErrors := 0
		for _, record := range sourceRecords {
			record.CustomerID = targetID
			record.CustomerName = targetCustomer.Name
			if err := w.repo.UpdateFollowRecord(ctx, record); err != nil {
				recordErrors++
			}
		}
		if recordErrors > 0 {
			batchErrors = append(batchErrors, fmt.Sprintf("merge %s->%s: %d follow record errors", sourceID, targetID, recordErrors))
		}

		successCount++
	}

	w.logger.Info("Customer merge batch completed",
		"total", len(mergeMap),
		"success", successCount,
		"failed", len(batchErrors))

	if len(batchErrors) > 0 {
		w.logger.Warn("Some merges had errors", "first_error", batchErrors[0], "error_count", len(batchErrors))
	}

	return nil
}
