// 热词流水线 CLI：在服务器上手动执行。若当天尚未生成热词统计则执行并写入，否则不执行。
// 用法（在 records 目录下）：go run ./cmd/hotwords [config.yml]
package main

import (
	"context"
	"log"
	"os"

	"records/internal/config"
	"records/internal/database"
	"records/internal/hotwords"
	"records/pkg/logger"
)

func main() {
	cfgPath := "config.yml"
	if len(os.Args) > 1 && os.Args[1] != "" {
		cfgPath = os.Args[1]
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	loggr := logger.New(cfg.Logging)
	db, err := database.New(cfg.Database)
	if err != nil {
		loggr.Fatal("connect database", "error", err)
	}
	defer db.Close()

	loggr.Info("initializing database schema...")
	if err := database.InitDatabase(db); err != nil {
		loggr.Fatal("init database", "error", err)
	}

	extractor := hotwords.NewExtractor(hotwords.ExtractorConfig{
		APIKey:              cfg.AI.OpenAI.APIKey,
		BaseURL:             cfg.AI.OpenAI.BaseURL,
		ModelName:           cfg.AI.OpenAI.ModelName,
		Temperature:         0.3,
		MaxCompletionTokens: cfg.AI.OpenAI.MaxCompletionTokens,
		SystemPrompt:        cfg.Prompts.HotwordsExtractor,
	}, loggr)

	pipe := hotwords.NewPipeline(db, extractor, hotwords.PipelineConfig{
		BatchSize:        20,
		LimitPerCategory: 50,
		IncrementalSince: nil,
	}, loggr)

	ctx := context.Background()
	ran, err := hotwords.RunIfNotGeneratedToday(ctx, db, pipe.Run)
	if err != nil {
		loggr.Fatal("hotwords run_once", "error", err)
	}
	if ran {
		loggr.Info("已生成当日热词统计")
	} else {
		loggr.Info("当日已有热词统计，未执行")
	}
}
