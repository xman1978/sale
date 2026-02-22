package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"records/internal/config"

	"github.com/sirupsen/logrus"
)

// Logger 日志接口
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// logrusLogger logrus实现
type logrusLogger struct {
	logger       *logrus.Logger
	reportCaller bool
}

// New 创建新的日志实例
func New(cfg config.Logging) Logger {
	logger := logrus.New()

	// 设置日志级别
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// 不使用 logrus 的 SetReportCaller，因其只能拿到 logger.go 的调用位置
	// 真实调用位置由 mergeFields 通过 runtime.Caller 手动获取
	logger.SetReportCaller(false)

	// 设置日志格式
	if cfg.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	// 设置输出
	if cfg.Output == "file" && cfg.FilePath != "" {
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			logger.SetOutput(file)
		}
	} else {
		logger.SetOutput(os.Stdout)
	}

	return &logrusLogger{logger: logger, reportCaller: cfg.Caller}
}

func (l *logrusLogger) Debug(msg string, fields ...interface{}) {
	l.logger.WithFields(l.mergeFields(l.parseFields(fields...))).Debug(msg)
}

func (l *logrusLogger) Info(msg string, fields ...interface{}) {
	l.logger.WithFields(l.mergeFields(l.parseFields(fields...))).Info(msg)
}

func (l *logrusLogger) Warn(msg string, fields ...interface{}) {
	l.logger.WithFields(l.mergeFields(l.parseFields(fields...))).Warn(msg)
}

func (l *logrusLogger) Error(msg string, fields ...interface{}) {
	l.logger.WithFields(l.mergeFieldsWithStack(l.parseFields(fields...))).Error(msg)
}

func (l *logrusLogger) Fatal(msg string, fields ...interface{}) {
	l.logger.WithFields(l.mergeFieldsWithStack(l.parseFields(fields...))).Fatal(msg)
}

// parseFields 解析字段参数
func (l *logrusLogger) parseFields(fields ...interface{}) logrus.Fields {
	logFields := logrus.Fields{}

	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			if key, ok := fields[i].(string); ok {
				logFields[key] = fields[i+1]
			}
		}
	}

	return logFields
}

// mergeFields 合并 caller 信息，skip=3 跳过 getCaller/mergeFields/本包方法，得到真实调用位置
func (l *logrusLogger) mergeFields(fields logrus.Fields) logrus.Fields {
	if !l.reportCaller {
		return fields
	}
	file, line := getCaller(3)
	fields["file"] = file
	fields["line"] = line
	return fields
}

// mergeFieldsWithStack 合并 caller 信息和调用堆栈，用于 Error/Fatal；Error/Fatal 始终包含 stack 便于定位
func (l *logrusLogger) mergeFieldsWithStack(fields logrus.Fields) logrus.Fields {
	fields = l.mergeFields(fields)
	fields["stack"] = getStack(3)
	return fields
}

// getCaller 返回调用者的文件路径（相对路径）和行号，skip 为跳过的栈帧数
func getCaller(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "?", 0
	}
	wd, err := os.Getwd()
	if err == nil {
		if rel, err := filepath.Rel(wd, file); err == nil {
			return filepath.ToSlash(rel), line
		}
	}
	return filepath.ToSlash(file), line
}

// getStack 返回调用堆栈字符串，skip 为跳过的栈帧数（跳过 getStack/mergeFieldsWithStack/本包方法）
func getStack(skip int) string {
	pc := make([]uintptr, 64)
	n := runtime.Callers(skip, pc)
	if n == 0 {
		return ""
	}
	pc = pc[:n]
	frames := runtime.CallersFrames(pc)
	var out bytes.Buffer
	for {
		frame, more := frames.Next()
		if frame.File == "" {
			break
		}
		file := frame.File
		wd, err := os.Getwd()
		if err == nil {
			if rel, err := filepath.Rel(wd, file); err == nil {
				file = filepath.ToSlash(rel)
			}
		}
		fmt.Fprintf(&out, "\n\t%s:%d %s", file, frame.Line, frame.Function)
		if !more {
			break
		}
	}
	return out.String()
}
