package server

import (
	"context"
	"time"

	"records/internal/hotwords"
)

// runHotwordsScheduler 在后台按日执行热词流水线（每日 00:05 运行一次）
func (s *Server) runHotwordsScheduler(ctx context.Context) {
	extractor := hotwords.NewExtractor(hotwords.ExtractorConfig{
		APIKey:              s.config.AI.OpenAI.APIKey,
		BaseURL:             s.config.AI.OpenAI.BaseURL,
		ModelName:           s.config.AI.OpenAI.ModelName,
		Temperature:         0.3,
		MaxCompletionTokens: s.config.AI.OpenAI.MaxCompletionTokens,
		SystemPrompt:        s.config.Prompts.HotwordsExtractor,
	}, s.logger)
	pipe := hotwords.NewPipeline(s.db, extractor, hotwords.PipelineConfig{
		BatchSize:        20,
		LimitPerCategory: 50,
		IncrementalSince: nil,
	}, s.logger)

	// 计算下次 00:05 的时间
	next := func() time.Time {
		now := time.Now()
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 5, 0, 0, now.Location())
		if t.Before(now) || t.Equal(now) {
			t = t.Add(24 * time.Hour)
		}
		return t
	}
	timer := time.NewTimer(time.Until(next()))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.logger.Info("hotwords pipeline: scheduled run check")
			ran, err := hotwords.RunIfNotGeneratedToday(ctx, s.db, pipe.Run)
			if err != nil {
				s.logger.Error("hotwords pipeline failed", "error", err)
			} else if ran {
				s.logger.Info("hotwords pipeline: scheduled run completed, 已生成当日热词统计")
			} else {
				s.logger.Info("hotwords pipeline: 当日已有热词统计，跳过")
			}
			timer.Reset(time.Until(next()))
		}
	}
}
