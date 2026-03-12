package hotwords

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const defaultLimitPerCategory = 50

// Repo 热词相关数据访问
type Repo struct {
	db *sqlx.DB
}

// NewRepo 创建热词 Repo
func NewRepo(db *sqlx.DB) *Repo {
	return &Repo{db: db}
}

// FollowLogForExtract 单条跟进日志（用于热词抽取）
type FollowLogForExtract struct {
	ID   uuid.UUID `db:"id"`
	Text string    `db:"log_text"`
}

// ListFollowLogsForExtract 获取待抽取的跟进日志：仅 created_at 在 cutoff 之后的记录。若 since 非空则 cutoff = since；否则 cutoff = sales_keyword_records 的 MAX(create_time)（表为空时为 1970-01-01），从而只处理「上次抽取之后新增」的跟进，可配合 BatchSize>1 使用。
func (r *Repo) ListFollowLogsForExtract(ctx context.Context, since *time.Time) ([]FollowLogForExtract, error) {
	var cutoff time.Time
	if since != nil {
		cutoff = *since
	} else {
		var t time.Time
		err := r.db.GetContext(ctx, &t, `SELECT COALESCE(MAX(create_time), '1970-01-01'::timestamptz) FROM sales_keyword_records`)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("max create_time for extract cutoff: %w", err)
		}
		if err == nil {
			cutoff = t
		}
	}
	query := `
SELECT id,
  '跟进事项：' || COALESCE(follow_content,'') ||
  E'\n跟进结果：' || COALESCE(follow_result,'') ||
  E'\n下一步计划：' || COALESCE(next_plan,'') ||
  E'\n风险：' || COALESCE(risk_content,'')
  AS log_text
FROM follow_records
WHERE created_at > $1
ORDER BY created_at ASC`
	var rows []FollowLogForExtract
	if err := r.db.SelectContext(ctx, &rows, query, cutoff); err != nil {
		return nil, fmt.Errorf("list follow logs for extract: %w", err)
	}
	return rows, nil
}

// InsertKeywordRecords 批量写入关键词明细
func (r *Repo) InsertKeywordRecords(ctx context.Context, records []KeywordRecord) error {
	if len(records) == 0 {
		return nil
	}
	q := `INSERT INTO sales_keyword_records (id, category, term, count, create_time) VALUES ($1, $2, $3, $4, $5)`
	for _, rec := range records {
		id := rec.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		_, err := r.db.ExecContext(ctx, q, id, rec.Category, rec.Term, rec.Count, rec.CreateTime)
		if err != nil {
			return fmt.Errorf("insert keyword record: %w", err)
		}
	}
	return nil
}

