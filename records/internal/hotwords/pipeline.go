package hotwords

import (
	"context"
	"fmt"
	"strings"
	"time"

	"records/pkg/logger"

	"github.com/jmoiron/sqlx"
)

// PipelineConfig 流水线配置
type PipelineConfig struct {
	BatchSize        int        // 每批送 LLM 的日志条数，建议 20
	LimitPerCategory int        // 每分类保留前 N 个热词
	PagesDir         string     // 静态页输出目录，如 records/pages
	IncrementalSince *time.Time // 若非空则覆盖 ListFollowLogsForExtract 的 cutoff，只处理该时间之后新增的 follow_records
}

// Pipeline 热词流水线
type Pipeline struct {
	db        *sqlx.DB
	extractor *Extractor
	cfg       PipelineConfig
	log       logger.Logger
}

// NewPipeline 创建流水线
func NewPipeline(db *sqlx.DB, extractor *Extractor, cfg PipelineConfig, log logger.Logger) *Pipeline {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.LimitPerCategory <= 0 {
		cfg.LimitPerCategory = defaultLimitPerCategory
	}
	return &Pipeline{db: db, extractor: extractor, cfg: cfg, log: log}
}

// Run 执行全流程：抽取日志 -> LLM 抽取 -> 同义词归一 -> 写关键词表 -> 统计 -> 写统计表 -> 生成 H5
func (p *Pipeline) Run(ctx context.Context) error {
	repo := NewRepo(p.db)
	synonyms, err := repo.LoadSynonyms(ctx)
	if err != nil {
		return fmt.Errorf("load synonyms: %w", err)
	}
	p.log.Info("hotwords pipeline: loaded synonyms", "count", len(synonyms))

	logs, err := repo.ListFollowLogsForExtract(ctx, p.cfg.IncrementalSince)
	if err != nil {
		return fmt.Errorf("list follow logs: %w", err)
	}
	p.log.Info("hotwords pipeline: follow logs to process", "total", len(logs))

	now := time.Now()
	var totalInserted int
	for i := 0; i < len(logs); i += p.cfg.BatchSize {
		end := i + p.cfg.BatchSize
		if end > len(logs) {
			end = len(logs)
		}
		batch := logs[i:end]
		payload, err := p.extractor.Extract(ctx, p.buildBatchText(batch))
		if err != nil {
			return fmt.Errorf("extract batch %d: %w", i/p.cfg.BatchSize, err)
		}
		records := p.payloadToRecords(payload, synonyms, now)
		if err := repo.InsertKeywordRecords(ctx, records); err != nil {
			return fmt.Errorf("insert keyword records: %w", err)
		}
		totalInserted += len(records)
	}
	p.log.Info("hotwords pipeline: keyword records inserted", "count", totalInserted)

	runTime := time.Now()
	if err := BuildAndPersistStats(ctx, p.db, runTime, p.cfg.LimitPerCategory); err != nil {
		return fmt.Errorf("build and persist stats: %w", err)
	}
	p.log.Info("hotwords pipeline: stats persisted", "run_time", runTime)
	// 热词展示由 records/pages/hot_words.html 动态页通过 API 拉取每日统计
	return nil
}

func (p *Pipeline) buildBatchText(batch []FollowLogForExtract) string {
	var b strings.Builder
	for i, log := range batch {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("---\n")
		b.WriteString(log.Text)
	}
	return b.String()
}

// payloadToRecords 将 LLM 返回的 payload 转为关键词记录。
func (p *Pipeline) payloadToRecords(payload *ExtractedPayload, synonyms map[string]string, createTime time.Time) []KeywordRecord {
	var out []KeywordRecord
	add := func(category string, terms []TermCount) {
		for _, tc := range terms {
			if tc.Term == "" {
				continue
			}
			term := NormalizeTerm(strings.TrimSpace(tc.Term), synonyms)
			if term == "" {
				continue
			}
			out = append(out, KeywordRecord{
				Category:   category,
				Term:       term,
				CreateTime: createTime,
				Count:      tc.Count,
			})
		}
	}

	add(CategoryProducts, payload.Products)
	add(CategoryBusinessRequirements, payload.BusinessRequirements)
	add(CategoryPainPoints, payload.PainPoints)
	add(CategoryTransactionFriction, payload.TransactionFriction)

	return out
}
