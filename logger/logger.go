package logger

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"
)

// Logger 定义日志记录器结构
type Logger struct {
	debugMode atomic.Bool
	logger    *log.Logger
}

// New 创建新的日志记录器
func New(debug bool) *Logger {
	l := &Logger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
	l.debugMode.Store(debug)
	return l
}

// SetDebug 设置调试模式
func (l *Logger) SetDebug(debug bool) {
	l.debugMode.Store(debug)
}

// Debug 记录调试信息
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.debugMode.Load() {
		l.logger.Printf("[DEBUG] "+format, v...)
	}
}

// Info 记录一般信息
func (l *Logger) Info(format string, v ...interface{}) {
	l.logger.Printf("[INFO] "+format, v...)
}

// Warn 记录警告信息
func (l *Logger) Warn(format string, v ...interface{}) {
	l.logger.Printf("[WARN] "+format, v...)
}

// Error 记录错误信息
func (l *Logger) Error(format string, v ...interface{}) {
	l.logger.Printf("[ERROR] "+format, v...)
}

// Fatal 记录致命错误并退出程序
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.logger.Fatalf("[FATAL] "+format, v...)
	// 为了确保程序退出，添加显式的os.Exit调用
	os.Exit(1)
}

// LogRequest 记录HTTP请求
func (l *Logger) LogRequest(method, url string, duration time.Duration, statusCode int) {
	l.logger.Printf("[REQUEST] %s %s %d %v", method, url, statusCode, duration)
}

// FormatBytes 格式化字节数
func (l *Logger) FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatDuration 格式化持续时间
func (l *Logger) FormatDuration(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%.2f h", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%.2f m", d.Minutes())
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	default:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
}
