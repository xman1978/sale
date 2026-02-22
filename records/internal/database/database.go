package database

import (
	"fmt"
	"os"
	"path/filepath"

	"records/internal/config"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// New 创建数据库连接
func New(cfg config.Database) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s search_path=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode, cfg.Schema)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// loadSchemaSQL 从文件加载 SQL 架构
func loadSchemaSQL() (string, error) {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// 构建 schema.sql 文件路径
	schemaPath := filepath.Join(wd, "sql", "schema.sql")

	// 读取 SQL 文件
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", fmt.Errorf("failed to read schema.sql: %w", err)
	}

	return string(sqlBytes), nil
}

// 初始化数据库，创建表结构
func InitDatabase(db *sqlx.DB) error {

	// 检查表是否已存在
	var tableCount int
	err := db.Get(&tableCount, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'sale'
		AND table_name IN ('users', 'customers', 'sessions', 'dialogs', 'follow_records')
	`)
	if err != nil {
		return fmt.Errorf("failed to check existing tables: %w", err)
	}

	// 如果所有表都已存在，跳过迁移
	if tableCount >= 5 {
		return nil
	}

	// 从文件加载 SQL 架构
	schemaSQL, err := loadSchemaSQL()
	if err != nil {
		return fmt.Errorf("failed to load schema SQL: %w", err)
	}

	// 执行数据库初始化SQL
	_, err = db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute database schema: %w", err)
	}

	return nil
}
