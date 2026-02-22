package engine

import (
	"fmt"

	"records/internal/models"
	"records/pkg/logger"

	"github.com/google/uuid"
)

// RuleEngine 规则引擎
type RuleEngine struct {
	logger logger.Logger
}

// NewRuleEngine 创建规则引擎
func NewRuleEngine(logger logger.Logger) *RuleEngine {
	return &RuleEngine{
		logger: logger,
	}
}

// DetermineState 根据客户数据确定状态
func (e *RuleEngine) DetermineState(customer *models.Customer, followRecord *models.FollowRecord) string {
	if customer == nil {
		return models.StateCustomerName
	}

	if followRecord == nil {
		return models.StateFollowContent
	}

	if followRecord.FollowContent == nil || *followRecord.FollowContent == "" {
		return models.StateFollowContent
	}

	if followRecord.FollowGoal == nil || *followRecord.FollowGoal == "" {
		return models.StateFollowGoal
	}

	if followRecord.FollowResult == nil || *followRecord.FollowResult == "" {
		return models.StateFollowResult
	}

	if followRecord.NextPlan == nil || *followRecord.NextPlan == "" {
		return models.StateNextPlan
	}

	if followRecord.FollowMethod == nil || *followRecord.FollowMethod == "" {
		return models.StateFollowMethod
	}

	return models.StateComplete
}

// DetermineStatus 根据所有客户状态确定会话状态
// 每聊完一个客户即确认落库：all complete → CONFIRMING；empty → ASKING_OTHER_CUSTOMERS（追问是否还有其他客户）
func (e *RuleEngine) DetermineStatus(customerStates map[uuid.UUID]string) string {
	e.logger.Debug("Determining status", "customer_states", customerStates)

	if len(customerStates) == 0 {
		// 无待收集客户（已全部确认落库）→ 追问是否还有其他客户
		return models.StatusAskingOtherCustomers
	}

	// 如果存在任一客户 state ≠ COMPLETE → COLLECTING
	for _, state := range customerStates {
		if state != models.StateComplete {
			return models.StatusCollecting
		}
	}

	// 全部客户 state = COMPLETE → CONFIRMING（确认当前客户后落库，再追问是否还有其他客户）
	return models.StatusConfirming
}

// SelectFocusCustomer 选择聚焦客户
func (e *RuleEngine) SelectFocusCustomer(
	currentFocus *uuid.UUID,
	mentionedCustomer *uuid.UUID,
	customerStates map[uuid.UUID]string,
	status string,
) *uuid.UUID {
	// 在 CONFIRMING 阶段，仅在用户显式点名时才允许变更
	if status == models.StatusConfirming {
		if mentionedCustomer != nil {
			return mentionedCustomer
		}
		return currentFocus
	}

	// 在 ASKING_OTHER_CUSTOMERS 阶段，用户提及新客户时切换聚焦
	if status == models.StatusAskingOtherCustomers && mentionedCustomer != nil {
		return mentionedCustomer
	}

	// 优先级顺序：
	// 1. 本轮用户明确点名的客户
	if mentionedCustomer != nil {
		return mentionedCustomer
	}

	// 2. 上一轮 focus_customer 且未 COMPLETE
	if currentFocus != nil {
		if state, exists := customerStates[*currentFocus]; exists && state != models.StateComplete {
			return currentFocus
		}
	}

	// 3. state 顺序最靠前的客户
	stateOrder := []string{
		models.StateCustomerName,
		models.StateFollowContent,
		models.StateFollowGoal,
		models.StateFollowResult,
		models.StateNextPlan,
		models.StateFollowMethod,
	}

	for _, targetState := range stateOrder {
		for customerID, state := range customerStates {
			if state == targetState {
				return &customerID
			}
		}
	}

	return nil
}

// CanWriteField 判断是否可以写入字段
func (e *RuleEngine) CanWriteField(
	semanticRelevance string,
	currentState string,
	fieldName string,
) bool {
	if semanticRelevance != models.SemanticStrong {
		return false
	}

	// 检查字段是否与当前状态对应
	expectedField := models.GetFieldByState(currentState)
	if expectedField == "" {
		return false
	}

	return fieldName == expectedField
}