// LoadSynonyms 加载同义词词典 source_term -> target_term
func (r *Repo) LoadSynonyms(ctx context.Context) (map[string]string, error) {
	var rows []struct {
		Source string `db:"source_term"`
		Target string `db:"target_term"`
	}
	if err := r.db.SelectContext(ctx, &rows, `SELECT source_term, target_term FROM sales_keyword_synonyms`); err != nil {
		if err == sql.ErrNoRows {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("load synonyms: %w", err)
	}
	m := make(map[string]string, len(rows))
	for _, row := range rows {
		m[row.Source] = row.Target
	}
	return m, nil
}

// AggregateByTimeWindow 从 sales_keyword_records 按时间窗口聚合，返回 category, term, frequency，按 frequency 降序
func (r *Repo) AggregateByTimeWindow(ctx context.Context, windowDays int, limitPerCategory int) ([]struct {
	Category  string `db:"category"`
	Term      string `db:"term"`
	Frequency int    `db:"frequency"`
}, error) {
	if limitPerCategory <= 0 {
		limitPerCategory = defaultLimitPerCategory
	}
	query := fmt.Sprintf(`
WITH ranked AS (
  SELECT category, term, SUM(count) AS frequency,
         ROW_NUMBER() OVER (PARTITION BY category ORDER BY SUM(count) DESC) AS rn
  FROM sales_keyword_records
  WHERE create_time >= NOW() - INTERVAL '%d days'
  GROUP BY category, term
)
SELECT category, term, frequency FROM ranked WHERE rn <= $1 ORDER BY category, rn`, windowDays)
	var out []struct {
		Category  string `db:"category"`
		Term      string `db:"term"`
		Frequency int    `db:"frequency"`
	}
	if err := r.db.SelectContext(ctx, &out, query, limitPerCategory); err != nil {
		return nil, fmt.Errorf("aggregate by time window: %w", err)
	}
	return out, nil
}

// DeleteStatsNotForDate 删除指定日期以外的所有统计（只保留该日期的数据）
func (r *Repo) DeleteStatsNotForDate(ctx context.Context, date time.Time) error {
	dateStr := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Format("2006-01-02")
	_, err := r.db.ExecContext(ctx, `DELETE FROM sales_hot_words_stats WHERE run_time::date != $1::date`, dateStr)
	if err != nil {
		return fmt.Errorf("delete stats not for date: %w", err)
	}
	return nil
}

// InsertStats 批量写入统计结果表
func (r *Repo) InsertStats(ctx context.Context, runTime time.Time, timeWindowDays int, rows []StatsRow) error {
	q := `INSERT INTO sales_hot_words_stats (id, run_time, time_window_days, category, term, frequency, rank, create_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (run_time, time_window_days, category, term) DO UPDATE SET frequency = EXCLUDED.frequency, rank = EXCLUDED.rank, create_time = EXCLUDED.create_time`
	for _, row := range rows {
		id := row.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		_, err := r.db.ExecContext(ctx, q, id, runTime, timeWindowDays, row.Category, row.Term, row.Frequency, row.Rank, row.CreateTime)
		if err != nil {
			return fmt.Errorf("insert stats: %w", err)
		}
	}
	return nil
}

// LatestStatsByWindow 取各时间窗口最近一次统计结果（用于 H5 展示）
func (r *Repo) LatestStatsByWindow(ctx context.Context, timeWindowDays int) ([]StatsRow, error) {
	query := `
SELECT id, run_time, time_window_days, category, term, frequency, rank, create_time
FROM sales_hot_words_stats
WHERE time_window_days = $1
  AND run_time = (SELECT MAX(run_time) FROM sales_hot_words_stats WHERE time_window_days = $1)
ORDER BY category, rank`
	var out []StatsRow
	if err := r.db.SelectContext(ctx, &out, query, timeWindowDays); err != nil {
		return nil, fmt.Errorf("latest stats by window: %w", err)
	}
	return out, nil
}

// RunTimeForDate 取某日（run_time 的日期）当天最后一次统计的 run_time；若该日无统计则返回零值
func (r *Repo) RunTimeForDate(ctx context.Context, date time.Time) (time.Time, error) {
	dateStr := date.Format("2006-01-02")
	var t time.Time
	query := `SELECT COALESCE(MAX(run_time), '0001-01-01'::timestamptz) FROM sales_hot_words_stats WHERE run_time::date = $1::date`
	if err := r.db.GetContext(ctx, &t, query, dateStr); err != nil {
		return time.Time{}, fmt.Errorf("run time for date: %w", err)
	}
	return t, nil
}

// StatsByRunTime 取指定 run_time 下各时间窗口的统计（同一 run 下所有 window 的 run_time 相同）
func (r *Repo) StatsByRunTime(ctx context.Context, runTime time.Time) ([]StatsRow, error) {
	query := `
SELECT id, run_time, time_window_days, category, term, frequency, rank, create_time
FROM sales_hot_words_stats
WHERE run_time = $1
ORDER BY time_window_days, category, rank`
	var out []StatsRow
	if err := r.db.SelectContext(ctx, &out, query, runTime); err != nil {
		return nil, fmt.Errorf("stats by run time: %w", err)
	}
	return out, nil
}

// ListRunDates 返回有统计结果的日期列表（run_time 的日期，去重，降序），用于前端日期选择
func (r *Repo) ListRunDates(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 31
	}
	query := `SELECT DISTINCT run_time::date::text AS d FROM sales_hot_words_stats ORDER BY d DESC LIMIT $1`
	var out []string
	if err := r.db.SelectContext(ctx, &out, query, limit); err != nil {
		return nil, fmt.Errorf("list run dates: %w", err)
	}
	return out, nil
}
