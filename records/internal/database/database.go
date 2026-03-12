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
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	schemaPath := filepath.Join(wd, "sql", "schema.sql")
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", fmt.Errorf("failed to read schema.sql: %w", err)
	}
	return string(sqlBytes), nil
}

// loadHotwordsSQL 加载热词表 DDL
func loadHotwordsSQL() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	p := filepath.Join(wd, "sql", "hotwords.sql")
	sqlBytes, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("failed to read hotwords.sql: %w", err)
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

	if tableCount < 5 {
		schemaSQL, err := loadSchemaSQL()
		if err != nil {
			return fmt.Errorf("failed to load schema SQL: %w", err)
		}
		if _, err = db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("failed to execute database schema: %w", err)
		}
	}

	// 热词表：若不存在则创建
	var hotwordsExist int
	err = db.Get(&hotwordsExist, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'sale' AND table_name = 'sales_keyword_records'`)
	if err != nil {
		return fmt.Errorf("failed to check hotwords tables: %w", err)
	}
	if hotwordsExist == 0 {
		hotwordsSQL, err := loadHotwordsSQL()
		if err != nil {
			return fmt.Errorf("failed to load hotwords SQL: %w", err)
		}
		if _, err = db.Exec(hotwordsSQL); err != nil {
			return fmt.Errorf("failed to execute hotwords schema: %w", err)
		}
	}

	return nil
}
