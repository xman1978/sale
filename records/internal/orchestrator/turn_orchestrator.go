package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"records/internal/ai"
	"records/internal/engine"
	"records/internal/models"
	"records/internal/repository"
	"records/internal/worker"
	"records/pkg/logger"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// TurnOrchestrator 对话轮次编排器
type TurnOrchestrator struct {
	db                   *sqlx.DB
	aiClient             ai.Client
	ruleEngine           *engine.RuleEngine
	repo                 *repository.Repository
	outputWorker         *worker.OutputWorker
	logger               logger.Logger
	askingOtherCustomers string
	outputtingConfirm    string
	outputtingEnded      string // OUTPUTTING 阶段用户发非跟进信息时的友好提示
}

// NewTurnOrchestrator 创建对话轮次编排器
func NewTurnOrchestrator(
	db *sqlx.DB,
	aiClient ai.Client,
	ruleEngine *engine.RuleEngine,
	repo *repository.Repository,
	outputWorker *worker.OutputWorker,
	logger logger.Logger,
	askingOtherCustomers string,
	outputtingConfirm string,
	outputtingEnded string,
) *TurnOrchestrator {

	return &TurnOrchestrator{
		db:                   db,
		aiClient:             aiClient,
		ruleEngine:           ruleEngine,
		repo:                 repo,
		outputWorker:         outputWorker,
		logger:               logger,
		askingOtherCustomers: askingOtherCustomers,
		outputtingConfirm:    outputtingConfirm,
		outputtingEnded:      outputtingEnded,
	}
}

