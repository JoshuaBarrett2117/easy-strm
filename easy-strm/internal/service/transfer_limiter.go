package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"easy-strm/internal/pkg/logger"
)

const (
	defaultRateLimitWindow = 1 * time.Second
	defaultMaxTransfers    = 5
	accountRateLimitKey    = "easy_strm:ratelimit:account:"
	globalRateLimitKey     = "easy_strm:ratelimit:global"
	defaultGlobalMaxQPS    = 20
)

type TransferLimiter struct {
	redisClient    *redis.Client
	accountLimits  map[int]*AccountLimiter
	accountMu      sync.RWMutex
	globalLimiter  *TokenBucket
	maxTransfers   int
	rateLimitWindow time.Duration
}

type AccountLimiter struct {
	bucket  *TokenBucket
	lastHit time.Time
}

type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
}

func (tb *TokenBucket) Wait(duration time.Duration) error {
	start := time.Now()
	for !tb.Allow() {
		if time.Since(start) > duration {
			return fmt.Errorf("timeout waiting for token")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func NewTransferLimiter(redisClient *redis.Client) *TransferLimiter {
	limiter := &TransferLimiter{
		redisClient:    redisClient,
		accountLimits:  make(map[int]*AccountLimiter),
		maxTransfers:   defaultMaxTransfers,
		rateLimitWindow: defaultRateLimitWindow,
	}

	limiter.globalLimiter = NewTokenBucket(float64(defaultGlobalMaxQPS), float64(defaultGlobalMaxQPS))

	return limiter
}

func (l *TransferLimiter) Wait(accountID int) error {
	l.accountMu.RLock()
	accountLimiter, exists := l.accountLimits[accountID]
	l.accountMu.RUnlock()

	if !exists {
		l.accountMu.Lock()
		if _, exists = l.accountLimits[accountID]; !exists {
			l.accountLimits[accountID] = &AccountLimiter{
				bucket:  NewTokenBucket(float64(l.maxTransfers), float64(l.maxTransfers)),
				lastHit: time.Now(),
			}
		}
		accountLimiter = l.accountLimits[accountID]
		l.accountMu.Unlock()
	}

	if !accountLimiter.bucket.Allow() {
		logger.Warnf("TransferLimiter[Wait] Account %d rate limit exceeded, waiting...", accountID)
		_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := accountLimiter.bucket.Wait(5 * time.Second)
		if err != nil {
			return fmt.Errorf("account %d rate limit wait timeout: %w", accountID, err)
		}
	}

	if !l.globalLimiter.Allow() {
		logger.Warnf("TransferLimiter[Wait] Global rate limit exceeded, waiting...")
		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := l.globalLimiter.Wait(10 * time.Second)
		if err != nil {
			return fmt.Errorf("global rate limit wait timeout: %w", err)
		}
	}

	return nil
}

func (l *TransferLimiter) Allow(accountID int) bool {
	l.accountMu.RLock()
	accountLimiter, exists := l.accountLimits[accountID]
	l.accountMu.RUnlock()

	if !exists {
		return true
	}

	if !accountLimiter.bucket.Allow() {
		return false
	}

	if !l.globalLimiter.Allow() {
		return false
	}

	return true
}

func (l *TransferLimiter) GetAccountStats(accountID int) (tokens float64, maxTokens float64) {
	l.accountMu.RLock()
	accountLimiter, exists := l.accountLimits[accountID]
	l.accountMu.RUnlock()

	if !exists || accountLimiter == nil {
		return float64(l.maxTransfers), float64(l.maxTransfers)
	}

	accountLimiter.bucket.mu.Lock()
	defer accountLimiter.bucket.mu.Unlock()
	accountLimiter.bucket.refill()

	return accountLimiter.bucket.tokens, accountLimiter.bucket.maxTokens
}

func (l *TransferLimiter) GetGlobalStats() (tokens float64, maxTokens float64) {
	l.globalLimiter.mu.Lock()
	defer l.globalLimiter.mu.Unlock()
	l.globalLimiter.refill()

	return l.globalLimiter.tokens, l.globalLimiter.maxTokens
}

func (l *TransferLimiter) SetMaxTransfersPerAccount(max int) {
	l.maxTransfers = max
	logger.Infof("TransferLimiter[SetMaxTransfersPerAccount] Updated to %d", max)
}

func (l *TransferLimiter) SetGlobalMaxQPS(max int) {
	if l.globalLimiter != nil {
		l.globalLimiter.mu.Lock()
		l.globalLimiter.maxTokens = float64(max)
		l.globalLimiter.refillRate = float64(max)
		l.globalLimiter.mu.Unlock()
	}
	logger.Infof("TransferLimiter[SetGlobalMaxQPS] Updated to %d", max)
}

func (l *TransferLimiter) ResetAccount(accountID int) {
	l.accountMu.Lock()
	delete(l.accountLimits, accountID)
	l.accountMu.Unlock()
	logger.Infof("TransferLimiter[ResetAccount] Reset limiter for account %d", accountID)
}

func (l *TransferLimiter) ResetAll() {
	l.accountMu.Lock()
	l.accountLimits = make(map[int]*AccountLimiter)
	l.accountMu.Unlock()
	logger.Infof("TransferLimiter[ResetAll] All account limiters reset")
}
