package hotwords

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// BuildAndPersistStats 按时间窗口从关键词表聚合，写入统计结果表；只保留当天统计，runTime 为本次统计批次时间
func BuildAndPersistStats(ctx context.Context, db *sqlx.DB, runTime time.Time, limitPerCategory int) error {
	repo := NewRepo(db)
	if err := repo.DeleteStatsNotForDate(ctx, runTime); err != nil {
		return err
	}
	for _, windowDays := range TimeWindowDays {
		rows, err := repo.AggregateByTimeWindow(ctx, windowDays, limitPerCategory)
		if err != nil {
			return err
		}
		// 按 category 分组并赋予 rank
		rankByCat := make(map[string]int)
		var stats []StatsRow
		for _, r := range rows {
			rankByCat[r.Category]++
			stats = append(stats, StatsRow{
				RunTime:         runTime,
				TimeWindowDays:  windowDays,
				Category:        r.Category,
				Term:            r.Term,
				Frequency:       r.Frequency,
				Rank:            rankByCat[r.Category],
				CreateTime:      runTime,
			})
		}
		if err := repo.InsertStats(ctx, runTime, windowDays, stats); err != nil {
			return err
		}
	}
	return nil
}
