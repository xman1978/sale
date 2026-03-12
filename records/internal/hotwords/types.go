package hotwords

import (
	"time"

	"github.com/google/uuid"
)

// ExtractedPayload LLM 返回的四类关键词（与设计文档 JSON 一致）
type ExtractedPayload struct {
	Products             []TermCount `json:"products"`
	BusinessRequirements []TermCount `json:"business_requirements"`
	PainPoints           []TermCount `json:"pain_points"`
	TransactionFriction  []TermCount `json:"transaction_friction"`
}

// TermCount 关键词及出现次数（单条日志内）
type TermCount struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}

// Category 分类常量
const (
	CategoryProducts             = "products"
	CategoryBusinessRequirements = "business_requirements"
	CategoryPainPoints           = "pain_points"
	CategoryTransactionFriction  = "transaction_friction"
)

// KeywordRecord 写入 sales_keyword_records 的一行
type KeywordRecord struct {
	ID         uuid.UUID `db:"id"`
	Category   string    `db:"category"`
	Term       string    `db:"term"`
	Count      int       `db:"count"`
	CreateTime time.Time `db:"create_time"`
}

// StatsRow 统计结果表一行
type StatsRow struct {
	ID             uuid.UUID `db:"id"`
	RunTime        time.Time `db:"run_time"`
	TimeWindowDays int       `db:"time_window_days"`
	Category       string    `db:"category"`
	Term           string    `db:"term"`
	Frequency      int       `db:"frequency"`
	Rank           int       `db:"rank"`
	CreateTime     time.Time `db:"create_time"`
}

// TimeWindowDays 支持的统计时间窗口（天）
var TimeWindowDays = []int{7, 30, 90}
