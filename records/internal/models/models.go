package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// User 用户模型
type User struct {
	ID        string     `db:"id" json:"id"`
	Name      string     `db:"name" json:"name"`
	Phone     *string    `db:"phone" json:"phone,omitempty"`
	OrgName   string     `db:"orgname" json:"orgname"`
	Status    int        `db:"status" json:"status"`
	AvatarURL *string    `db:"avatar_url" json:"avatar_url,omitempty"`
	StartLark *time.Time `db:"start_lark" json:"start_lark,omitempty"`
}

// Customer 客户模型
type Customer struct {
	ID            uuid.UUID `db:"id" json:"id"`
	Name          string    `db:"name" json:"name"`
	ContactPerson *string   `db:"contact_person" json:"contact_person,omitempty"`
	ContactPhone  *string   `db:"contact_phone" json:"contact_phone,omitempty"`
	ContactRole   *string   `db:"contact_role" json:"contact_role,omitempty"`
}

// CustomerContact 客户联系人模型
type CustomerContact struct {
	ID            uuid.UUID `db:"id" json:"id"`
	CustomerID    uuid.UUID `db:"customer_id" json:"customer_id"`
	ContactPerson *string   `db:"contact_person" json:"contact_person,omitempty"`
	ContactPhone  *string   `db:"contact_phone" json:"contact_phone,omitempty"`
	ContactRole   *string   `db:"contact_role" json:"contact_role,omitempty"`
	Status        int       `db:"status" json:"status"`
}

// Session 会话模型
type Session struct {
	ID      uuid.UUID  `db:"id" json:"id"`
	UserID  string     `db:"user_id" json:"user_id"`
	Status  string     `db:"status" json:"status"`
	EndedAt *time.Time `db:"ended_at" json:"ended_at,omitempty"`
}

// Dialog 对话模型
type Dialog struct {
	ID                uuid.UUID       `db:"id" json:"id"`
	SessionID         uuid.UUID       `db:"session_id" json:"session_id"`
	State             string          `db:"state" json:"state"`
	Status            string          `db:"status" json:"status"`
	TurnIndex         int             `db:"turn_index" json:"turn_index"`
	FocusCustomerID   *uuid.UUID      `db:"focus_customer_id" json:"focus_customer_id,omitempty"`
	IsFirstFocus      bool            `db:"is_first_focus" json:"is_first_focus"`
	SemanticRelevance *string         `db:"semantic_relevance" json:"semantic_relevance,omitempty"`
	PendingUpdates    json.RawMessage `db:"pending_updates" json:"pending_updates,omitempty"`
	RuntimeSnapshot   json.RawMessage `db:"runtime_snapshot" json:"runtime_snapshot"`
	TurnContent       *string         `db:"turn_content" json:"turn_content,omitempty"` // 本轮对话：User: …\nAssistant: …
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
}

