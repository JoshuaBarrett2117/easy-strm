package main

import (
	"fmt"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
	MaxDelay    time.Duration
	Backoff     bool
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	Delay:       1 * time.Second,
	MaxDelay:    10 * time.Second,
	Backoff:     true,
}

// Retry 执行带有重试机制的函数
func Retry(fn func() error, config RetryConfig) error {
	var lastErr error

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		Debug("Attempt %d of %d", attempt, config.MaxAttempts)

		err := fn()
		if err == nil {
			// 操作成功
			if attempt > 1 {
				Info("Operation succeeded after %d attempts", attempt)
			}
			return nil
		}

		// 操作失败，记录错误
		lastErr = err
		Debug("Attempt %d failed: %v", attempt, err)

		// 检查是否还有重试机会
		if attempt >= config.MaxAttempts {
			break
		}

		// 计算重试延迟
		delay := config.Delay
		if config.Backoff {
			delay = delay * time.Duration(attempt)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		// 等待后重试
		Debug("Waiting %v before next attempt", delay)
		time.Sleep(delay)
	}

	Error("Operation failed after %d attempts: %v", config.MaxAttempts, lastErr)
	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// RetryWithResult 执行带有重试机制的函数，返回结果
func RetryWithResult[T any](fn func() (T, error), config RetryConfig) (T, error) {
	var lastErr error
	var result T

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		Debug("Attempt %d of %d", attempt, config.MaxAttempts)

		currentResult, err := fn()
		if err == nil {
			// 操作成功
			if attempt > 1 {
				Info("Operation succeeded after %d attempts", attempt)
			}
			return currentResult, nil
		}

		// 操作失败，记录错误
		lastErr = err
		result = currentResult
		Debug("Attempt %d failed: %v", attempt, err)

		// 检查是否还有重试机会
		if attempt >= config.MaxAttempts {
			break
		}

		// 计算重试延迟
		delay := config.Delay
		if config.Backoff {
			delay = delay * time.Duration(attempt)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		// 等待后重试
		Debug("Waiting %v before next attempt", delay)
		time.Sleep(delay)
	}

	Error("Operation failed after %d attempts: %v", config.MaxAttempts, lastErr)
	return result, fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// IsRetryableError 检查错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查常见的可重试错误
	errorStr := err.Error()
	retryableErrors := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"network is unreachable",
		"no such host",
		"temporary failure in name resolution",
		"502 Bad Gateway",
		"503 Service Unavailable",
		"504 Gateway Timeout",
	}

	for _, retryable := range retryableErrors {
		if containsIgnoreCase(errorStr, retryable) {
			return true
		}
	}

	return false
}

// containsIgnoreCase 检查字符串是否包含子串（忽略大小写）
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalsIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalsIgnoreCase 比较两个字符串是否相等（忽略大小写）
func equalsIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

// toLower 将字符转换为小写
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
