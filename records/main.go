package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"records/internal/config"
	"records/internal/database"
	"records/internal/feishu"
	"records/internal/server"
	"records/pkg/logger"
)

func main() {
	// 加载配置
	cfg, err := config.Load("config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化日志
	logger := logger.New(cfg.Logging)

	// 初始化数据库
	db, err := database.New(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", "error", err)
	}
	defer db.Close()

	// 运行数据库迁移
	logger.Info("Initializing database schema...")
	if err := database.InitDatabase(db); err != nil {
		logger.Fatal("Failed to initialize database schema", "error", err)
	}
	logger.Info("Database schema initialized successfully")

	// 初始化飞书客户端
	feishuClient := feishu.NewClient(cfg.Feishu, logger)

	// 初始化服务器
	srv := server.New(cfg, db, feishuClient, logger)

	// 启动服务器
	go func() {
		if err := srv.Start(); err != nil {
			logger.Fatal("Failed to start server", "error", err)
		}
	}()

	logger.Info("Sales log system started successfully")

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited")
}