// ProcessTurn 处理单轮对话
func (o *TurnOrchestrator) ProcessTurn(ctx context.Context, userID, userInput string) (string, error) {
	var reply string

	// 使用事务确保数据一致性
	err := o.repo.WithTx(ctx, func(txCtx context.Context) error {
		// 1. 加载或创建会话
		session, err := o.loadOrCreateSession(txCtx, userID)
		if err != nil {
			return fmt.Errorf("failed to load session: %w", err)
		}

		// 1.5 OUTPUTTING 阶段用户继续发消息：判断是否为新的客户跟进信息
		if session.Status == models.StatusOutputting {
			isFollowUp, err := o.aiClient.IsCustomerFollowRelated(ctx, userInput)
			if err != nil {
				o.logger.Error("IsCustomerFollowRelated failed in OUTPUTTING", "error", err)
			}
			if !isFollowUp {
				reply = o.outputtingEnded
				if reply == "" {
					reply = "对话已结束，如果有新的客户跟进情况要整理，再找我~"
				}
				return nil
			}
			// 是新的跟进信息，结束当前会话并创建新会话
			endTime := time.Now()
			if err := o.repo.UpdateSession(txCtx, &models.Session{ID: session.ID, Status: models.StatusExit, EndedAt: &endTime}); err != nil {
				return fmt.Errorf("end OUTPUTTING session: %w", err)
			}
			newSession := &models.Session{
				ID:     uuid.New(),
				UserID: userID,
				Status: models.StatusCollecting,
			}
			if err := o.repo.CreateSession(txCtx, newSession); err != nil {
				return fmt.Errorf("create new session: %w", err)
			}
			session = newSession
		}

		// 2. 加载最新运行态
		runtime, err := o.loadLatestRuntime(txCtx, session.ID)
		if err != nil {
			return fmt.Errorf("failed to load runtime: %w", err)
		}

		// 3. 增加轮次索引
		runtime.TurnIndex++

		// 4. 语义分析（如果需要）
		var semanticResult *models.SemanticAnalysisResult
		if o.ShouldCallSemanticAnalysis(txCtx, runtime.Status, userInput) {
			focusCustomerName := ""
			if runtime.FocusCustomerID != nil {
				customer, _ := o.repo.GetCustomer(txCtx, *runtime.FocusCustomerID)
				if customer != nil {
					focusCustomerName = customer.Name
				}
			}

			// 获取此前对话历史，帮助大模型理解上下文
			convHistory, _ := o.repo.GetSessionConversationHistory(txCtx, session.ID, runtime.TurnIndex)
			expectedField := o.getExpectedField(runtime.State)
			semanticResult, err = o.aiClient.SemanticAnalysis(ctx, userInput, runtime.Status, focusCustomerName, expectedField, convHistory)
			if err != nil {
				o.logger.Error("Semantic analysis failed", "error", err)
				// 语义分析失败不影响对话继续
			}
		}

		// 5. 规则引擎处理
		newRuntime, err := o.processWithRuleEngine(txCtx, runtime, semanticResult, userInput, userID)
		if err != nil {
			return fmt.Errorf("rule engine processing failed: %w", err)
		}

		// 6. 处理 OUTPUTTING 阶段（异步）
		if newRuntime.Status == models.StatusOutputting {
			// 提交异步任务，不阻塞用户交互
			if err := o.outputWorker.SubmitTask(session.ID, userID); err != nil {
				o.logger.Error("Failed to submit output task", "error", err)
				// 任务提交失败不影响对话回复
			} else {
				o.logger.Info("Output task submitted successfully", "session_id", session.ID)
			}

			// 立即更新会话状态为 OUTPUTTING（实际的 EXIT 状态由 worker 异步更新）
			session.Status = models.StatusOutputting
			if err := o.repo.UpdateSession(txCtx, session); err != nil {
				return fmt.Errorf("failed to update session: %w", err)
			}
		}

		// 7. 生成对话回复
		var replyErr error
		reply, replyErr = o.generateReply(txCtx, newRuntime, userInput)
		if replyErr != nil {
			o.logger.Error("Failed to generate reply", "error", replyErr)
			reply = "抱歉，我遇到了一些问题，请稍后再试。"
		}

		// 8. 持久化运行态快照（含原始对话内容，供后续大模型理解上下文）
		if err := o.saveRuntimeSnapshot(txCtx, session.ID, newRuntime, userInput, reply); err != nil {
			o.logger.Error("Failed to save runtime snapshot", "error", err)
			return fmt.Errorf("failed to save runtime snapshot: %w", err)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return reply, nil
}

// ShouldCallSemanticAnalysis 判断是否应该调用语义分析
func (o *TurnOrchestrator) ShouldCallSemanticAnalysis(
	ctx context.Context,
	status string,
	userInput string,
) bool {
	if status != models.StatusCollecting && status != models.StatusConfirming && status != models.StatusAskingOtherCustomers {
		return false
	}

	// 检查是否和客户跟进相关
	isCustomerFollowRelated, err := o.aiClient.IsCustomerFollowRelated(ctx, userInput)
	if err != nil {
		return false
	}

	if !isCustomerFollowRelated {
		return false
	}

	return true
}

// loadOrCreateSession 加载或创建会话
func (o *TurnOrchestrator) loadOrCreateSession(ctx context.Context, userID string) (*models.Session, error) {
	// 查找用户的活跃会话
	session, err := o.repo.GetActiveSession(ctx, userID)
	if err != nil {
		return nil, err
	}

	if session != nil {
		return session, nil
	}

	// 创建新会话
	session = &models.Session{
		ID:     uuid.New(),
		UserID: userID,
		Status: models.StatusCollecting,
	}

	if err := o.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// loadLatestRuntime 加载最新运行态
func (o *TurnOrchestrator) loadLatestRuntime(ctx context.Context, sessionID uuid.UUID) (*RuntimeContext, error) {
	dialog, err := o.repo.GetLatestDialog(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if dialog == nil {
		// 新会话，创建初始运行态
		return &RuntimeContext{
			SessionID:       sessionID,
			TurnIndex:       0,
			State:           models.StateCustomerName,
			Status:          models.StatusCollecting,
			FocusCustomerID: nil,
			PendingUpdates:  make(map[string]map[string]interface{}),
		}, nil
	}

	// 从快照恢复运行态（兼容旧版 flat pending_updates 与新版 per-customer 结构）
	pendingUpdates, err := o.loadPendingUpdatesFromSnapshot(dialog)
	if err != nil {
		return nil, err
	}

	var snapshot models.RuntimeState
	if err := json.Unmarshal(dialog.RuntimeSnapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal runtime snapshot: %w", err)
	}

	focusCustomerID := snapshot.FocusCustomerID
	// 注：若为 nil，由 processWithRuleEngine 入口的 ensureFocusWhenPending 统一恢复

	return &RuntimeContext{
		SessionID:        sessionID,
		TurnIndex:        dialog.TurnIndex,
		State:            snapshot.State,
		Status:           snapshot.Status,
		FocusCustomerID:  focusCustomerID,
		PendingUpdates:   pendingUpdates,
		PendingReconfirm: snapshot.PendingReconfirm,
	}, nil
}

// loadPendingUpdatesFromSnapshot 从快照加载 pending_updates，兼容旧版 flat 结构
func (o *TurnOrchestrator) loadPendingUpdatesFromSnapshot(dialog *models.Dialog) (map[string]map[string]interface{}, error) {
	var snapshot struct {
		PendingUpdates  json.RawMessage `json:"pending_updates"`
		FocusCustomerID *uuid.UUID      `json:"focus_customer_id"`
	}
	if err := json.Unmarshal(dialog.RuntimeSnapshot, &snapshot); err != nil {
		return nil, err
	}
	if len(snapshot.PendingUpdates) == 0 || string(snapshot.PendingUpdates) == "null" {
		return make(map[string]map[string]interface{}), nil
	}
	result := make(map[string]map[string]interface{})

	// 先尝试新结构：map[customer_id]map[field]value
	var newFormat map[string]map[string]interface{}
	if err := json.Unmarshal(snapshot.PendingUpdates, &newFormat); err == nil && len(newFormat) > 0 {
		return newFormat, nil
	}

	// 旧版 flat 结构：map[field]value，归入 focus_customer
	var flatFormat map[string]interface{}
	if err := json.Unmarshal(snapshot.PendingUpdates, &flatFormat); err == nil && len(flatFormat) > 0 && snapshot.FocusCustomerID != nil {
		result[snapshot.FocusCustomerID.String()] = flatFormat
	}
	return result, nil
}

// ensureFocusWhenPending 当有 PendingUpdates 但 focus 为 nil 时，从对话表恢复（统一入口，避免分散兜底）
func (o *TurnOrchestrator) ensureFocusWhenPending(ctx context.Context, runtime *RuntimeContext) {
	if runtime.FocusCustomerID != nil || len(runtime.PendingUpdates) == 0 {
		return
	}
	status := runtime.Status
	if status != models.StatusCollecting && status != models.StatusConfirming && status != models.StatusAskingOtherCustomers {
		return
	}
	if latest, err := o.repo.GetLatestFocusCustomerIDFromDialogs(ctx, runtime.SessionID); err == nil && latest != nil {
		if _, exists := runtime.PendingUpdates[latest.String()]; exists {
			runtime.FocusCustomerID = latest
			o.logger.Info("Recovered focus from dialogs", "customer_id", latest, "status", status)
		}
	}
}

// processWithRuleEngine 使用规则引擎处理，返回新的运行态
func (o *TurnOrchestrator) processWithRuleEngine(
	ctx context.Context,
	runtime *RuntimeContext,
	semanticResult *models.SemanticAnalysisResult,
	userInput string,
	userID string,
) (*RuntimeContext, error) {

	newRuntime := *runtime // 复制运行态

	// 第一步：统一确保 focus（有 PendingUpdates 时不能为空，多客户时从对话表获取正确关联）
	o.ensureFocusWhenPending(ctx, &newRuntime)

	// CONFIRMING 阶段：用户给出肯定答复时，落库当前客户并进入 ASKING_OTHER_CUSTOMERS
	if runtime.Status == models.StatusConfirming {
		confirmed, err := o.aiClient.IsUserConfirmation(ctx, userInput)
		if err != nil {
			o.logger.Error("IsUserConfirmation failed", "error", err)
		} else if confirmed {
			if err := o.saveConfirmedCustomerAndTransition(ctx, &newRuntime, userID); err != nil {
				o.logger.Error("Failed to save confirmed customer", "error", err)
				return nil, err
			}
			o.logger.Info("User confirmed, saved customer, transitioning to ASKING_OTHER_CUSTOMERS")
			return &newRuntime, nil
		}
	}

	// ASKING_OTHER_CUSTOMERS 阶段：用户表示没有其他客户时，进入 OUTPUTTING（归一与收尾）
	if runtime.Status == models.StatusAskingOtherCustomers {
		noMore, err := o.aiClient.IsUserNoMoreCustomers(ctx, userInput)
		if err != nil {
			o.logger.Error("IsUserNoMoreCustomers failed", "error", err)
		} else if noMore {
			newRuntime.Status = models.StatusOutputting
			o.logger.Info("User has no more customers, transitioning to OUTPUTTING")
			return &newRuntime, nil
		}
	}

	// 处理语义分析结果
	if semanticResult != nil {
		newRuntime.SemanticRelevance = semanticResult.SemanticRelevance

		if semanticResult.SemanticRelevance == models.SemanticStrong && len(semanticResult.CustomerRefs) > 0 {
			// 获取当前聚焦客户名（COLLECTING/ASKING 阶段用于匹配）
			focusCustomerName := ""
			if newRuntime.FocusCustomerID != nil {
				if focusC, _ := o.repo.GetCustomer(ctx, *newRuntime.FocusCustomerID); focusC != nil {
					focusCustomerName = focusC.Name
				}
			}

			for _, item := range semanticResult.CustomerRefs {
				// CONFIRMING 阶段：1) 用户确认 → 落库（见上方 IsUserConfirmation） 2) 用户修改 → 仅更新 pending_updates，待用户再次确认后才落库
				if newRuntime.Status == models.StatusConfirming {
					if newRuntime.FocusCustomerID != nil && len(item.FieldUpdates) > 0 {
						if err := o.processFieldUpdates(ctx, &newRuntime, item.FieldUpdates); err != nil {
							o.logger.Error("Failed to process CONFIRMING correction", "error", err)
						}
					}
					continue
				}
				// 用户未点名客户时，有 field_updates 则归入当前 focus
				if item.CustomerName == "" {
					if newRuntime.FocusCustomerID != nil && len(item.FieldUpdates) > 0 {
						if err := o.processFieldUpdates(ctx, &newRuntime, item.FieldUpdates); err != nil {
							o.logger.Error("Failed to process field updates for focus", "error", err)
						}
					}
					continue
				}
				// COLLECTING/ASKING：若客户名与 focus 一致，应用到 focus
				if item.CustomerName == focusCustomerName {
					if err := o.processFieldUpdates(ctx, &newRuntime, item.FieldUpdates); err != nil {
						o.logger.Error("Failed to process field updates for focus", "customer", item.CustomerName, "error", err)
					}
					continue
				}
				customerID, err := o.findOrCreateCustomer(ctx, item.CustomerName)
				if err != nil {
					o.logger.Error("Failed to find or create customer", "name", item.CustomerName, "error", err)
					continue
				}
				newRuntime.MentionedCustomerID = &customerID
				newRuntime.FocusCustomerID = &customerID
				if err := o.processFieldUpdates(ctx, &newRuntime, item.FieldUpdates); err != nil {
					o.logger.Error("Failed to process field updates", "customer", item.CustomerName, "error", err)
				}
			}
		}
	}

	// 重新计算状态
	if err := o.recalculateStates(ctx, &newRuntime); err != nil {
		return nil, err
	}

	return &newRuntime, nil
}

// processFieldUpdates 处理字段更新
func (o *TurnOrchestrator) processFieldUpdates(
	ctx context.Context,
	runtime *RuntimeContext,
	fieldUpdates map[string]interface{},
) error {
	if runtime.FocusCustomerID == nil {
		return nil
	}

	// 在 CONFIRMING 阶段，检查是否有字段修改
	if runtime.Status == models.StatusConfirming {
		if err := o.handleConfirmingStageModifications(ctx, runtime, fieldUpdates); err != nil {
			return err
		}
		return nil
	}

	// 处理当前字段写入
	if err := o.handleCurrentFieldWrite(ctx, runtime, fieldUpdates); err != nil {
		return err
	}

	// 处理其他字段到 pending_updates
	o.handlePendingUpdates(runtime, fieldUpdates)

	// 处理特殊字段（risk 和 contact_person）
	if err := o.handleSpecialFields(ctx, runtime, fieldUpdates); err != nil {
		return err
	}

	return nil
}

// saveConfirmedCustomerAndTransition 将已确认的客户落库，并从 pending 移除，切换至 ASKING_OTHER_CUSTOMERS
func (o *TurnOrchestrator) saveConfirmedCustomerAndTransition(ctx context.Context, runtime *RuntimeContext, userID string) error {
	// 注：focus 由 processWithRuleEngine 入口 ensureFocusWhenPending 统一确保
	if runtime.FocusCustomerID == nil {
		return fmt.Errorf("no focus customer to save")
	}
	customerID := *runtime.FocusCustomerID
	customerKey := customerID.String()
	data := runtime.PendingUpdates[customerKey]
	if len(data) == 0 {
		return fmt.Errorf("no pending data for customer %s", customerKey)
	}

	customer, err := o.repo.GetCustomer(ctx, customerID)
	if err != nil || customer == nil {
		return fmt.Errorf("get customer %s: %w", customerID, err)
	}

	followRecord := o.buildFollowRecordFromCollectedData(customer, data)
	if followRecord == nil {
		return fmt.Errorf("failed to build follow record for customer %s", customerKey)
	}

	followTime := o.getFirstFocusTimeForCustomer(ctx, runtime.SessionID, customerID)
	followRecord.FollowTime = followTime
	followRecord.UserID = userID
	followRecord.ID = uuid.New()

	if err := o.repo.CreateFollowRecord(ctx, followRecord); err != nil {
		return fmt.Errorf("create follow record customer=%s: %w", customerID, err)
	}

	delete(runtime.PendingUpdates, customerKey)
	runtime.FocusCustomerID = nil
	runtime.MentionedCustomerID = nil // 避免 recalculateStates 仍把已落库客户计入 customerStates
	runtime.State = models.StateCustomerName

	// 若还有待确认客户（均已 COMPLETE），recalculateStates 会设为 CONFIRMING；否则 pending 为空时进入 ASKING_OTHER_CUSTOMERS
	if err := o.recalculateStates(ctx, runtime); err != nil {
		return err
	}

	o.logger.Info("Saved confirmed customer to DB", "customer_id", customerID, "customer_name", customer.Name)
	return nil
}

// getFirstFocusTimeForCustomer 获取客户首次聚焦时间（用于 follow_time）
func (o *TurnOrchestrator) getFirstFocusTimeForCustomer(ctx context.Context, sessionID uuid.UUID, customerID uuid.UUID) time.Time {
	dialogs, err := o.repo.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return time.Now()
	}
	for _, dialog := range dialogs {
		if dialog.FocusCustomerID != nil && *dialog.FocusCustomerID == customerID && dialog.IsFirstFocus {
			return dialog.CreatedAt
		}
	}
	return time.Now()
}

// handleConfirmingStageModifications 处理 CONFIRMING 阶段的字段修改
func (o *TurnOrchestrator) handleConfirmingStageModifications(
	ctx context.Context,
	runtime *RuntimeContext,
	fieldUpdates map[string]interface{},
) error {
	for fieldName, value := range fieldUpdates {
		if fieldName == "risk" || fieldName == "contact_person" {
			continue
		}
		// 检测到字段修改，执行回退逻辑
		if value != nil {
			return o.handleFieldModificationInConfirming(ctx, runtime, fieldName, value)
		}
	}
	return nil
}

// handleCurrentFieldWrite 处理当前字段的写入（存入 pending_updates）
func (o *TurnOrchestrator) handleCurrentFieldWrite(
	ctx context.Context,
	runtime *RuntimeContext,
	fieldUpdates map[string]interface{},
) error {
	currentField := o.getExpectedField(runtime.State)
	customerKey := runtime.FocusCustomerID.String()

	// 优先使用本轮提取的值，否则使用 pending 中已有的值
	var value interface{}
	if v, exists := fieldUpdates[currentField]; exists && v != nil {
		value = v
	} else if runtime.PendingUpdates[customerKey] != nil {
		if v, exists := runtime.PendingUpdates[customerKey][currentField]; exists && v != nil {
			value = v
		}
	}
	if value != nil {
		return o.writeFieldToRuntime(ctx, runtime, *runtime.FocusCustomerID, currentField, value)
	}
	return nil
}

// handlePendingUpdates 将非当前状态字段存入 pending_updates
func (o *TurnOrchestrator) handlePendingUpdates(
	runtime *RuntimeContext,
	fieldUpdates map[string]interface{},
) {
	if runtime.FocusCustomerID == nil {
		return
	}
	customerKey := runtime.FocusCustomerID.String()
	if runtime.PendingUpdates[customerKey] == nil {
		runtime.PendingUpdates[customerKey] = make(map[string]interface{})
	}

	currentField := o.getExpectedField(runtime.State)

	for fieldName, value := range fieldUpdates {
		if fieldName != currentField && fieldName != "risk" && fieldName != "contact_person" && value != nil {
			runtime.PendingUpdates[customerKey][fieldName] = value
		}
	}
}

// handleSpecialFields 处理特殊字段（risk 和 contact_person）
func (o *TurnOrchestrator) handleSpecialFields(
	ctx context.Context,
	runtime *RuntimeContext,
	fieldUpdates map[string]interface{},
) error {
	// 处理风险字段
	if riskValue, exists := fieldUpdates["risk"]; exists && riskValue != nil {
		if o.ruleEngine.CanWriteRisk(runtime.SemanticRelevance, runtime.State) {
			if err := o.writeFieldToRuntime(ctx, runtime, *runtime.FocusCustomerID, "risk", riskValue); err != nil {
				return err
			}
		}
	}

	// 处理联系人字段（合并 contact_person、contact_role、contact_phone）
	hasContact := false
	contactInfo := make(map[string]interface{})
	if v, exists := fieldUpdates["contact_person"]; exists && v != nil {
		contactInfo["name"] = v
		hasContact = true
	}
	if v, exists := fieldUpdates["contact_role"]; exists && v != nil {
		contactInfo["role"] = v
		hasContact = true
	}
	if v, exists := fieldUpdates["contact_phone"]; exists && v != nil {
		contactInfo["phone"] = v
		hasContact = true
	}
	if hasContact {
		if err := o.writeContactPerson(ctx, *runtime.FocusCustomerID, contactInfo); err != nil {
			o.logger.Error("Failed to write contact person", "error", err)
		}
	}

	return nil
}

// recalculateStates 重新计算状态
func (o *TurnOrchestrator) recalculateStates(ctx context.Context, runtime *RuntimeContext) error {
	o.logger.Debug("Recalculating states", "runtime", runtime)

	// 获取所有客户的状态（COLLECTING/CONFIRMING 使用 collected_follow_data，OUTPUTTING 使用 DB）
	customerStates, err := o.getAllCustomerStates(ctx, runtime)
	if err != nil {
		return err
	}

	// 记录之前的聚焦客户
	previousFocusCustomerID := runtime.FocusCustomerID

	// 选择聚焦客户
	runtime.FocusCustomerID = o.ruleEngine.SelectFocusCustomer(
		runtime.FocusCustomerID,
		runtime.MentionedCustomerID,
		customerStates,
		runtime.Status,
	)

	// 根因修复：有 PendingUpdates 表示未结束客户对话，FocusCustomerID 永远不能为空
	if runtime.FocusCustomerID == nil && len(runtime.PendingUpdates) > 0 {
		if len(customerStates) > 0 {
			// 用 state 顺序选取（与 SelectFocusCustomer 一致），对 customerIDs 排序以保证确定性
			stateOrder := []string{
				models.StateCustomerName, models.StateFollowContent, models.StateFollowGoal,
				models.StateFollowResult, models.StateNextPlan, models.StateFollowMethod, models.StateComplete,
			}
			customerIDsSorted := make([]uuid.UUID, 0, len(customerStates))
			for id := range customerStates {
				customerIDsSorted = append(customerIDsSorted, id)
			}
			sort.Slice(customerIDsSorted, func(i, j int) bool {
				return customerIDsSorted[i].String() < customerIDsSorted[j].String()
			})
			for _, targetState := range stateOrder {
				for _, customerID := range customerIDsSorted {
					if customerStates[customerID] == targetState {
						runtime.FocusCustomerID = &customerID
						break
					}
				}
				if runtime.FocusCustomerID != nil {
					break
				}
			}
		}
		// 兜底：SelectFocusCustomer 可能返回 nil，复用统一恢复逻辑
		if runtime.FocusCustomerID == nil && len(runtime.PendingUpdates) > 0 {
			o.ensureFocusWhenPending(ctx, runtime)
		}
	}

	// 检查是否是首次聚焦该客户
	if runtime.FocusCustomerID != nil {
		if previousFocusCustomerID == nil || *previousFocusCustomerID != *runtime.FocusCustomerID {
			// 检查该客户是否之前被聚焦过
			isFirstFocus, err := o.isFirstFocusForCustomer(ctx, runtime.SessionID, *runtime.FocusCustomerID)
			if err != nil {
				o.logger.Error("Failed to check first focus", "error", err)
			} else {
				runtime.IsFirstFocus = isFirstFocus
			}
		}
	}

	// 重新计算当前客户状态（COLLECTING/CONFIRMING 使用 pending_updates）
	if runtime.FocusCustomerID != nil {
		customer, _ := o.repo.GetCustomer(ctx, *runtime.FocusCustomerID)
		var followRecord *models.FollowRecord
		if runtime.Status == models.StatusCollecting || runtime.Status == models.StatusConfirming || runtime.Status == models.StatusAskingOtherCustomers {
			followRecord = o.buildFollowRecordFromCollectedData(customer, runtime.PendingUpdates[runtime.FocusCustomerID.String()])
		} else {
			followRecord, _ = o.repo.GetLatestFollowRecord(ctx, *runtime.FocusCustomerID)
		}
		runtime.State = o.ruleEngine.DetermineState(customer, followRecord)
	}

	// 重新计算会话状态
	newStatus := o.ruleEngine.DetermineStatus(customerStates)
	// CONFIRMING 阶段用户提出修改后回到 COLLECTING，当全部客户再次 COMPLETE 时应直接回到 CONFIRMING 而非 ASKING_OTHER_CUSTOMERS
	// 仅当仍有待处理客户（customerStates 非空）时才应用此覆盖；若已无客户（刚确认落库最后一个），应进入 ASKING_OTHER_CUSTOMERS
	if newStatus == models.StatusAskingOtherCustomers && runtime.PendingReconfirm && len(customerStates) > 0 {
		runtime.Status = models.StatusConfirming
		runtime.PendingReconfirm = false
		o.logger.Info("Returning to CONFIRMING after modification completion")
	} else {
		runtime.Status = newStatus
		if newStatus == models.StatusAskingOtherCustomers {
			runtime.PendingReconfirm = false
		}
	}

	return nil
}

// generateReply 生成回复
func (o *TurnOrchestrator) generateReply(ctx context.Context, runtime *RuntimeContext, userInput string) (string, error) {
	// OUTPUTTING 阶段：直接返回确认文案，无需调用对话模型
	if runtime.Status == models.StatusOutputting {
		return o.outputtingConfirm, nil
	}

	// ASKING_OTHER_CUSTOMERS 阶段：固定追问文案，无需调用对话模型
	if runtime.Status == models.StatusAskingOtherCustomers {
		return o.askingOtherCustomers, nil
	}

	focusCustomerName := ""
	if runtime.FocusCustomerID != nil {
		customer, _ := o.repo.GetCustomer(ctx, *runtime.FocusCustomerID)
		if customer != nil {
			focusCustomerName = customer.Name
		}
	}

	// 获取原始对话历史，供大模型理解上下文
	conversationHistory, _ := o.repo.GetSessionConversationHistory(ctx, runtime.SessionID, runtime.TurnIndex)

	// 构建历史上下文和摘要
	historyContext := ""
	summary := ""

	if runtime.Status == models.StatusConfirming {
		// 构建完整的跟进信息 JSON，供大模型完整复述（含客户名、联系人、跟进时间、跟进方式、内容、目标、结果、风险、下一步计划）
		if runtime.FocusCustomerID != nil {
			displayData := o.buildConfirmationDisplayData(ctx, runtime, *runtime.FocusCustomerID)
			if len(displayData) > 0 {
				historyBytes, _ := json.Marshal(displayData)
				historyContext = string(historyBytes)
			}
		}
	} else if (runtime.Status == models.StatusCollecting || runtime.Status == models.StatusAskingOtherCustomers) && runtime.FocusCustomerID != nil {
		// 在收集阶段，为当前聚焦客户生成摘要（从 pending_updates）
		customer, _ := o.repo.GetCustomer(ctx, *runtime.FocusCustomerID)
		followRecord := o.buildFollowRecordFromCollectedData(customer, runtime.PendingUpdates[runtime.FocusCustomerID.String()])
		if followRecord != nil || len(runtime.PendingUpdates[runtime.FocusCustomerID.String()]) > 0 {
			summaryData := make(map[string]interface{})
			if customer != nil {
				summaryData["customer_name"] = customer.Name
			}
			if followRecord != nil {
				if followRecord.FollowMethod != nil {
					summaryData["follow_method"] = *followRecord.FollowMethod
				}
				if followRecord.FollowContent != nil {
					summaryData["follow_content"] = *followRecord.FollowContent
				}
				if followRecord.FollowGoal != nil {
					summaryData["follow_goal"] = *followRecord.FollowGoal
				}
				if followRecord.FollowResult != nil {
					summaryData["follow_result"] = *followRecord.FollowResult
				}
				if followRecord.NextPlan != nil {
					summaryData["next_plan"] = *followRecord.NextPlan
				}
				if followRecord.RiskContent != nil {
					summaryData["risk_content"] = *followRecord.RiskContent
				}
			}

			if len(summaryData) > 0 {
				summaryBytes, _ := json.Marshal(summaryData)
				summary, _ = o.aiClient.SummarizeCustomerInfo(ctx, string(summaryBytes))
			}
		}
	}

	expectedField := o.getExpectedInfo(runtime.State)

	return o.aiClient.GenerateDialogue(ctx, runtime.Status, focusCustomerName, expectedField, userInput, historyContext, summary, conversationHistory)
}

// saveRuntimeSnapshot 保存运行态快照（含用户输入与助手回复，供后续大模型理解对话上下文）
func (o *TurnOrchestrator) saveRuntimeSnapshot(ctx context.Context, sessionID uuid.UUID, runtime *RuntimeContext, userInput, assistantReply string) error {
	snapshot := models.RuntimeState{
		SessionID:        sessionID,
		FocusCustomerID:  runtime.FocusCustomerID,
		State:            runtime.State,
		Status:           runtime.Status,
		PendingUpdates:   runtime.PendingUpdates,
		PendingReconfirm: runtime.PendingReconfirm,
	}

	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	var pendingBytes []byte
	if len(runtime.PendingUpdates) > 0 {
		pendingBytes, _ = json.Marshal(runtime.PendingUpdates) // map[customer_id]map[field]value
	} else {
		pendingBytes = []byte("{}")
	}

	dialog := &models.Dialog{
		ID:                uuid.New(),
		SessionID:         sessionID,
		State:             runtime.State,
		Status:            runtime.Status,
		TurnIndex:         runtime.TurnIndex,
		FocusCustomerID:   runtime.FocusCustomerID,
		IsFirstFocus:      runtime.IsFirstFocus,
		SemanticRelevance: &runtime.SemanticRelevance,
		PendingUpdates:    pendingBytes,
		RuntimeSnapshot:   snapshotBytes,
	}
	if userInput != "" || assistantReply != "" {
		tc := "User: " + userInput + "\nAssistant: " + assistantReply
		dialog.TurnContent = &tc
	}

	return o.repo.CreateDialog(ctx, dialog)
}

// getExpectedField 获取期望收集的字段
func (o *TurnOrchestrator) getExpectedField(state string) string {
	return models.GetFieldByState(state)
}

// getExpectedInfo 获取期望收集的信息
func (o *TurnOrchestrator) getExpectedInfo(state string) string {
	return models.GetExpectedInfo(state)
}

func (o *TurnOrchestrator) findOrCreateCustomer(ctx context.Context, name string) (uuid.UUID, error) {
	customer, err := o.repo.GetCustomerByName(ctx, name)
	if err != nil {
		return uuid.Nil, err
	}

	if customer != nil {
		return customer.ID, nil
	}

	// 创建新客户
	newCustomer := &models.Customer{
		ID:   uuid.New(),
		Name: name,
	}

	if err := o.repo.CreateCustomer(ctx, newCustomer); err != nil {
		return uuid.Nil, err
	}

	return newCustomer.ID, nil
}

// writeFieldToRuntime 将字段写入 pending_updates（COLLECTING/CONFIRMING 阶段，OUTPUTTING 时由 output_worker 写入 follow_records）
func (o *TurnOrchestrator) writeFieldToRuntime(ctx context.Context, runtime *RuntimeContext, customerID uuid.UUID, fieldName string, value interface{}) error {
	switch fieldName {
	case "customer_name":
		// 客户名称在创建客户时已经设置
		return nil
	case "follow_time":
		// 用户明确表达的时间，保留原样（可能为"今天下午"等自然语言）
		customerKey := customerID.String()
		if runtime.PendingUpdates[customerKey] == nil {
			runtime.PendingUpdates[customerKey] = make(map[string]interface{})
		}
		runtime.PendingUpdates[customerKey]["follow_time"] = fmt.Sprintf("%v", value)
		return nil
	case "follow_method", "follow_content", "follow_goal", "follow_result", "next_plan", "risk":
		valueStr := fmt.Sprintf("%v", value)
		dbKey := fieldName
		if fieldName == "risk" {
			dbKey = "risk_content"
		}
		customerKey := customerID.String()
		if runtime.PendingUpdates[customerKey] == nil {
			runtime.PendingUpdates[customerKey] = make(map[string]interface{})
		}
		runtime.PendingUpdates[customerKey][dbKey] = valueStr
		return nil
	default:
		o.logger.Warn("Unknown field name", "field", fieldName)
		return nil
	}
}

func (o *TurnOrchestrator) getAllCustomerStates(ctx context.Context, runtime *RuntimeContext) (map[uuid.UUID]string, error) {
	// 每聊完一个即落库：只统计仍在 pending 中的客户 + 当前 focus/mentioned（可能尚未有字段写入）
	customerStates := make(map[uuid.UUID]string)
	customerIDs := make(map[uuid.UUID]bool)
	var batchErrors []string

	for customerKey := range runtime.PendingUpdates {
		customerID, err := uuid.Parse(customerKey)
		if err != nil {
			continue
		}
		customerIDs[customerID] = true
	}
	if runtime.FocusCustomerID != nil {
		customerIDs[*runtime.FocusCustomerID] = true
	}
	if runtime.MentionedCustomerID != nil {
		customerIDs[*runtime.MentionedCustomerID] = true
	}

	for customerID := range customerIDs {
		customer, err := o.repo.GetCustomer(ctx, customerID)
		if err != nil {
			batchErrors = append(batchErrors, fmt.Sprintf("customer %s: %v", customerID, err))
			continue
		}

		// COLLECTING/CONFIRMING 使用 pending_updates，OUTPUTTING 使用 DB（但此时由 output_worker 处理）
		var followRecord *models.FollowRecord
		if runtime.Status == models.StatusCollecting || runtime.Status == models.StatusConfirming || runtime.Status == models.StatusAskingOtherCustomers {
			followRecord = o.buildFollowRecordFromCollectedData(customer, runtime.PendingUpdates[customerID.String()])
		} else {
			followRecord, err = o.repo.GetLatestFollowRecord(ctx, customerID)
			if err != nil {
				batchErrors = append(batchErrors, fmt.Sprintf("follow record %s: %v", customerID, err))
				continue
			}
		}

		state := o.ruleEngine.DetermineState(customer, followRecord)

		o.logger.Info("Determining state", "state", state, "customer_name", customer.Name, "follow_record", followRecord)

		customerStates[customerID] = state
	}

	if len(batchErrors) > 0 {
		o.logger.Warn("Batch processing completed with errors",
			"session_id", runtime.SessionID,
			"total_customers", len(customerIDs),
			"success_count", len(customerStates),
			"error_count", len(batchErrors),
			"first_error", batchErrors[0])
	}

	return customerStates, nil
}

// buildFollowRecordFromCollectedData 从 collected_follow_data 构建 FollowRecord，联系人信息从 Customer 表补充
func (o *TurnOrchestrator) buildFollowRecordFromCollectedData(customer *models.Customer, data map[string]interface{}) *models.FollowRecord {
	if customer == nil {
		return nil
	}
	r := &models.FollowRecord{
		CustomerID:   customer.ID,
		CustomerName: customer.Name,
		FollowTime:   time.Now(), // 占位，落库时由 first_focus 时间补齐
	}
	// 联系人信息从 Customer 表获取（handleSpecialFields 已通过 writeContactPerson 写入）
	if customer.ContactPerson != nil {
		r.ContactPerson = customer.ContactPerson
	}
	if customer.ContactPhone != nil {
		r.ContactPhone = customer.ContactPhone
	}
	if customer.ContactRole != nil {
		r.ContactRole = customer.ContactRole
	}
	if len(data) == 0 {
		return r
	}
	for k, v := range data {
		s := fmt.Sprintf("%v", v)
		switch k {
		case "follow_time":
			// 用户明确表达的时间（字符串），落库时若需可解析；展示时优先使用
			if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
				r.FollowTime = t
			} else if t, err := time.Parse(time.RFC3339, s); err == nil {
				r.FollowTime = t
			}
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

// buildFollowRecordsForCustomer 从 pending_updates 构建指定客户的跟进记录（供单客户确认使用）
func (o *TurnOrchestrator) buildFollowRecordsForCustomer(ctx context.Context, runtime *RuntimeContext, customerID uuid.UUID) []*models.FollowRecord {
	data := runtime.PendingUpdates[customerID.String()]
	if len(data) == 0 {
		return nil
	}
	customer, err := o.repo.GetCustomer(ctx, customerID)
	if err != nil || customer == nil {
		return nil
	}
	r := o.buildFollowRecordFromCollectedData(customer, data)
	if r == nil {
		return nil
	}
	// 补齐 follow_time（用户未明确表达时用首次聚焦时间）
	if r.FollowTime.IsZero() {
		r.FollowTime = o.getFirstFocusTimeForCustomer(ctx, runtime.SessionID, customerID)
	}
	return []*models.FollowRecord{r}
}

// buildConfirmationDisplayData 构建供大模型完整复述的清爽 JSON（仅含必要字段，便于复述）
func (o *TurnOrchestrator) buildConfirmationDisplayData(ctx context.Context, runtime *RuntimeContext, customerID uuid.UUID) map[string]interface{} {
	data := runtime.PendingUpdates[customerID.String()]
	if len(data) == 0 {
		return nil
	}
	customer, err := o.repo.GetCustomer(ctx, customerID)
	if err != nil || customer == nil {
		return nil
	}
	out := make(map[string]interface{})
	out["customer_name"] = customer.Name
	if customer.ContactPerson != nil && *customer.ContactPerson != "" {
		out["contact_person"] = *customer.ContactPerson
	}
	if customer.ContactPhone != nil && *customer.ContactPhone != "" {
		out["contact_phone"] = *customer.ContactPhone
	}
	if customer.ContactRole != nil && *customer.ContactRole != "" {
		out["contact_role"] = *customer.ContactRole
	}
	// follow_time：用户明确表达的优先，否则用首次聚焦时间
	if v, ok := data["follow_time"]; ok && v != nil && fmt.Sprintf("%v", v) != "" {
		out["follow_time"] = fmt.Sprintf("%v", v)
	} else {
		followTime := o.getFirstFocusTimeForCustomer(ctx, runtime.SessionID, customerID)
		out["follow_time"] = followTime.Format("2006-01-02")
	}
	for k, v := range data {
		if v == nil || (k == "follow_time" && fmt.Sprintf("%v", v) == "") {
			continue
		}
		switch k {
		case "follow_method", "follow_content", "follow_goal", "follow_result", "next_plan", "risk_content":
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

// buildSessionFollowRecordsFromPending 从 pending_updates 构建会话的跟进记录列表（供 CONFIRMING 阶段对话使用）
func (o *TurnOrchestrator) buildSessionFollowRecordsFromPending(ctx context.Context, runtime *RuntimeContext) []*models.FollowRecord {
	if len(runtime.PendingUpdates) == 0 {
		return nil
	}
	var records []*models.FollowRecord
	for customerKey, data := range runtime.PendingUpdates {
		if len(data) == 0 {
			continue
		}
		customerID, err := uuid.Parse(customerKey)
		if err != nil {
			continue
		}
		customer, err := o.repo.GetCustomer(ctx, customerID)
		if err != nil || customer == nil {
			continue
		}
		r := o.buildFollowRecordFromCollectedData(customer, data)
		if r != nil {
			records = append(records, r)
		}
	}
	return records
}

// writeContactPerson 写入联系人信息（支持语义分析返回的字符串或对象格式）
func (o *TurnOrchestrator) writeContactPerson(ctx context.Context, customerID uuid.UUID, contactInfo interface{}) error {
	var name, role, phone *string

	switch v := contactInfo.(type) {
	case string:
		// 语义分析返回 "contact_person": "张总" 的字符串格式
		if v != "" {
			name = &v
		}
	case map[string]interface{}:
		if n, exists := v["name"]; exists && n != nil {
			s := fmt.Sprintf("%v", n)
			name = &s
		}
		if r, exists := v["role"]; exists && r != nil {
			s := fmt.Sprintf("%v", r)
			role = &s
		}
		if p, exists := v["phone"]; exists && p != nil {
			s := fmt.Sprintf("%v", p)
			phone = &s
		}
	default:
		// 其他类型（如 float64 等 JSON 数字）转为字符串作为 name
		s := fmt.Sprintf("%v", contactInfo)
		if s != "" && s != "null" {
			name = &s
		}
	}

	if name == nil && role == nil && phone == nil {
		return nil
	}

	// 更新客户表中的联系人信息
	customer, err := o.repo.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}

	if name != nil {
		customer.ContactPerson = name
	}
	if role != nil {
		customer.ContactRole = role
	}
	if phone != nil {
		customer.ContactPhone = phone
	}

	return o.repo.UpdateCustomer(ctx, customer)
}

// handleFieldModificationInConfirming 处理 CONFIRMING 阶段的字段修改（修改 pending_updates，不写 DB）
func (o *TurnOrchestrator) handleFieldModificationInConfirming(
	ctx context.Context,
	runtime *RuntimeContext,
	modifiedField string,
	newValue interface{},
) error {
	// follow_time、risk_content 等不参与状态回退，直接更新
	if modifiedField == "follow_time" || modifiedField == "risk_content" || modifiedField == "risk" {
		customerKey := runtime.FocusCustomerID.String()
		if runtime.PendingUpdates[customerKey] == nil {
			runtime.PendingUpdates[customerKey] = make(map[string]interface{})
		}
		key := modifiedField
		if modifiedField == "risk" {
			key = "risk_content"
		}
		runtime.PendingUpdates[customerKey][key] = fmt.Sprintf("%v", newValue)
		return nil
	}
	// 使用规则引擎确定需要回退的状态和清空的字段
	newState, fieldsToClear := o.ruleEngine.HandleFieldModification(modifiedField)
	if newState == "" {
		return nil
	}

	customerKey := runtime.FocusCustomerID.String()
	if runtime.PendingUpdates[customerKey] == nil {
		runtime.PendingUpdates[customerKey] = make(map[string]interface{})
	}
	data := runtime.PendingUpdates[customerKey]

	// 清空需要重置的字段
	for _, field := range fieldsToClear {
		delete(data, field)
		if field == "risk" {
			delete(data, "risk_content")
		}
	}

	// 写入新值
	dbKey := modifiedField
	if modifiedField == "risk" {
		dbKey = "risk_content"
	}
	data[dbKey] = fmt.Sprintf("%v", newValue)

	// 更新运行态状态
	runtime.State = newState

	// 清空 pending_updates 中被重置的字段
	if runtime.PendingUpdates[customerKey] != nil {
		for _, field := range fieldsToClear {
			delete(runtime.PendingUpdates[customerKey], field)
		}
	}

	// 用户提出修改后，仅更新 pending_updates，不落库；流程回到 COLLECTING，待用户再次确认后才落库
	runtime.PendingReconfirm = true

	o.logger.Info("Field modified in CONFIRMING stage",
		"modified_field", modifiedField,
		"new_state", newState,
		"fields_cleared", fieldsToClear)

	return nil
}

// isFirstFocusForCustomer 检查是否是首次聚焦该客户
func (o *TurnOrchestrator) isFirstFocusForCustomer(ctx context.Context, sessionID uuid.UUID, customerID uuid.UUID) (bool, error) {
	dialogs, err := o.repo.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}

	for _, dialog := range dialogs {
		if dialog.FocusCustomerID != nil && *dialog.FocusCustomerID == customerID {
			return false, nil
		}
	}

	return true, nil
}

// RuntimeContext 运行时上下文
type RuntimeContext struct {
	SessionID           uuid.UUID                         `json:"session_id"`
	TurnIndex           int                               `json:"turn_index"`
	State               string                            `json:"state"`
	Status              string                            `json:"status"`
	FocusCustomerID     *uuid.UUID                        `json:"focus_customer_id,omitempty"`
	MentionedCustomerID *uuid.UUID                        `json:"mentioned_customer_id,omitempty"`
	SemanticRelevance   string                            `json:"semantic_relevance"`
	PendingUpdates      map[string]map[string]interface{} `json:"pending_updates"` // customer_id -> field -> value，用于状态推导和 OUTPUTTING 写入 follow_records
	IsFirstFocus        bool                              `json:"is_first_focus"`
	PendingReconfirm    bool                              `json:"pending_reconfirm"` // CONFIRMING 修改后回到 COLLECTING，待全部 COMPLETE 后应直接回到 CONFIRMING
}