// FollowRecord 跟进记录模型
type FollowRecord struct {
	ID            uuid.UUID `db:"id" json:"id"`
	UserID        string    `db:"user_id" json:"user_id"`
	CustomerID    uuid.UUID `db:"customer_id" json:"customer_id"`
	CustomerName  string    `db:"customer_name" json:"customer_name"`
	ContactPerson *string   `db:"contact_person" json:"contact_person,omitempty"`
	ContactPhone  *string   `db:"contact_phone" json:"contact_phone,omitempty"`
	ContactRole   *string   `db:"contact_role" json:"contact_role,omitempty"`
	FollowTime    time.Time `db:"follow_time" json:"follow_time"`
	FollowMethod  *string   `db:"follow_method" json:"follow_method,omitempty"`
	FollowContent *string   `db:"follow_content" json:"follow_content,omitempty"`
	FollowGoal    *string   `db:"follow_goal" json:"follow_goal,omitempty"`
	FollowResult  *string   `db:"follow_result" json:"follow_result,omitempty"`
	RiskContent   *string   `db:"risk_content" json:"risk_content,omitempty"`
	NextPlan      *string   `db:"next_plan" json:"next_plan,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// RuntimeState 运行态状态
type RuntimeState struct {
	SessionID       uuid.UUID  `json:"session_id"`
	FocusCustomerID *uuid.UUID `json:"focus_customer_id,omitempty"`
	State           string     `json:"state"`
	Status          string     `json:"status"`
	// PendingUpdates: customer_id -> field -> value，存储 COLLECTING/CONFIRMING 阶段收集的所有跟进字段，OUTPUTTING 时写入 follow_records
	PendingUpdates map[string]map[string]interface{} `json:"pending_updates,omitempty"`
	// PendingReconfirm: CONFIRMING 阶段用户提出修改后，回到 COLLECTING 修改信息，待全部客户 COMPLETE 后应直接回到 CONFIRMING 而非 ASKING_OTHER_CUSTOMERS
	PendingReconfirm bool `json:"pending_reconfirm,omitempty"`
}

// SemanticAnalysisResult 语义分析结果（支持多客户）
type SemanticAnalysisResult struct {
	SemanticRelevance string            `json:"semantic_relevance"`
	CustomerRefs      []CustomerRefItem `json:"customer_refs"`
}

// CustomerRefItem 单个客户的引用及其跟进信息
type CustomerRefItem struct {
	CustomerName string                 `json:"customer_name"`
	FieldUpdates map[string]interface{} `json:"field_updates,omitempty"`
}

// CustomerRef 客户引用
type CustomerRef struct {
	Mentioned    bool    `json:"mentioned"`
	CustomerName *string `json:"customer_name,omitempty"`
}

// ContactInfo 联系人信息
type ContactInfo struct {
	Name  *string `json:"name,omitempty"`
	Role  *string `json:"role,omitempty"`
	Phone *string `json:"phone,omitempty"`
}

// EntityMention 实体提及（客户/联系人在对话中的提及）
type EntityMention struct {
	MentionID    string `json:"mention_id"`
	EntityType   string `json:"entity_type"` // "customer" or "contact"
	Name         string `json:"name"`
	CustomerName string `json:"customer_name,omitempty"` // 联系人所属客户
	Context      string `json:"context,omitempty"`
}

// CandidateEntity 候选实体（系统中已有的客户/联系人）
type CandidateEntity struct {
	EntityID     string  `json:"entity_id"`
	EntityType   string  `json:"entity_type"` // "customer" or "contact"
	Name         string  `json:"name"`
	CustomerName string  `json:"customer_name,omitempty"` // 联系人所属客户
	ContactRole  *string `json:"contact_role,omitempty"`
	ContactPhone *string `json:"contact_phone,omitempty"`
}

// NormalizationEvidence 归一证据
type NormalizationEvidence struct {
	NameMatch  float64 `json:"name_match"`
	Context    float64 `json:"context"`
	Attributes float64 `json:"attributes"`
	History    float64 `json:"history"`
}

// NormalizationResult 归一结果
type NormalizationResult struct {
	MentionID          string                `json:"mention_id"`
	EntityID           *string               `json:"entity_id"`
	NormalizationScore float64               `json:"normalization_score"`
	NormalizationLevel string                `json:"normalization_level"` // "high" | "medium" | "low" | "none"
	Evidence           NormalizationEvidence `json:"evidence"`
	NeedsConfirmation  bool                  `json:"needs_confirmation"`
}

// NormalizationRequest 归一请求
type NormalizationRequest struct {
	DialogContext     string            `json:"dialog_context"`
	MentionsEntity    []EntityMention   `json:"mentions_entity"`
	CandidateEntities []CandidateEntity `json:"candidate_entities"`
}

// 状态枚举
const (
	// 客户状态
	StateCustomerName  = "CUSTOMER_NAME"
	StateFollowMethod  = "FOLLOW_METHOD"
	StateFollowContent = "FOLLOW_CONTENT"
	StateFollowGoal    = "FOLLOW_GOAL"
	StateFollowResult  = "FOLLOW_RESULT"
	StateNextPlan      = "NEXT_PLAN"
	StateComplete      = "COMPLETE"

	// 会话阶段
	StatusCollecting           = "COLLECTING"
	StatusAskingOtherCustomers = "ASKING_OTHER_CUSTOMERS" // 全部客户 COMPLETE 后，追问是否还有其他客户
	StatusConfirming           = "CONFIRMING"
	StatusOutputting           = "OUTPUTTING"
	StatusExit                 = "EXIT"

	// 语义相关性
	SemanticStrong = "STRONG"
	SemanticNone   = "NONE"
)

// StateFieldMap 状态到字段的映射
var StateFieldMap = map[string]string{
	StateCustomerName:  "customer_name",
	StateFollowMethod:  "follow_method",
	StateFollowContent: "follow_content",
	StateFollowGoal:    "follow_goal",
	StateFollowResult:  "follow_result",
	StateNextPlan:      "next_plan",
}

// GetFieldByState 根据状态获取对应的字段名
func GetFieldByState(state string) string {
	return StateFieldMap[state]
}

// StateFieldMap 状态到字段的映射
var StateInfoMap = map[string]string{
	StateCustomerName:  "客户名称",
	StateFollowMethod:  "跟进方式（线上/线下）",
	StateFollowContent: "跟进事项/项目",
	StateFollowGoal:    "跟进期望达到的目标",
	StateFollowResult:  "跟进实际达到的结果",
	StateNextPlan:      "跟进后下一步的计划",
}

// GetExpectedField 获取期望收集的字段
func GetExpectedInfo(state string) string {
	return StateInfoMap[state]
}
