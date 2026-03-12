package hotwords

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// cutoffDate 用于判断“无统计”的边界（早于此日期视为当日尚未生成）
var cutoffDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// RunIfNotGeneratedToday 若当天尚未生成热词统计则执行 runPipeline，否则不执行；返回本次是否执行了流水线
func RunIfNotGeneratedToday(ctx context.Context, db *sqlx.DB, runPipeline func(context.Context) error) (ran bool, err error) {
	repo := NewRepo(db)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	runTime, err := repo.RunTimeForDate(ctx, today)
	if err != nil {
		return false, err
	}
	if !runTime.Before(cutoffDate) {
		return false, nil
	}
	if err := runPipeline(ctx); err != nil {
		return false, err
	}
	return true, nil
}
