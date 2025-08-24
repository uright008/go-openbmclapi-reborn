package cluster

import (
	"os"
	"sync"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/logger"
)

// ErrorRetryManager 错误重试管理器
type ErrorRetryManager struct {
	maxRetries    int
	errorCount    int
	lastErrorTime time.Time
	mu            sync.Mutex
	logger        *logger.Logger
}

// NewErrorRetryManager 创建新的错误重试管理器
func NewErrorRetryManager(maxRetries int, logger *logger.Logger) *ErrorRetryManager {
	return &ErrorRetryManager{
		maxRetries: maxRetries,
		logger:     logger,
	}
}

// RecordError 记录错误，如果错误次数超过最大重试次数则关闭进程
func (erm *ErrorRetryManager) RecordError(err error) {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	erm.errorCount++
	erm.lastErrorTime = time.Now()

	erm.logger.Error("发生错误 (%d/%d): %v", erm.errorCount, erm.maxRetries, err)

	if erm.errorCount > erm.maxRetries {
		erm.logger.Fatal("错误次数超过最大重试次数 (%d)，正在关闭进程", erm.maxRetries)
		os.Exit(1)
	}
}

// ResetErrors 重置错误计数
func (erm *ErrorRetryManager) ResetErrors() {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	if erm.errorCount > 0 {
		erm.logger.Info("重置错误计数: %d -> 0", erm.errorCount)
		erm.errorCount = 0
	}
}

// GetErrorCount 获取当前错误计数
func (erm *ErrorRetryManager) GetErrorCount() int {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	return erm.errorCount
}