// CanWriteRisk 判断是否可以写入风险字段
func (e *RuleEngine) CanWriteRisk(
	semanticRelevance string,
	currentState string,
) bool {
	if semanticRelevance != models.SemanticStrong {
		return false
	}

	// risk 仅允许在 FOLLOW_RESULT 或 NEXT_PLAN 状态下写入
	return currentState == models.StateFollowResult || currentState == models.StateNextPlan
}

// ProcessPendingUpdates 处理暂存更新
func (e *RuleEngine) ProcessPendingUpdates(
	pendingUpdates map[string]interface{},
	currentState string,
	newFieldUpdates map[string]interface{},
) map[string]interface{} {
	if pendingUpdates == nil {
		pendingUpdates = make(map[string]interface{})
	}

	// 获取当前状态对应的字段
	currentField := models.GetFieldByState(currentState)

	// 处理新的字段更新
	for fieldName, value := range newFieldUpdates {
		if fieldName == currentField {
			// 当前状态对应的字段不进入 pending_updates
			continue
		}

		// 其他字段进入 pending_updates
		if value != nil {
			pendingUpdates[fieldName] = value
		}
	}

	return pendingUpdates
}

// GetFieldFromPending 从暂存中获取字段值
func (e *RuleEngine) GetFieldFromPending(
	pendingUpdates map[string]interface{},
	fieldName string,
) interface{} {
	if pendingUpdates == nil {
		return nil
	}

	return pendingUpdates[fieldName]
}

// RemoveFromPending 从暂存中移除字段
func (e *RuleEngine) RemoveFromPending(
	pendingUpdates map[string]interface{},
	fieldName string,
) map[string]interface{} {
	if pendingUpdates == nil {
		return nil
	}

	delete(pendingUpdates, fieldName)
	return pendingUpdates
}

// ValidateStateTransition 验证状态转换
func (e *RuleEngine) ValidateStateTransition(
	fromState, toState string,
	status string,
) error {
	// 在 COLLECTING 阶段，state 不允许回退
	if status == models.StatusCollecting {
		stateOrder := map[string]int{
			models.StateCustomerName:  0,
			models.StateFollowMethod:  1,
			models.StateFollowContent: 2,
			models.StateFollowGoal:    3,
			models.StateFollowResult:  4,
			models.StateNextPlan:      5,
			models.StateComplete:      6,
		}

		fromOrder, fromExists := stateOrder[fromState]
		toOrder, toExists := stateOrder[toState]

		if !fromExists || !toExists {
			return fmt.Errorf("invalid state: from=%s, to=%s", fromState, toState)
		}

		if toOrder < fromOrder {
			return fmt.Errorf("state cannot go backward in COLLECTING stage: from=%s, to=%s", fromState, toState)
		}
	}

	return nil
}

// HandleFieldModification 处理 CONFIRMING 阶段的字段修改
// 返回需要回退到的状态；修改任一字段时仅更新该字段，不清空其他字段
func (e *RuleEngine) HandleFieldModification(
	modifiedField string,
) (newState string, fieldsToReset []string) {
	// 字段顺序：follow_method 在 follow_content 之后
	fieldStateMap := map[string]string{
		"customer_name":  models.StateCustomerName,
		"follow_content": models.StateFollowContent,
		"follow_goal":    models.StateFollowGoal,
		"follow_result":  models.StateFollowResult,
		"next_plan":      models.StateNextPlan,
		"follow_method":  models.StateFollowMethod,
	}

	newState = fieldStateMap[modifiedField]
	// 不清空任何其他字段，仅更新被修改字段
	return newState, nil
}

// ClearFollowRecordFields 清空跟进记录的指定字段
func (e *RuleEngine) ClearFollowRecordFields(
	followRecord *models.FollowRecord,
	fieldsToClear []string,
) {
	for _, field := range fieldsToClear {
		switch field {
		case "follow_method":
			followRecord.FollowMethod = nil
		case "follow_content":
			followRecord.FollowContent = nil
		case "follow_goal":
			followRecord.FollowGoal = nil
		case "follow_result":
			followRecord.FollowResult = nil
		case "next_plan":
			followRecord.NextPlan = nil
		}
	}
}
